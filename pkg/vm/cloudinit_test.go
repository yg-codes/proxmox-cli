package vm

import "testing"

// TestValidateCloudInitDumpType pins the enum the API accepts. Verified against
// a live cluster: any other value is rejected server-side with an enum error, so
// checking locally is what turns a typo into a usable message.
func TestValidateCloudInitDumpType(t *testing.T) {
	for _, valid := range []string{"user", "network", "meta"} {
		t.Run("accepts "+valid, func(t *testing.T) {
			if err := ValidateCloudInitDumpType(valid); err != nil {
				t.Fatalf("%q is a valid dump type: %v", valid, err)
			}
		})
	}

	for _, invalid := range []string{"", "bogus", "User", "USER", "vendor", "userdata"} {
		t.Run("rejects "+invalid, func(t *testing.T) {
			err := ValidateCloudInitDumpType(invalid)
			if err == nil {
				t.Fatalf("%q must be rejected", invalid)
			}
			// The message must name the alternatives, or the user has to go
			// looking for them.
			if !containsAll(err.Error(), "user", "network", "meta") {
				t.Fatalf("error should list the valid types, got: %s", err)
			}
		})
	}
}

// TestIsCloudInitKey guards which config keys the CLI treats as cloudinit.
//
// Getting this wrong is quietly misleading in both directions: a missed key
// makes 'cloudinit pending' claim a VM has no cloudinit config when it does,
// and a false positive lists an unrelated setting as cloudinit's.
func TestIsCloudInitKey(t *testing.T) {
	cloudInit := []string{
		"ciuser", "cipassword", "citype", "ciupgrade",
		"sshkeys", "nameserver", "searchdomain",
		// ipconfigN is matched by prefix because the index is unbounded.
		"ipconfig0", "ipconfig1", "ipconfig31",
	}
	for _, key := range cloudInit {
		t.Run(key+" is cloudinit", func(t *testing.T) {
			if !IsCloudInitKey(key) {
				t.Fatalf("%q feeds the cloudinit drive and must be recognised", key)
			}
		})
	}

	notCloudInit := []string{
		"cores", "memory", "scsi0", "net0", "name", "boot", "bios",
		"agent", "description", "protection", "template", "efidisk0",
		// A near-miss: "cpu" starts with 'c' but has nothing to do with cloudinit.
		"cpu", "cdrom",
	}
	for _, key := range notCloudInit {
		t.Run(key+" is not cloudinit", func(t *testing.T) {
			if IsCloudInitKey(key) {
				t.Fatalf("%q is not a cloudinit setting and must not be listed as one", key)
			}
		})
	}
}

func TestCloudInitDumpTypesIsACopy(t *testing.T) {
	first := CloudInitDumpTypes()
	if len(first) != 3 {
		t.Fatalf("expected 3 dump types, got %d", len(first))
	}
	// Mutating the returned slice must not corrupt the package's own list.
	first[0] = "mutated"
	if second := CloudInitDumpTypes(); second[0] != "user" {
		t.Fatalf("CloudInitDumpTypes must return a copy; got %q after mutation", second[0])
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for i := 0; i+len(n) <= len(haystack); i++ {
			if haystack[i:i+len(n)] == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
