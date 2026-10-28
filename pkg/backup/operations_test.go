package backup

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// writeJSON writes a Proxmox-style envelope response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// testDeletesAttempted counts DELETE requests the fixture received; reset
// by newBackupTestOps, incremented by the content handler.
var testDeletesAttempted int

// newBackupTestOps wires backup.Operations to a local TLS test server hosting
// three backups for VM 100 on storage "local". The DELETE whose volid equals
// failDeleteVolid returns an API error; every other deletion succeeds.
func newBackupTestOps(t *testing.T, failDeleteVolid string) *Operations {
	t.Helper()
	testDeletesAttempted = 0

	const (
		volidJan = "local:backup/vzdump-qemu-100-2024_01_01-12_00_00.vma.zst"
		volidJun = "local:backup/vzdump-qemu-100-2024_06_01-12_00_00.vma.zst"
		volidDec = "local:backup/vzdump-qemu-100-2024_12_01-12_00_00.vma.zst"
	)
	items := []map[string]interface{}{
		{"volid": volidJan, "vmid": float64(100), "ctime": float64(1704067200), "format": "vma.zst", "size": float64(1024 * 1024 * 1024)},
		{"volid": volidJun, "vmid": float64(100), "ctime": float64(1717200000), "format": "vma.zst", "size": float64(1024 * 1024 * 1024)},
		{"volid": volidDec, "vmid": float64(100), "ctime": float64(1733011200), "format": "vma.zst", "size": float64(1024 * 1024 * 1024)},
	}

	taskStatus := regexp.MustCompile(`^/api2/json/nodes/[^/]+/tasks/.+/status$`)
	contentBase := "/api2/json/nodes/pve1/storage/local/content"

	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []map[string]interface{}{{"node": "pve1", "status": "online"}},
		})
	})
	content := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]interface{}{"data": items})
		case http.MethodDelete:
			testDeletesAttempted++
			volid := strings.TrimPrefix(r.URL.Path, contentBase+"/")
			if unescaped, err := url.PathUnescape(volid); err == nil {
				volid = unescaped
			}
			if volid == failDeleteVolid {
				http.Error(w, `{"errors":{"any":"deletion failed"}}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]interface{}{"data": "UPID:pve1:TEST:TEST:0:vzdump:100:root@pam:"})
		default:
			http.NotFound(w, r)
		}
	}
	mux.HandleFunc(contentBase, content)
	mux.HandleFunc(contentBase+"/", content)
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
	return NewOperations(client, vm.NewOperations(client, logger), logger)
}

// Cleanup must attempt every deletion and still return a non-nil error when
// one deletion fails — the count alone must not read as full success.
func TestDeleteOldBackupsPartialFailureReturnsError(t *testing.T) {
	const failVolid = "local:backup/vzdump-qemu-100-2024_06_01-12_00_00.vma.zst"
	ops := newBackupTestOps(t, failVolid)

	// keep-count 1 keeps the newest (Dec); Jan and Jun are eligible for
	// deletion, and Jun's DELETE fails server-side.
	deleted, err := ops.DeleteOldBackups(context.Background(), "100", "local", 1, 0)
	if deleted != 1 {
		t.Errorf("deleted=%d, want 1 (the successful deletion must still count)", deleted)
	}
	if err == nil {
		t.Fatal("DeleteOldBackups with one failed deletion: err=nil, want aggregate error")
	}
	if !strings.Contains(err.Error(), "2024_06_01") {
		t.Errorf("aggregate error %q does not name the failed backup", err)
	}
}

// --- Task 7: documented glob contract for --pattern -------------------------

func TestBackupPatternGlobMatching(t *testing.T) {
	const volid = "local:backup/vzdump-qemu-100-2024_06_01-12_00_00.vma.zst"
	cases := []struct {
		name    string
		pattern string
		want    bool
		wantErr bool
	}{
		{name: "empty pattern matches everything (documented match-all)", pattern: "", want: true},
		{name: "glob across the path matches", pattern: "local:backup/*2024_06*", want: true},
		{name: "star does not cross the storage separator", pattern: "*2024_06*", want: false},
		{name: "exact volid matches", pattern: volid, want: true},
		{name: "single-char wildcard matches", pattern: "local:backup/vzdump-qemu-1?0-*", want: true},
		{name: "non-matching pattern", pattern: "local:backup/*2025*", want: false},
		{name: "malformed character class errors", pattern: "local:backup/vzdump-[", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := backupPatternMatches(tc.pattern, volid)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("pattern %q: err=nil, want bad-pattern error", tc.pattern)
				}
				return
			}
			if err != nil {
				t.Fatalf("pattern %q: unexpected error: %v", tc.pattern, err)
			}
			if got != tc.want {
				t.Errorf("pattern %q: matched=%v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}

// A valid glob must delete exactly the matched backups; a malformed pattern
// must fail before any deletion is attempted.
func TestDeleteBackupsByPatternGlobAndValidation(t *testing.T) {
	t.Run("glob deletes only matches", func(t *testing.T) {
		ops := newBackupTestOps(t, "")
		deleted, err := ops.DeleteBackupsByPattern(context.Background(), "100", "local", "local:backup/*2024_06*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deleted != 1 {
			t.Errorf("deleted=%d, want 1 (only the June backup matches)", deleted)
		}
		if testDeletesAttempted != 1 {
			t.Errorf("deletesAttempted=%d, want 1", testDeletesAttempted)
		}
	})

	t.Run("malformed pattern fails before deleting", func(t *testing.T) {
		ops := newBackupTestOps(t, "")
		deleted, err := ops.DeleteBackupsByPattern(context.Background(), "100", "local", "local:backup/vzdump-[")
		if err == nil {
			t.Fatal("malformed pattern: err=nil, want validation error")
		}
		if !strings.Contains(err.Error(), "pattern") {
			t.Errorf("error %q does not mention the pattern", err)
		}
		if deleted != 0 || testDeletesAttempted != 0 {
			t.Errorf("deleted=%d attempted=%d, want 0/0 (fail before deletion)", deleted, testDeletesAttempted)
		}
	})
}
