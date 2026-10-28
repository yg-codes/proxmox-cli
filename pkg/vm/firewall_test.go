package vm

import "testing"

// TestValidateFirewallAction guards the API's actual contract, which is a name
// PATTERN and not an enum: ACCEPT/DROP/REJECT and security group names are all
// valid. An earlier version of this code restricted non-group rules to the three
// policies, which would have rejected legitimate security-group rules.
func TestValidateFirewallAction(t *testing.T) {
	valid := []string{
		"ACCEPT", "DROP", "REJECT",
		// Security group names are equally valid per the API pattern.
		"web-servers", "db_tier", "grp1", "Mixed-Case_9",
	}
	for _, a := range valid {
		t.Run("accepts "+a, func(t *testing.T) {
			if err := validateFirewallAction(a); err != nil {
				t.Fatalf("%q matches the API pattern and must be accepted: %v", a, err)
			}
		})
	}

	invalid := []struct {
		name   string
		action string
	}{
		{"empty", ""},
		{"single character", "A"},
		{"leading digit", "1group"},
		{"leading dash", "-group"},
		{"contains space", "web servers"},
		{"contains dot", "web.servers"},
		{"contains slash", "web/servers"},
		{"too long", "abcdefghijklmnopqrstuvwxyz"},
	}
	for _, tc := range invalid {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := validateFirewallAction(tc.action); err == nil {
				t.Fatalf("%q violates the API pattern and must be rejected", tc.action)
			}
		})
	}
}

func TestValidateFirewallRule(t *testing.T) {
	t.Run("minimal valid rule", func(t *testing.T) {
		r := &FirewallRule{Type: "in", Action: "ACCEPT"}
		if err := ValidateFirewallRule(r); err != nil {
			t.Fatalf("type+action is the minimum valid rule: %v", err)
		}
	})

	t.Run("group rule with a security group name", func(t *testing.T) {
		r := &FirewallRule{Type: "group", Action: "web-servers"}
		if err := ValidateFirewallRule(r); err != nil {
			t.Fatalf("a group rule names its security group in action: %v", err)
		}
	})

	for _, typ := range []string{"in", "out", "forward", "group"} {
		t.Run("accepts type "+typ, func(t *testing.T) {
			r := &FirewallRule{Type: typ, Action: "ACCEPT"}
			if err := ValidateFirewallRule(r); err != nil {
				t.Fatalf("%q is a valid rule type: %v", typ, err)
			}
		})
	}

	t.Run("unknown type is rejected", func(t *testing.T) {
		r := &FirewallRule{Type: "sideways", Action: "ACCEPT"}
		if err := ValidateFirewallRule(r); err == nil {
			t.Fatal("expected an error for an unknown rule type")
		}
	})

	t.Run("missing type is rejected", func(t *testing.T) {
		r := &FirewallRule{Action: "ACCEPT"}
		if err := ValidateFirewallRule(r); err == nil {
			t.Fatal("type is required")
		}
	})

	t.Run("missing action is rejected", func(t *testing.T) {
		r := &FirewallRule{Type: "in"}
		if err := ValidateFirewallRule(r); err == nil {
			t.Fatal("action is required")
		}
	})

	for _, level := range []string{"emerg", "alert", "crit", "err", "warning",
		"notice", "info", "debug", "nolog"} {
		t.Run("accepts log level "+level, func(t *testing.T) {
			r := &FirewallRule{Type: "in", Action: "ACCEPT", Log: level}
			if err := ValidateFirewallRule(r); err != nil {
				t.Fatalf("%q is a valid log level: %v", level, err)
			}
		})
	}

	t.Run("unknown log level is rejected", func(t *testing.T) {
		r := &FirewallRule{Type: "in", Action: "ACCEPT", Log: "verbose"}
		if err := ValidateFirewallRule(r); err == nil {
			t.Fatal("expected an error for an unknown log level")
		}
	})

	t.Run("enable outside 0/1 is rejected", func(t *testing.T) {
		r := &FirewallRule{Type: "in", Action: "ACCEPT", Enable: 2}
		if err := ValidateFirewallRule(r); err == nil {
			t.Fatal("enable must be 0 or 1")
		}
	})

	t.Run("nil rule is rejected", func(t *testing.T) {
		if err := ValidateFirewallRule(nil); err == nil {
			t.Fatal("a nil rule must be rejected rather than panicking")
		}
	})
}

// TestHasRuleEdits backs the guard that refuses --move-to alongside field
// edits. The API ignores every other argument when moving, so a missed edit
// would be silently discarded rather than reported.
func TestHasRuleEdits(t *testing.T) {
	if hasRuleEdits(nil) {
		t.Error("a nil rule carries no edits")
	}
	if hasRuleEdits(&FirewallRule{}) {
		t.Error("an empty rule carries no edits")
	}
	// Digest alone is not an edit — it is concurrency protection, and is sent
	// with a move.
	if hasRuleEdits(&FirewallRule{Digest: "abc123"}) {
		t.Error("a digest is not a field edit")
	}
	// Pos likewise identifies the rule rather than changing it.
	if hasRuleEdits(&FirewallRule{Pos: 3}) {
		t.Error("pos identifies the rule and is not an edit")
	}

	for name, rule := range map[string]*FirewallRule{
		"comment": {Comment: "x"},
		"action":  {Action: "DROP"},
		"type":    {Type: "out"},
		"dport":   {DPort: "80"},
		"source":  {Source: "192.0.2.0/8"},
		"log":     {Log: "info"},
		"enable":  {Enable: 1},
	} {
		t.Run(name+" counts as an edit", func(t *testing.T) {
			if !hasRuleEdits(rule) {
				t.Fatalf("a changed %s must count as an edit", name)
			}
		})
	}
}

// TestBoolPtrFrom covers the three shapes Proxmox uses for a boolean, plus the
// absent case. Collapsing "unset" into "false" would make a firewall option
// look explicitly disabled when it is merely at its default.
func TestBoolPtrFrom(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want *bool
	}{
		{"float 1", float64(1), boolPtr(true)},
		{"float 0", float64(0), boolPtr(false)},
		{"bool true", true, boolPtr(true)},
		{"bool false", false, boolPtr(false)},
		{"string 1", "1", boolPtr(true)},
		{"string 0", "0", boolPtr(false)},
		{"string true", "true", boolPtr(true)},
		{"absent stays nil", nil, nil},
		{"unexpected type stays nil", []string{"x"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := boolPtrFrom(tc.in)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("expected nil for %v, got %v", tc.in, *got)
			case tc.want != nil && got == nil:
				t.Fatalf("expected %v for %v, got nil", *tc.want, tc.in)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("expected %v for %v, got %v", *tc.want, tc.in, *got)
			}
		})
	}
}

func TestFirewallEnumAccessorsReturnCopies(t *testing.T) {
	types := FirewallRuleTypes()
	types[0] = "mutated"
	if again := FirewallRuleTypes(); again[0] != "in" {
		t.Fatalf("FirewallRuleTypes must return a copy, got %q", again[0])
	}

	if len(FirewallLogLevels()) != 9 {
		t.Fatalf("expected 9 log levels, got %d", len(FirewallLogLevels()))
	}
	if len(FirewallPolicies()) != 3 {
		t.Fatalf("expected 3 policies, got %d", len(FirewallPolicies()))
	}
}

func boolPtr(b bool) *bool { return &b }
