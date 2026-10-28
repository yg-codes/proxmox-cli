package main

// Handler-layer test harness (issue #27).
//
// It runs the real command tree (rootCmd.Execute → initializeApp → RunE
// handlers) against a local TLS fixture speaking the Proxmox API subset, so
// the user-visible contract — exit error, printed summary, actual server-side
// mutation — is asserted end to end.
//
// Constraints (see the implementation notes on issue #27):
//   - package is SERIAL-ONLY: handlers and this harness share package globals;
//     no t.Parallel() anywhere in pve tests while this file exists
//   - cobra flags persist across Execute() calls — resetCommandFlags restores
//     every flag to its default before each run
//   - token auth performs no HTTP roundtrip, so the fixture needs no /access
//     route; PVE_HOST/PVE_PORT env (step 0 of the issue) points initializeApp
//     at the fixture

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// fixtureVM is one mutable VM in the fixture's cluster state.
type fixtureVM struct {
	VMID   string
	Name   string
	Node   string
	Status string // "running" | "stopped"
	Snaps  []string
}

// apiFixture is a composable in-memory Proxmox cluster behind a TLS test
// server. Tests configure failure maps and delays BEFORE execCLI and assert
// the mutated state AFTER.
type apiFixture struct {
	srv *httptest.Server
	mu  sync.Mutex

	nodes []string
	vms   map[string]*fixtureVM // keyed by VMID

	// Observable call counts (keyed by VMID where per-VM).
	startCalls    map[string]int
	shutdownCalls map[string]int
	vzdumpCalls   map[string]int
	snapDeletes   map[string]int
	totalRequests int

	// Failure injection.
	failStart    map[string]bool
	failShutdown map[string]bool
	failVzdump   map[string]bool
	failSnapDel  map[string]bool   // "vmid:snapname" -> fail this deletion
	startDelay   time.Duration     // server-side delay per start (cancellation tests)
	onStartHook  func(vmid string) // runs inside the server handler, first thing
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	f := &apiFixture{
		nodes:         []string{"pve1"},
		vms:           map[string]*fixtureVM{},
		startCalls:    map[string]int{},
		shutdownCalls: map[string]int{},
		vzdumpCalls:   map[string]int{},
		snapDeletes:   map[string]int{},
		failStart:     map[string]bool{},
		failShutdown:  map[string]bool{},
		failVzdump:    map[string]bool{},
		failSnapDel:   map[string]bool{},
	}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.route))
	t.Cleanup(f.srv.Close)
	return f
}

// addVM registers a VM in the fixture state.
func (f *apiFixture) addVM(vmid, name, status string, snaps ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vms[vmid] = &fixtureVM{VMID: vmid, Name: name, Node: f.nodes[0], Status: status, Snaps: snaps}
}

// vmStatus returns a copy of the VM's current status ("", ok=false when absent).
func (f *apiFixture) vmStatus(vmid string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	vm, ok := f.vms[vmid]
	if !ok {
		return "", false
	}
	return vm.Status, true
}

// snaps returns the VM's remaining snapshot names.
func (f *apiFixture) snaps(vmid string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	vm := f.vms[vmid]
	out := make([]string, len(vm.Snaps))
	copy(out, vm.Snaps)
	return out
}

func (f *apiFixture) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.totalRequests
}

// writeJSON writes a Proxmox-style envelope response.
func (f *apiFixture) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *apiFixture) apiError(w http.ResponseWriter, msg string) {
	http.Error(w, `{"errors":{"any":"`+msg+`"}}`, http.StatusInternalServerError)
}

var taskStatusRe = regexp.MustCompile(`^/api2/json/nodes/[^/]+/tasks/.+/status$`)

