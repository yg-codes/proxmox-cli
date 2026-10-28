package backupjob

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/yg-codes/proxmox-cli/pkg/api"
)

func TestValidateVMIDList(t *testing.T) {
	valid := []string{
		"4001",
		"4001,4002",
		"5001",
	}
	for _, s := range valid {
		if err := ValidateVMIDList(s); err != nil {
			t.Errorf("ValidateVMIDList(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"",             // empty
		"4001,",        // trailing comma
		",4001",        // leading comma
		"4001,,4002",   // double comma
		"rocky10-vm01", // a VM *name* — silently selects nothing
		"7201-7205",    // a range — not valid pve-vmid-list syntax
		"0",            // non-positive
		"-1",           // negative (also not a list element)
		"4001 4002",    // space-separated
	}
	for _, s := range invalid {
		if err := ValidateVMIDList(s); err == nil {
			t.Errorf("ValidateVMIDList(%q) = nil, want an error", s)
		}
	}

	// The hyphenated-name case is the dangerous one: unlike CLI selectors,
	// this value is stored in jobs.cfg and expanded at job runtime, so a name
	// here fails silently rather than loudly. The message must say names are
	// the problem.
	err := ValidateVMIDList("rocky10-vm01")
	if err == nil || !strings.Contains(err.Error(), "not names") {
		t.Fatalf("error for a VM name must explain that names are invalid: %v", err)
	}
}

func TestValidateSelection(t *testing.T) {
	cases := []struct {
		name    string
		job     Job
		wantErr bool
	}{
		{"all", Job{All: true}, false},
		{"vmid", Job{VMID: "4001"}, false},
		{"pool", Job{Pool: "dev"}, false},
		{"all with exclude", Job{All: true, Exclude: "4002"}, false},
		{"none", Job{}, true},
		{"all and vmid", Job{All: true, VMID: "4001"}, true},
		{"vmid and pool", Job{VMID: "4001", Pool: "dev"}, true},
		{"all three", Job{All: true, VMID: "4001", Pool: "dev"}, true},
		{"exclude without all", Job{VMID: "4001", Exclude: "4002"}, true},
		{"vmid is a name", Job{VMID: "web01"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSelection(&tc.job)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateJobID(t *testing.T) {
	valid := []string{"weekly-vms", "a", "job_2", "nightly-prod-1"}
	for _, id := range valid {
		if err := ValidateJobID(id); err != nil {
			t.Errorf("ValidateJobID(%q) = %v, want nil", id, err)
		}
	}

	invalid := map[string]string{
		"":           "empty",
		"1abc":       "starts with a digit",
		"-weekly":    "starts with a hyphen",
		"weekly vms": "contains a space",
		"jöver":      "non-ASCII",
	}
	for id, why := range invalid {
		if err := ValidateJobID(id); err == nil {
			t.Errorf("ValidateJobID(%q) = nil, want an error (%s)", id, why)
		}
	}

	// 41 chars.
	long := strings.Repeat("a", 41)
	if err := ValidateJobID(long); err == nil {
		t.Error("ValidateJobID(41 chars) = nil, want an error")
	}
	if err := ValidateJobID(strings.Repeat("a", 40)); err != nil {
		t.Errorf("ValidateJobID(40 chars) = %v, want nil", err)
	}
}

// TestApplyJobFieldsFormValues pins the exact wire form, including the two
// decisions most likely to be "fixed" into bugs later: the enabled tri-state
// and the deliberate absence of a digest.
func TestApplyJobFieldsFormValues(t *testing.T) {
	t.Run("enabled tri-state", func(t *testing.T) {
		var data url.Values

		data = url.Values{}
		applyJobFields(data, &Job{})
		if _, present := data["enabled"]; present {
			t.Error("unset Enabled must not send an enabled key (server default applies)")
		}

		yes := true
		data = url.Values{}
		applyJobFields(data, &Job{Enabled: &yes})
		if got := data.Get("enabled"); got != "1" {
			t.Errorf("enabled=true sent %q, want \"1\"", got)
		}

		no := false
		data = url.Values{}
		applyJobFields(data, &Job{Enabled: &no})
		if got := data.Get("enabled"); got != "0" {
			t.Errorf("enabled=false sent %q, want \"0\"", got)
		}
	})

	t.Run("exclude-path is repeated, not comma-joined", func(t *testing.T) {
		data := url.Values{}
		applyJobFields(data, &Job{ExcludePath: []string{"/var/log", "/tmp/*"}})
		got := data["exclude-path"]
		if len(got) != 2 || got[0] != "/var/log" || got[1] != "/tmp/*" {
			t.Fatalf("exclude-path sent %q, want two repeated keys", got)
		}
	})

	t.Run("no digest is ever fabricated", func(t *testing.T) {
		data := url.Values{}
		yes := true
		applyJobFields(data, &Job{ID: "x", Schedule: "* *", Storage: "s", Enabled: &yes})
		if _, present := data["digest"]; present {
			t.Fatal("applyJobFields sent a digest: no backup-job endpoint accepts one " +
				"(jobs.cfg uses cfs locking, not optimistic locking)")
		}
	})

	t.Run("full create payload", func(t *testing.T) {
		no := false
		data := url.Values{}
		applyJobFields(data, &Job{
			ID:           "weekly-vms",
			All:          true,
			Exclude:      "4002",
			Storage:      "local",
			Schedule:     "sat 02:30",
			Enabled:      &no,
			Mode:         "snapshot",
			Compress:     "zstd",
			Comment:      "weekly",
			PruneBackups: "keep-last=7",
			BWLimit:      102400,
		})
		want := url.Values{
			"id":            {"weekly-vms"},
			"all":           {"1"},
			"exclude":       {"4002"},
			"storage":       {"local"},
			"schedule":      {"sat 02:30"},
			"enabled":       {"0"},
			"mode":          {"snapshot"},
			"compress":      {"zstd"},
			"comment":       {"weekly"},
			"prune-backups": {"keep-last=7"},
			"bwlimit":       {"102400"},
		}
		for k, vs := range want {
			if got := data[k]; !equalStrings(got, vs) {
				t.Errorf("field %q = %q, want %q", k, got, vs)
			}
		}
		if len(data) != len(want) {
			t.Errorf("payload has %d keys, want %d: %v", len(data), len(want), data)
		}
	})
}

func TestUpdateJobRejectsBadDeleteKeys(t *testing.T) {
	// Validate the guard directly; Operations.UpdateJob needs a live client.
	for _, key := range []string{"id", "schedule"} {
		if err := validateDeletableKey(key); err == nil {
			t.Errorf("validateDeletableKey(%q) = nil, want an error", key)
		}
	}
	for _, key := range []string{"bwlimit", "protected", "prune-backups", "mailto"} {
		if err := validateDeletableKey(key); err != nil {
			t.Errorf("validateDeletableKey(%q) = %v, want nil", key, err)
		}
	}
	if err := validateDeletableKey(""); err == nil {
		t.Error("validateDeletableKey(\"\") = nil, want an error")
	}
}

func TestJobFromMap(t *testing.T) {
	m := map[string]interface{}{
		"id":                        "weekly-vms",
		"type":                      "vzdump",
		"schedule":                  "sat 02:30",
		"enabled":                   0, // numeric false, as the API sends it
		"all":                       true,
		"exclude":                   "4002",
		"exclude-path":              []interface{}{"/var/log", "/tmp/*"},
		"storage":                   "local",
		"mode":                      "snapshot",
		"compress":                  "zstd",
		"comment":                   "weekly",
		"prune-backups":             "keep-last=7",
		"bwlimit":                   float64(102400),
		"protected":                 1,
		"next-run":                  float64(1760000000),
		"pbs-change-detection-mode": "metadata",
	}
	j := jobFromMap(m)
	if j.ID != "weekly-vms" || j.Schedule != "sat 02:30" {
		t.Fatalf("basic fields wrong: %+v", j)
	}
	if j.Enabled == nil || *j.Enabled {
		t.Fatalf("enabled=0 must parse as pointer-to-false, got %v", j.Enabled)
	}
	if !j.All || !j.Protected {
		t.Error("boolean true fields not parsed")
	}
	if j.NextRun != 1760000000 {
		t.Errorf("next-run = %d, want 1760000000", j.NextRun)
	}
	if len(j.ExcludePath) != 2 || j.ExcludePath[0] != "/var/log" {
		t.Errorf("exclude-path = %q, want [/var/log /tmp/*]", j.ExcludePath)
	}
	if j.BWLimit != 102400 {
		t.Errorf("bwlimit = %d, want 102400", j.BWLimit)
	}
	if j.PBSChangeDet != "metadata" {
		t.Errorf("pbs-change-detection-mode = %q", j.PBSChangeDet)
	}

	// Absent enabled must stay nil so the diff renderer treats it as unknown,
	// and absent next-run must stay 0.
	j2 := jobFromMap(map[string]interface{}{"id": "x"})
	if j2.Enabled != nil {
		t.Errorf("absent enabled must be nil, got %v", *j2.Enabled)
	}
	if j2.NextRun != 0 {
		t.Errorf("absent next-run must be 0, got %d", j2.NextRun)
	}
}

func TestPermHint(t *testing.T) {
	t.Run("403 is annotated", func(t *testing.T) {
		err := permHint("list backup jobs", &api.ProxmoxAPIError{
			Message:    "permission check failed",
			StatusCode: http.StatusForbidden,
		})
		msg := err.Error()
		for _, want := range []string{"Sys.Audit", "Sys.Modify", "PVEAuditor", "PVESysAdmin"} {
			if !strings.Contains(msg, want) {
				t.Errorf("hint missing %q: %s", want, msg)
			}
		}
	})

	t.Run("other errors pass through", func(t *testing.T) {
		sentinel := errors.New("connection refused")
		if err := permHint("list backup jobs", sentinel); err != sentinel {
			t.Errorf("non-403 error was wrapped: %v", err)
		}
	})

	t.Run("non-API 403-equivalent is untouched", func(t *testing.T) {
		sentinel := errors.New("some other failure")
		if err := permHint("x", sentinel); err != sentinel {
			t.Errorf("plain error was wrapped: %v", err)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
