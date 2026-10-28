package vm

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// newTaskTestOps wires vm.Operations to a test server whose task-status
// endpoint answers "running" forever for UPID "UPID:run", stops OK for
// "UPID:ok", and stops with a failure for "UPID:bad".
func newTaskTestOps(t *testing.T) *Operations {
	t.Helper()

	taskStatus := regexp.MustCompile(`^/api2/json/nodes/[^/]+/tasks/.+/status$`)

	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{{"node": "pve1", "status": "online"}},
		})
	})
	mux.HandleFunc("/api2/json/nodes/pve1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if !taskStatus.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "UPID:run"):
			writeJSON(w, map[string]interface{}{
				"data": map[string]interface{}{"status": "running"},
			})
		case strings.Contains(r.URL.Path, "UPID:bad"):
			writeJSON(w, map[string]interface{}{
				"data": map[string]interface{}{"status": "stopped", "exitstatus": "command failed - exit code 1"},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
			})
		}
	})

	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatalf("split test server URL: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	client := api.NewClient(&api.ClientConfig{
		Host:       host,
		Port:       port,
		Username:   "root@pam",
		TokenName:  "test",
		TokenValue: "secret",
		Logger:     logger,
	})
	return NewOperations(client, logger)
}

// writeJSON writes a Proxmox-style envelope response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// A task that never stops must end monitoring at the configured timeout with
// context.DeadlineExceeded — not poll forever.
func TestMonitorTaskTimeoutReturnsDeadlineExceeded(t *testing.T) {
	ops := newTaskTestOps(t)
	ops.SetTaskTimeout(200 * time.Millisecond)

	start := time.Now()
	err := ops.MonitorTask(context.Background(), "pve1", "UPID:run")
	if err == nil {
		t.Fatal("MonitorTask on a never-stopping task: err=nil, want deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err=%v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("MonitorTask took %v to give up, want it bounded near the timeout", elapsed)
	}
}

// Cancelling the caller's context must stop monitoring promptly with
// context.Canceled.
func TestMonitorTaskCancellationReturnsCanceled(t *testing.T) {
	ops := newTaskTestOps(t)
	ops.SetTaskTimeout(30 * time.Second) // timeout must NOT be the trigger

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := ops.MonitorTask(ctx, "pve1", "UPID:run")
	if err == nil {
		t.Fatal("MonitorTask after cancellation: err=nil, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("MonitorTask took %v to notice cancellation, want prompt return", elapsed)
	}
}

// Existing outcomes must be preserved: a stopped-OK task succeeds and a
// stopped-failed task reports the exit status.
func TestMonitorTaskStoppedOutcomesPreserved(t *testing.T) {
	ops := newTaskTestOps(t)

	if err := ops.MonitorTask(context.Background(), "pve1", "UPID:ok"); err != nil {
		t.Errorf("stopped/OK task: err=%v, want nil", err)
	}

	err := ops.MonitorTask(context.Background(), "pve1", "UPID:bad")
	if err == nil {
		t.Fatal("stopped/failed task: err=nil, want exit-status error")
	}
	if !strings.Contains(err.Error(), "exit code 1") {
		t.Errorf("err=%q, want it to carry the task exit status", err)
	}
}

// --- Task 8: complete-or-error cluster discovery -----------------------------

// newDiscoveryOps serves two nodes: pve1 lists one VM; pve2 lists the same
// shape of data, or fails every request when failPVE2 is set, exercising the
// partial-inventory path.
func newDiscoveryOps(t *testing.T, failPVE2 bool) *Operations {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{
				{"node": "pve1", "status": "online"},
				{"node": "pve2", "status": "online"},
			},
		})
	})
	qemuList := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{
				{"vmid": float64(100), "name": "vm-100", "status": "running"},
			},
		})
	}
	mux.HandleFunc("/api2/json/nodes/pve1/qemu", qemuList)
	mux.HandleFunc("/api2/json/nodes/pve2/", func(w http.ResponseWriter, r *http.Request) {
		if failPVE2 {
			http.Error(w, `{"errors":{"any":"node unreachable"}}`, http.StatusBadGateway)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/qemu") {
			qemuList(w, r)
			return
		}
		http.NotFound(w, r)
	})

	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatalf("split test server URL: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	client := api.NewClient(&api.ClientConfig{
		Host: host, Port: port,
		Username: "root@pam", TokenName: "test", TokenValue: "secret",
		Logger: logger,
	})
	return NewOperations(client, logger)
}

// One failed node must surface as a non-nil error naming that node, while the
// discovered VMs are still returned — callers abort before mutating on the
// error, and the partial slice documents what was readable.
func TestGetAllVMsNodeFailure(t *testing.T) {
	ops := newDiscoveryOps(t, true)

	vms, err := ops.GetAllVMs()
	if err == nil {
		t.Fatal("GetAllVMs with one failed node: err=nil, want aggregate error")
	}
	if !strings.Contains(err.Error(), "pve2") {
		t.Errorf("error %q does not name the failed node", err)
	}
	if len(vms) != 1 || vms[0].VMID != "100" {
		t.Errorf("partial discovery lost pve1's VMs: got %+v", vms)
	}
}

// The healthy path must remain a clean nil error.
func TestGetAllVMsAllNodesHealthy(t *testing.T) {
	ops := newDiscoveryOps(t, false)

	vms, err := ops.GetAllVMs()
	if err != nil {
		t.Fatalf("healthy cluster: err=%v, want nil", err)
	}
	if len(vms) != 2 {
		t.Errorf("vms=%d, want 2 (one per node)", len(vms))
	}
}