func (f *apiFixture) route(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.totalRequests++
	f.mu.Unlock()

	if taskStatusRe.MatchString(r.URL.Path) {
		f.writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api2/json/")
	parts := strings.Split(rest, "/")
	if parts[0] != "nodes" {
		http.NotFound(w, r)
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "nodes" && r.Method == http.MethodGet: // /nodes
		nodes := make([]map[string]interface{}, 0, len(f.nodes))
		for _, n := range f.nodes {
			nodes = append(nodes, map[string]interface{}{"node": n, "status": "online"})
		}
		f.writeJSON(w, map[string]interface{}{"data": nodes})
		return

	case len(parts) == 3 && parts[2] == "qemu" && r.Method == http.MethodGet: // VM list
		node := parts[1]
		var items []map[string]interface{}
		f.mu.Lock()
		for _, vm := range f.vms {
			if vm.Node != node {
				continue
			}
			id, _ := strconv.ParseFloat(vm.VMID, 64)
			items = append(items, map[string]interface{}{
				"vmid": id, "name": vm.Name, "status": vm.Status, "cpus": 2.0, "memory": 1024.0,
			})
		}
		f.mu.Unlock()
		f.writeJSON(w, map[string]interface{}{"data": items})
		return

	case len(parts) == 3 && parts[2] == "storage" && r.Method == http.MethodGet:
		f.writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{{"storage": "local", "type": "dir", "active": 1}},
		})
		return

	case len(parts) == 5 && parts[2] == "storage" && parts[4] == "status" && r.Method == http.MethodGet:
		// Storage status: object data is unwrapped by the client; GetBackupStorages
		// reads content/active/avail/total at the top level.
		f.writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"content": "backup", "active": 1.0,
				"avail": 10 * 1024 * 1024 * 1024.0, "total": 20 * 1024 * 1024 * 1024.0,
			},
		})
		return

	case len(parts) == 5 && parts[2] == "storage" && parts[4] == "content":
		f.handleStorageContent(w, r)
		return

	case len(parts) == 3 && parts[2] == "vzdump" && r.Method == http.MethodPost:
		f.handleVzdump(w, r)
		return

	case len(parts) >= 4 && parts[2] == "qemu":
		f.handleQemu(w, r, parts)
		return
	}
	http.NotFound(w, r)
}

func (f *apiFixture) handleQemu(w http.ResponseWriter, r *http.Request, parts []string) {
	vmid := parts[3]

	switch {
	case len(parts) == 6 && parts[4] == "status" && parts[5] == "current" && r.Method == http.MethodGet:
		f.mu.Lock()
		vm := f.vms[vmid]
		var status, name string
		if vm != nil {
			status, name = vm.Status, vm.Name
		}
		f.mu.Unlock()
		if vm == nil {
			f.apiError(w, "no such vm")
			return
		}
		id, _ := strconv.ParseFloat(vmid, 64)
		f.writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{"vmid": id, "name": name, "status": status, "qmpstatus": status},
		})

	case len(parts) == 6 && parts[4] == "status" && r.Method == http.MethodPost: // power ops
		action := parts[5]
		f.mu.Lock()
		if hook := f.onStartHook; hook != nil && action == "start" {
			f.mu.Unlock()
			hook(vmid)
			f.mu.Lock()
		}
		if f.startDelay > 0 && action == "start" {
			time.Sleep(f.startDelay)
		}
		f.startCalls[vmid]++
		fail := (action == "start" && f.failStart[vmid]) || (action == "shutdown" && f.failShutdown[vmid])
		if action == "shutdown" {
			f.shutdownCalls[vmid]++
		}
		if !fail {
			switch action {
			case "start", "resume", "reset", "reboot":
				f.vms[vmid].Status = "running"
			case "stop", "shutdown", "suspend":
				f.vms[vmid].Status = "stopped"
			}
		}
		f.mu.Unlock()
		if fail {
			f.apiError(w, "power operation failed")
			return
		}
		f.writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:" + action + ":" + vmid + ":root@pam:"})

	case len(parts) == 5 && parts[4] == "snapshot" && r.Method == http.MethodGet:
		f.mu.Lock()
		vm := f.vms[vmid]
		f.mu.Unlock()
		if vm == nil {
			f.apiError(w, "no such vm")
			return
		}
		items := []map[string]interface{}{}
		f.mu.Lock()
		for _, s := range vm.Snaps {
			items = append(items, map[string]interface{}{"name": s, "snaptime": 1700000000.0})
		}
		f.mu.Unlock()
		f.writeJSON(w, map[string]interface{}{"data": items})

	case len(parts) == 5 && parts[4] == "snapshot" && r.Method == http.MethodPost:
		f.writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:qmsnapshot:" + vmid + ":root@pam:"})

	case len(parts) == 6 && parts[4] == "snapshot" && r.Method == http.MethodDelete:
		name := parts[5]
		f.mu.Lock()
		f.snapDeletes[vmid]++
		fail := f.failSnapDel[vmid+":"+name]
		if !fail {
			vm := f.vms[vmid]
			var kept []string
			for _, s := range vm.Snaps {
				if s != name {
					kept = append(kept, s)
				}
			}
			vm.Snaps = kept
		}
		f.mu.Unlock()
		if fail {
			f.apiError(w, "snapshot deletion failed")
			return
		}
		f.writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:qmdelsnapshot:" + vmid + ":root@pam:"})

	default:
		http.NotFound(w, r)
	}
}

