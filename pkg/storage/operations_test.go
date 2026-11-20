package storage_test

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
	"github.com/yg-codes/proxmox-cli/pkg/storage"
)

func storageOps(t *testing.T, failPath string, failStatus int, empty bool) *storage.Operations {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == failPath {
			http.Error(w, "fixture API failure", failStatus)
			return
		}
		var data interface{}
		switch r.URL.Path {
		case "/api2/json/nodes":
			data = []map[string]interface{}{{"node": "node-a"}, {"node": "node-b"}}
		case "/api2/json/nodes/node-a/storage", "/api2/json/nodes/node-b/storage":
			data = []map[string]interface{}{}
			if !empty {
				data = []map[string]interface{}{{"storage": "local-fast", "type": "zfspool"}}
			}
		case "/api2/json/nodes/node-a/storage/local-fast/status", "/api2/json/nodes/node-b/storage/local-fast/status":
			avail := int64(10) << 30
			if strings.Contains(r.URL.Path, "/node-b/") {
				avail = int64(1000) << 30
			}
			data = map[string]interface{}{
				"content": "images,rootdir,backup", "active": 1,
				"avail": avail, "total": int64(2000) << 30,
			}
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": data}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	client := api.NewClient(&api.ClientConfig{
		Host: host, Port: port, Username: "fixture@pve", TokenName: "fixture", TokenValue: "fixture",
		CACert: srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, Logger: logger,
	})
	return storage.NewOperations(client, logger)
}

var listings = []struct {
	name    string
	list    func(*storage.Operations) ([]*storage.Storage, error)
	display func(*storage.Operations) error
}{
	{"VM", (*storage.Operations).GetVMStorages, (*storage.Operations).DisplayVMStorages},
	{"backup", (*storage.Operations).GetBackupStorages, (*storage.Operations).DisplayBackupStorages},
}

func TestStorageListingsPropagateAPIErrors(t *testing.T) {
	for _, listing := range listings {
		t.Run(listing.name, func(t *testing.T) {
			for _, path := range []string{
				"/api2/json/nodes/node-b/storage",
				"/api2/json/nodes/node-b/storage/local-fast/status",
			} {
				for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
					t.Run(path+"/"+strconv.Itoa(status), func(t *testing.T) {
						rows, err := listing.list(storageOps(t, path, status, false))
						if err == nil || rows != nil {
							t.Fatalf("partial inventory accepted: rows=%v error=%v", rows, err)
						}
						var apiErr *api.ProxmoxAPIError
						if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
							t.Fatalf("API failure not preserved: %v", err)
						}
						if !strings.Contains(err.Error(), "node-b") {
							t.Fatalf("error lacks node context: %v", err)
						}
						if strings.HasSuffix(path, "/status") && !strings.Contains(err.Error(), "local-fast") {
							t.Fatalf("error lacks storage context: %v", err)
						}
					})
				}
			}
		})
	}
}

func TestStorageListingsReadableInventory(t *testing.T) {
	for _, listing := range listings {
		for _, empty := range []bool{false, true} {
			t.Run(listing.name+"/empty="+strconv.FormatBool(empty), func(t *testing.T) {
				rows, err := listing.list(storageOps(t, "", 0, empty))
				if err != nil {
					t.Fatal(err)
				}
				if empty && len(rows) != 0 || !empty && len(rows) == 0 {
					t.Fatalf("unexpected readable inventory: empty=%v rows=%v", empty, rows)
				}
			})
		}
	}
}

func TestStorageValidationRejectsPartialInventory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		empty  bool
	}{
		{"healthy", 0, false},
		{"denied", http.StatusForbidden, false},
		{"unavailable", http.StatusServiceUnavailable, false},
		{"empty", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failPath := ""
			if tc.status != 0 {
				failPath = "/api2/json/nodes/node-b/storage"
			}
			err := storageOps(t, failPath, tc.status, tc.empty).ValidateStorage("local-fast")
			if tc.status != 0 {
				var apiErr *api.ProxmoxAPIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status || !strings.Contains(err.Error(), "node-b") {
					t.Fatalf("validation hid the failed node: %v", err)
				}
			} else if tc.empty {
				if err == nil || !strings.Contains(err.Error(), "not found") {
					t.Fatalf("expected missing storage error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStorageListingsPreserveNodeScope(t *testing.T) {
	for _, listing := range listings {
		t.Run(listing.name, func(t *testing.T) {
			ops := storageOps(t, "", 0, false)
			rows, err := listing.list(ops)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 {
				t.Fatalf("got %d rows for 2 independent local pools, want 2", len(rows))
			}
			// Keep API node order, including for legacy name-only validators.
			if rows[0].Node != "node-a" || rows[0].AvailableGB != 10 || rows[1].Node != "node-b" || rows[1].AvailableGB != 1000 {
				t.Fatalf("lost node identity, order or free space: %+v %+v", rows[0], rows[1])
			}
			for _, row := range rows {
				if row.Name != "local-fast" {
					t.Fatalf("unexpected storage name: %q", row.Name)
				}
			}
			out, err := os.CreateTemp(t.TempDir(), "storage-table")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = out.Close() })
			func() {
				original := os.Stdout
				os.Stdout = out
				defer func() { os.Stdout = original }()
				err = listing.display(ops)
			}()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := out.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Node", "node-a", "node-b", "10.0 GB", "1000.0 GB"} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("table missing %q:\n%s", want, data)
				}
			}
		})
	}
}
