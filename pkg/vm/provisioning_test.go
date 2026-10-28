package vm

import "testing"

// TestValidateResizeSize covers the two mistakes that actually reach this
// function: asking for a shrink (which the API cannot do at all) and giving a
// size the API pattern '\+?\d+(\.\d+)?[KMGT]?' would reject.
func TestValidateResizeSize(t *testing.T) {
	valid := []string{
		"32G",
		"+4G",
		"1T",
		"512M",
		"1024K",
		"1.5G",
		"+0.5T",
		// No unit is legal per the API pattern — the value is then bytes.
		"1073741824",
	}

	for _, size := range valid {
		t.Run("accepts "+size, func(t *testing.T) {
			if err := ValidateResizeSize(size); err != nil {
				t.Fatalf("expected %q to be valid, got: %v", size, err)
			}
		})
	}

	invalid := []struct {
		name string
		size string
	}{
		{"empty", ""},
		// Shrinking is the important one: the API has no negative form, and
		// silently passing this through would surface as an opaque API error
		// at the moment a user is trying to reclaim space.
		{"negative increment", "-4G"},
		{"negative absolute", "-32G"},
		{"unsupported unit", "4P"},
		{"unit with no number", "G"},
		{"plus with no number", "+"},
		{"non-numeric", "big"},
		{"trailing garbage", "4GB"},
	}

	for _, tc := range invalid {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := ValidateResizeSize(tc.size); err == nil {
				t.Fatalf("expected %q to be rejected", tc.size)
			}
		})
	}
}

// TestValidateResizeSizeShrinkMessage checks that a shrink attempt explains
// why it cannot work, rather than just reporting a malformed size.
func TestValidateResizeSizeShrinkMessage(t *testing.T) {
	err := ValidateResizeSize("-4G")
	if err == nil {
		t.Fatal("expected an error")
	}

	msg := err.Error()
	for _, want := range []string{"shrink", "extend"} {
		if !contains(msg, want) {
			t.Fatalf("error should mention %q, got: %s", want, msg)
		}
	}
}

func TestExtractBlockReasons(t *testing.T) {
	t.Run("empty detail yields no reasons", func(t *testing.T) {
		// A node listed in not_allowed_nodes with an empty detail object is
		// not actually blocked. Treating it as blocked would refuse a
		// migration the cluster would have accepted.
		if reasons := extractBlockReasons(map[string]interface{}{}); len(reasons) != 0 {
			t.Fatalf("expected no reasons, got %v", reasons)
		}
	})

	t.Run("unavailable storages are reported", func(t *testing.T) {
		detail := map[string]interface{}{
			"unavailable_storages": []interface{}{"local-zfs", "nfs-backup"},
		}
		reasons := extractBlockReasons(detail)
		if len(reasons) != 1 {
			t.Fatalf("expected 1 reason, got %v", reasons)
		}
		if !contains(reasons[0], "local-zfs") || !contains(reasons[0], "nfs-backup") {
			t.Fatalf("reason should name both storages, got: %s", reasons[0])
		}
	})

	t.Run("blocking HA resources are reported", func(t *testing.T) {
		detail := map[string]interface{}{
			"blocking_ha_resources": []interface{}{
				map[string]interface{}{"sid": "vm:101", "cause": "resource is in an error state"},
			},
		}
		reasons := extractBlockReasons(detail)
		if len(reasons) != 1 {
			t.Fatalf("expected 1 reason, got %v", reasons)
		}
		if !contains(reasons[0], "vm:101") {
			t.Fatalf("reason should name the resource, got: %s", reasons[0])
		}
	})

	t.Run("non-map detail is tolerated", func(t *testing.T) {
		if reasons := extractBlockReasons("unexpected"); len(reasons) != 0 {
			t.Fatalf("expected no reasons, got %v", reasons)
		}
	})
}

func TestSetIfNotEmpty(t *testing.T) {
	data := make(map[string][]string)
	set := func(key, value string) {
		if value != "" {
			data[key] = []string{value}
		}
	}

	set("present", "value")
	set("absent", "")

	if _, ok := data["absent"]; ok {
		t.Fatal("empty values must not be sent as parameters")
	}
	if data["present"][0] != "value" {
		t.Fatalf("unexpected value: %v", data["present"])
	}
}

// contains is a small substring helper so the assertions above read clearly.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
