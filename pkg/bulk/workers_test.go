package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/yg-codes/proxmox-cli/pkg/snapshot"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// Documents the worker-count split: snapshot and VM power operations are
// configured independently.
func TestWorkerCountsAreIndependent(t *testing.T) {
	m := NewManager(nil, nil, nil)

	if m.maxWorkers != 3 || m.maxVMWorkers != 3 {
		t.Fatalf("defaults: maxWorkers=%d maxVMWorkers=%d, want 3/3", m.maxWorkers, m.maxVMWorkers)
	}

	m.SetMaxWorkers(2)   // snapshot limit
	m.SetMaxVMWorkers(8) // vm limit

	if m.maxWorkers != 2 {
		t.Errorf("maxWorkers=%d, want 2", m.maxWorkers)
	}
	if m.maxVMWorkers != 8 {
		t.Errorf("maxVMWorkers=%d, want 8 (VM ops must not inherit the snapshot limit)", m.maxVMWorkers)
	}

	// Zero/negative must be ignored, matching SetMaxWorkers' existing contract.
	m.SetMaxVMWorkers(0)
	m.SetMaxVMWorkers(-1)
	if m.maxVMWorkers != 8 {
		t.Errorf("maxVMWorkers=%d after invalid input, want 8", m.maxVMWorkers)
	}
}

// --- Task 2: lifecycle regression tests -----------------------------------
//
// These pin two lifecycle contracts the Manager must hold:
//  1. one Manager can run several operations sequentially (the result channel
//     is per-operation, never reused after being closed), and
//  2. a cancelled operation returns promptly instead of waiting forever for a
//     result count that cancelled workers will never produce.

// writeJSON writes a Proxmox-style envelope response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// newBulkTestManager returns a Manager wired to a local TLS test server that
// speaks the subset of the Proxmox API the snapshot workers touch. createDelay,
// when non-zero, slows each snapshot-create call so a test can cancel while
// work is still in flight.
func newBulkTestManager(t *testing.T, createDelay time.Duration, failDeleteVMIDs ...string) *Manager {
	t.Helper()

	failDelete := make(map[string]bool, len(failDeleteVMIDs))
	for _, vmid := range failDeleteVMIDs {
		failDelete[vmid] = true
	}

	taskStatus := regexp.MustCompile(`^/api2/json/nodes/[^/]+/tasks/.+/status$`)

	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{{"node": "pve1", "status": "online"}},
		})
	})
	mux.HandleFunc("/api2/json/nodes/pve1/qemu/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api2/json/nodes/pve1/qemu/"), "/")
		vmid := parts[0]
		switch {
		case len(parts) == 3 && parts[1] == "status":
			writeJSON(w, map[string]interface{}{
				"data": map[string]interface{}{"vmid": vmid, "name": "vm-" + vmid, "status": "running"},
			})
		case len(parts) == 2 && parts[1] == "snapshot" && r.Method == http.MethodGet:
			writeJSON(w, map[string]interface{}{
				"data": []map[string]interface{}{{"name": "snap1", "snaptime": 1700000000.0}},
			})
		case len(parts) == 2 && parts[1] == "snapshot" && r.Method == http.MethodPost:
			if createDelay > 0 {
				time.Sleep(createDelay)
			}
			writeJSON(w, map[string]interface{}{
				"data": "UPID:pve1:TEST:TEST:0:qmsnapshot:" + vmid + ":root@pam:",
			})
		case len(parts) == 3 && parts[1] == "snapshot" && r.Method == http.MethodDelete:
			if failDelete[vmid] {
				http.Error(w, `{"errors":{"any":"permission denied"}}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]interface{}{
				"data": "UPID:pve1:TEST:TEST:0:qmdelsnapshot:" + vmid + ":root@pam:",
			})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api2/json/nodes/pve1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if !taskStatus.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
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
	vmOps := vm.NewOperations(client, logger)
	snapOps := snapshot.NewOperations(client, vmOps, logger)
	return NewManager(vmOps, snapOps, logger)
}

func testVMs(n int) []*vm.VM {
	vms := make([]*vm.VM, 0, n)
	for i := 100; i < 100+n; i++ {
		vms = append(vms, &vm.VM{VMID: strconv.Itoa(i), Name: fmt.Sprintf("vm-%d", i)})
	}
	return vms
}

// A second operation on the same Manager must not panic on the result channel
// the first operation closed.
func TestManagerCanRunSequentialOperations(t *testing.T) {
	m := newBulkTestManager(t, 0)
	vms := testVMs(2)

	for call := 1; call <= 2; call++ {
		if err := m.DeleteSnapshots(context.Background(), vms, "snap1"); err != nil {
			t.Fatalf("sequential DeleteSnapshots call %d: %v", call, err)
		}
		if got := len(m.GetResults()); got != len(vms) {
			t.Fatalf("call %d: results=%d, want %d", call, got, len(vms))
		}
	}
}

// Cancelling mid-operation must return context.Canceled within the deadline —
// not spin forever waiting for results cancelled workers will never send.
func TestCreateSnapshotsCancellationReturns(t *testing.T) {
	m := newBulkTestManager(t, 150*time.Millisecond) // slow creates
	vms := testVMs(10)                               // ~500ms of work at 3 workers

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(250 * time.Millisecond) // work remains at this point
		cancel()
	}()

	done := make(chan error, 1)
	go func() { done <- m.CreateSnapshots(ctx, vms, "snap", false, false) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("CreateSnapshots after cancellation: err=%v, want context.Canceled", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("CreateSnapshots did not return after cancellation")
	}

	// The operation must be fully finalized: every submitted VM either
	// completed or was dropped, and results are stable afterwards.
	if got := len(m.GetResults()); got > len(vms) {
		t.Errorf("results=%d, want at most %d", got, len(vms))
	}
}

// --- Task 3: aggregate failure exit contract --------------------------------

// One failed target among several must surface as a non-nil aggregate error
// naming the target, while the per-target summary (results) is retained.
func TestBulkPartialFailureReturnsAggregateError(t *testing.T) {
	m := newBulkTestManager(t, 0, "102")
	vms := testVMs(3) // 100, 101, 102

	err := m.DeleteSnapshots(context.Background(), vms, "snap1")
	if err == nil {
		t.Fatal("DeleteSnapshots with one failed target: err=nil, want aggregate error")
	}
	if !strings.Contains(err.Error(), "102") {
		t.Errorf("aggregate error %q does not name failed VM 102", err)
	}
	if got := len(m.GetResults()); got != len(vms) {
		t.Fatalf("results=%d, want %d (summary must be retained)", got, len(vms))
	}
}

// Every target failing must also produce a non-nil error naming each VM.
func TestBulkAllTargetsFailReturnsAggregateError(t *testing.T) {
	m := newBulkTestManager(t, 0, "100", "101", "102")
	vms := testVMs(3)

	err := m.DeleteSnapshots(context.Background(), vms, "snap1")
	if err == nil {
		t.Fatal("DeleteSnapshots with all targets failed: err=nil, want aggregate error")
	}
	for _, vmid := range []string{"100", "101", "102"} {
		if !strings.Contains(err.Error(), vmid) {
			t.Errorf("aggregate error %q does not name failed VM %s", err, vmid)
		}
	}
}
