package resource_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
	"github.com/yg-codes/proxmox-cli/pkg/resource"
)

func capacityOps(t *testing.T, nodes []map[string]interface{}) *resource.Operations {
	t.Helper()
	items := append([]map[string]interface{}{}, nodes...)
	items = append(items, map[string]interface{}{
		"type": "qemu", "node": "node-a", "status": "running", "vmid": 100,
		"maxcpu": 8, "maxmem": int64(32) << 30,
	})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/resources" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": items}); err != nil {
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
	return resource.NewOperations(client, logger)
}

func TestCapacityAnalysisRequiresOnlineHardware(t *testing.T) {
	complete := map[string]interface{}{
		"type": "node", "node": "node-a", "status": "online", "cpu": 0.1,
		"maxcpu": 32, "maxmem": int64(128) << 30,
	}
	for _, tc := range []struct {
		name    string
		nodes   []map[string]interface{}
		wantErr bool
	}{
		{"complete", []map[string]interface{}{complete}, false},
		{"no nodes", nil, true},
		{"missing hardware", []map[string]interface{}{{"type": "node", "node": "node-a", "status": "online"}}, true},
		{"missing CPU capacity", []map[string]interface{}{{"type": "node", "node": "node-a", "status": "online", "cpu": 0.1, "maxmem": int64(128) << 30}}, true},
		{"missing RAM capacity", []map[string]interface{}{{"type": "node", "node": "node-a", "status": "online", "cpu": 0.1, "maxcpu": 32}}, true},
		{"zero hardware", []map[string]interface{}{{"type": "node", "node": "node-a", "status": "online", "cpu": 0.0, "maxcpu": 0, "maxmem": 0}}, true},
		{"CPU load absent", []map[string]interface{}{{"type": "node", "node": "node-a", "status": "online", "maxcpu": 32, "maxmem": int64(128) << 30}}, false},
		{"offline hardware absent", []map[string]interface{}{complete, {"type": "node", "node": "node-b", "status": "offline"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := capacityOps(t, tc.nodes).GetCapacityAnalysis()
			if tc.wantErr {
				if err == nil || got != nil {
					t.Fatalf("incomplete capacity returned analysis=%+v error=%v", got, err)
				}
				if !strings.Contains(err.Error(), "capacity") {
					t.Fatalf("error lacks capacity context: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.TotalPhysCPU != 32 || got.TotalPhysMemory != int64(128)<<30 {
				t.Fatalf("hardware: got %d CPUs/%d bytes, want 32 CPUs/128 GiB", got.TotalPhysCPU, got.TotalPhysMemory)
			}
			if got.TotalAllocVCPU != 8 || got.TotalAllocMemory != int64(32)<<30 || got.ClusterCPUOC != 0.25 || got.ClusterMemOC != 0.25 {
				t.Fatalf("allocation/ratios differ from the fixture: %+v", got)
			}
			if tc.name == "offline hardware absent" && (!got.N1Degraded || got.OfflineNodes != 1) {
				t.Fatalf("offline node not recorded: %+v", got)
			}
		})
	}
}