func (f *apiFixture) handleStorageContent(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Empty backup list is fine: creation prints nothing extra, listing 0.
		f.writeJSON(w, map[string]interface{}{"data": []map[string]interface{}{}})
		return
	}
	if r.Method == http.MethodDelete {
		f.writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:vzdump-remove:root@pam:"})
		return
	}
	http.NotFound(w, r)
}

func (f *apiFixture) handleVzdump(w http.ResponseWriter, r *http.Request) {
	vmid := r.FormValue("vmid")
	if vmid == "" {
		// client.Post sends map bodies as JSON, not form data
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["vmid"].(string); ok {
			vmid = v
		}
	}
	f.mu.Lock()
	f.vzdumpCalls[vmid]++
	fail := f.failVzdump[vmid]
	f.mu.Unlock()
	if fail {
		f.apiError(w, "backup failed")
		return
	}
	f.writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:vzdump:" + vmid + ":root@pam:"})
}

// --- CLI execution plumbing -------------------------------------------------

// resetCommandFlags restores every flag in the command tree to its default.
// cobra flag values persist across Execute() calls; without this, a --vmid set
// by one test leaks into the next.
func resetCommandFlags(cmd *cobra.Command) {
	reset := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(fl *pflag.Flag) {
			// Slice flags append on the second and later Set() calls, so resetting
			// them with DefValue ("[]") injects a literal "[]" token on the next
			// parse. Replace() writes the value directly instead.
			if sv, ok := fl.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = fl.Value.Set(fl.DefValue)
			}
			fl.Changed = false
		})
	}
	reset(cmd.Flags())
	reset(cmd.PersistentFlags())
	for _, sub := range cmd.Commands() {
		resetCommandFlags(sub)
	}
}

// execCLI runs the real command tree against the fixture: env-driven
// initializeApp (PVE_HOST/PVE_PORT point at the fixture), full flag parsing,
// the actual RunE handler. It returns captured stdout (logrus writes there
// too) and the RunE error — which is the process exit contract.
//
// The package globals that initializeApp overwrites are snapshot and restored
// via t.Cleanup; do NOT call t.Parallel in tests using this.
func execCLI(t *testing.T, f *apiFixture, args ...string) (string, error) {
	t.Helper()

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(f.srv.URL, "https://"))
	if err != nil {
		t.Fatalf("split fixture URL: %v", err)
	}
	t.Setenv("PVE_HOST", host)
	t.Setenv("PVE_PORT", portStr)
	t.Setenv("PVE_USER", "root@pam")
	t.Setenv("PVE_PASSWORD", "")
	t.Setenv("PVE_TOKEN_NAME", "test")
	t.Setenv("PVE_TOKEN_VALUE", "secret")
	t.Setenv("PVE_VERIFY_SSL", "false") // fixture serves a self-signed cert; pin explicitly so tests are independent of the default
	t.Setenv("PVE_TASK_TIMEOUT", "")

	// Snapshot the globals initializeApp will overwrite.
	savedClient, savedVM, savedSnap, savedBulk, savedCfg, savedLogger := client, vmOps, snapOps, bulkMgr, cfg, logger
	t.Cleanup(func() {
		client, vmOps, snapOps, bulkMgr, cfg, logger = savedClient, savedVM, savedSnap, savedBulk, savedCfg, savedLogger
	})

	resetCommandFlags(rootCmd)
	rootCmd.SetArgs(args)

	// Capture stdout BEFORE Execute: SetupLogger (inside initializeApp) binds
	// the logger to the current os.Stdout, so handler and log output are both
	// captured.
	oldStdout := os.Stdout
	r, w, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("pipe: %v", errPipe)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	execErr := rootCmd.Execute()

	_ = w.Close()
	os.Stdout = oldStdout
	out := <-done
	return out, execErr
}

// execCLIAsync is execCLI for tests that must act mid-execution (cancellation):
// it returns immediately with a channel for the (stdout, error) result. The
// fixture's onStartHook is the synchronized way to reach into the running
// command — see the cancellation golden test.
func execCLIAsync(t *testing.T, f *apiFixture, args ...string) <-chan execResult {
	t.Helper()
	res := make(chan execResult, 1)
	go func() {
		out, err := execCLI(t, f, args...)
		res <- execResult{stdout: out, err: err}
	}()
	return res
}

type execResult struct {
	stdout string
	err    error
}
