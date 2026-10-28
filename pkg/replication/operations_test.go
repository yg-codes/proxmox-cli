package replication

import "testing"

// TestValidateJobID covers the API's own id pattern, [1-9][0-9]{2,8}-\d{1,9}.
// The constraint is not obvious — the VMID part needs at least three digits, so
// a two-digit VMID cannot have a replication job at all — and the API's regex
// rejection names neither the rule nor which half was wrong.
func TestValidateJobID(t *testing.T) {
	valid := []string{
		"100-0",
		"100-1",
		"4001-0",
		"999999999-0",
		"100-999999999",
	}
	for _, id := range valid {
		t.Run("accepts "+id, func(t *testing.T) {
			if err := ValidateJobID(id); err != nil {
				t.Fatalf("%q should be a valid job ID: %v", id, err)
			}
		})
	}

	invalid := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"no separator", "100"},
		{"non-numeric", "abc"},
		// The API pattern requires 3-9 digits for the VMID, so a valid VM with
		// a two-digit id still cannot be replicated.
		{"two-digit vmid", "99-0"},
		{"one-digit vmid", "9-0"},
		{"leading zero", "0100-0"},
		{"missing job number", "100-"},
		{"missing vmid", "-0"},
		{"letters in vmid", "10a-0"},
		{"letters in job number", "100-a"},
		{"vmid too long", "1234567890-0"},
		{"job number too long", "100-1234567890"},
	}
	for _, tc := range invalid {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := ValidateJobID(tc.id); err == nil {
				t.Fatalf("%q must be rejected", tc.id)
			}
		})
	}
}

// TestValidateJobIDMessageDoesNotEchoBadInput guards a small but real trap: an
// error that builds its example from the rejected input suggests something
// equally invalid, so the operator retries and fails again.
func TestValidateJobIDMessageDoesNotEchoBadInput(t *testing.T) {
	err := ValidateJobID("abc")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !contains(got, "100-0") || contains(got, "abc-0") {
		t.Fatalf("the example should be a concrete valid id, not derived from the input: %s", got)
	}
}

// TestJobFromMap checks the three shapes Proxmox uses for a boolean, since the
// same field arrives as a number, a bool or a string depending on how it was
// set. Reading only one of them would report a disabled job as enabled.
func TestJobFromMap(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
		want bool
	}{
		{"float 1", float64(1), true},
		{"float 0", float64(0), false},
		{"bool true", true, true},
		{"bool false", false, false},
		{"string 1", "1", true},
		{"string 0", "0", false},
		{"absent", nil, false},
	}

	for _, tc := range cases {
		t.Run("disable as "+tc.name, func(t *testing.T) {
			m := map[string]interface{}{"id": "100-0", "target": "pve2"}
			if tc.raw != nil {
				m["disable"] = tc.raw
			}
			job := jobFromMap(m)
			if job.Disable != tc.want {
				t.Fatalf("disable=%v (%T): got %v, want %v", tc.raw, tc.raw, job.Disable, tc.want)
			}
			if job.ID != "100-0" || job.Target != "pve2" {
				t.Fatalf("unexpected job fields: %+v", job)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
