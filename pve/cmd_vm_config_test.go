package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestConfigSetFlagsPreserveValues guards the flag layer, not just the parser.
//
// --set was originally declared as StringSlice, which parses its value as CSV.
// That silently corrupted the two most common kinds of Proxmox config value:
// a comma-separated device spec was split into several broken entries, and a
// multi-line description was truncated at the first newline. parseKeyValuePairs
// was correct throughout — the damage happened before it was ever called, so a
// test that only exercises the parser cannot catch a regression here.
func TestConfigSetFlagsPreserveValues(t *testing.T) {
	// Read the flag type off the real command rather than re-registering it,
	// so the test tracks whatever initVMConfigFlags declares.
	newFlagSet := func(t *testing.T) *pflag.FlagSet {
		t.Helper()
		fs := pflag.NewFlagSet("config-set", pflag.ContinueOnError)
		setFlag := vmConfigSetCmd.Flags().Lookup("set")
		if setFlag == nil {
			t.Fatal("--set flag is not registered on vmConfigSetCmd")
		}
		fs.AddFlag(setFlag)
		return fs
	}

	t.Run("comma-separated value stays one entry", func(t *testing.T) {
		flags := newFlagSet(t)
		if err := flags.Parse([]string{"--set", "net0=virtio,bridge=vmbr0,tag=973"}); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		sets, err := flags.GetStringArray("set")
		if err != nil {
			t.Fatalf("GetStringArray failed: %v", err)
		}
		if len(sets) != 1 {
			t.Fatalf("value was split into %d entries: %q", len(sets), sets)
		}

		settings, err := parseKeyValuePairs(sets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "virtio,bridge=vmbr0,tag=973"
		if settings["net0"] != want {
			t.Fatalf("got %q, want %q", settings["net0"], want)
		}
	})

	t.Run("multi-line value is not truncated", func(t *testing.T) {
		flags := newFlagSet(t)
		desc := "### TEMPLATE INFO \nRocky 9.5  \nkernel-5.14.0\n2025-07-08"
		if err := flags.Parse([]string{"--set", "description=" + desc}); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		sets, err := flags.GetStringArray("set")
		if err != nil {
			t.Fatalf("GetStringArray failed: %v", err)
		}

		settings, err := parseKeyValuePairs(sets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := settings["description"]
		if got != desc {
			t.Fatalf("description was altered:\n got %q (%d bytes)\nwant %q (%d bytes)",
				got, len(got), desc, len(desc))
		}
		if strings.Count(got, "\n") != 3 {
			t.Fatalf("expected 3 newlines, got %d", strings.Count(got, "\n"))
		}
	})
}

// TestConfigFlagTypes pins the flag types on all three config subcommands.
//
// Two opposite regressions are guarded here at once:
//
//   - --vmid/--vmname must be StringSlice, or the documented selector syntax
//     (4103,4105) silently resolves as a single literal VM name and fails.
//   - --set/--delete must stay StringArray. A slice flag CSV-parses its value,
//     which is the shipped bug TestConfigSetFlagsPreserveValues exists for.
func TestConfigFlagTypes(t *testing.T) {
	cases := []struct {
		cmd  *cobra.Command
		flag string
		want string
	}{
		{vmConfigGetCmd, "vmid", "stringSlice"},
		{vmConfigGetCmd, "vmname", "stringSlice"},
		{vmConfigSetCmd, "vmid", "stringSlice"},
		{vmConfigSetCmd, "vmname", "stringSlice"},
		{vmConfigPendingCmd, "vmid", "stringSlice"},
		{vmConfigPendingCmd, "vmname", "stringSlice"},

		// The CSV-corruption guard: these must NOT become stringSlice.
		{vmConfigSetCmd, "set", "stringArray"},
		{vmConfigSetCmd, "delete", "stringArray"},
	}

	for _, tc := range cases {
		name := tc.cmd.Name() + "/--" + tc.flag
		t.Run(name, func(t *testing.T) {
			flag := tc.cmd.Flags().Lookup(tc.flag)
			if flag == nil {
				t.Fatalf("--%s is not registered on %q", tc.flag, tc.cmd.Name())
			}
			if got := flag.Value.Type(); got != tc.want {
				t.Fatalf("--%s is %q, want %q", tc.flag, got, tc.want)
			}
		})
	}
}

// TestConfigVMSelectorAcceptsMultipleValues checks the selector flags actually
// split, rather than only checking their declared type.
func TestConfigVMSelectorAcceptsMultipleValues(t *testing.T) {
	fs := pflag.NewFlagSet("config-get", pflag.ContinueOnError)
	fs.AddFlag(vmConfigGetCmd.Flags().Lookup("vmid"))

	if err := fs.Parse([]string{"--vmid", "4001,4002"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	got, err := fs.GetStringSlice("vmid")
	if err != nil {
		t.Fatalf("GetStringSlice failed: %v", err)
	}
	if len(got) != 2 || got[0] != "4001" || got[1] != "4002" {
		t.Fatalf("got %q, want [4001 4002]", got)
	}
}

// TestConfigTargetsRequiresSelector guards the empty-selector case. Without it
// an omitted --vmid would fall through to resolveVMs and, with no tokens to
// resolve, return an empty set that reads as "nothing to do".
func TestConfigTargetsRequiresSelector(t *testing.T) {
	cmd := &cobra.Command{Use: "get"}
	cmd.Flags().StringSlice("vmid", []string{}, "")
	cmd.Flags().StringSlice("vmname", []string{}, "")

	_, err := configTargets(cmd)
	if err == nil {
		t.Fatal("expected an error when neither --vmid nor --vmname is given")
	}
	if !strings.Contains(err.Error(), "must be specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConfigSetDoesNotAutoDetectBatchMode is the guard for a trap that would
// silently disable this command's own confirmation.
//
// autoDetectBatchMode() sets batchMode=true whenever more than one VM is
// selected. confirmExactText() hard-fails when batchMode is set without -y. So
// if runConfigSetCommand ever calls autoDetectBatchMode, every multi-VM run
// without -y would abort instead of prompting, and the typed-count guard would
// become unreachable. cmd_vm_snapshot_cluster.go omits it for the same reason.
//
// This asserts on source text because the failure is a *call that must not
// exist* -- there is no runtime state to observe once the trap has been sprung.
// It matches the call form "autoDetectBatchMode(" rather than the bare name, so
// that the comment above (and the one in cmd_vm_config.go) can name the trap
// without tripping the guard against it.
func TestConfigSetDoesNotAutoDetectBatchMode(t *testing.T) {
	src, err := os.ReadFile("cmd_vm_config.go")
	if err != nil {
		t.Fatalf("cannot read cmd_vm_config.go: %v", err)
	}

	// Self-check: the probe is only meaningful if this file still contains the
	// guard it is protecting, so verify a known-present sibling call before
	// trusting the absence of the forbidden one.
	if !strings.Contains(string(src), "confirmExactText(") {
		t.Fatal("sanity check failed: expected a confirmExactText( call in cmd_vm_config.go " +
			"-- the typed-count guard is missing, so this test proves nothing")
	}

	if strings.Contains(string(src), "autoDetectBatchMode(") {
		t.Fatal("cmd_vm_config.go calls autoDetectBatchMode: this sets batchMode " +
			"for >1 VM, which makes confirmExactText hard-fail instead of prompting")
	}
}

func TestParseKeyValuePairs(t *testing.T) {
	t.Run("simple pairs", func(t *testing.T) {
		got, err := parseKeyValuePairs([]string{"cores=4", "memory=8192"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["cores"] != "4" || got["memory"] != "8192" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	// Proxmox config values routinely contain '=' themselves, so only the
	// first separator may be treated as the key/value boundary.
	t.Run("value containing equals is preserved", func(t *testing.T) {
		got, err := parseKeyValuePairs([]string{"net0=virtio,bridge=vmbr0,tag=100"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "virtio,bridge=vmbr0,tag=100"
		if got["net0"] != want {
			t.Fatalf("got %q, want %q", got["net0"], want)
		}
	})

	t.Run("empty value is allowed", func(t *testing.T) {
		got, err := parseKeyValuePairs([]string{"description="})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v, ok := got["description"]; !ok || v != "" {
			t.Fatalf("expected empty description, got %#v", got)
		}
	})

	t.Run("missing separator is rejected", func(t *testing.T) {
		if _, err := parseKeyValuePairs([]string{"cores"}); err == nil {
			t.Fatal("expected an error for a pair with no '='")
		}
	})

	t.Run("empty key is rejected", func(t *testing.T) {
		if _, err := parseKeyValuePairs([]string{"=4"}); err == nil {
			t.Fatal("expected an error for an empty key")
		}
	})
}

func TestFormatConfigValue(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil renders as unset", nil, "(unset)"},
		{"string passthrough", "virtio", "virtio"},
		// The API returns numbers as float64; whole numbers must not render
		// as "4" -> "4e+00" or similar.
		{"whole float renders as integer", float64(4), "4"},
		{"large whole float", float64(8192), "8192"},
		// Disk and memory values reach byte scale, where %v/%g would switch to
		// scientific notation. The integer branch must catch these.
		{"byte-scale value is not scientific notation", float64(1099511627776), "1099511627776"},
		{"megabyte-scale value is not scientific notation", float64(16777216), "16777216"},
		{"fractional float", 1.5, "1.5"},
		{"true renders as 1", true, "1"},
		{"false renders as 0", false, "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatConfigValue(tc.value); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
