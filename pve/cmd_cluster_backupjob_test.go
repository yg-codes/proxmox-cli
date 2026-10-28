package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestBackupJobFlagTypes pins the flag types where a silent regression would
// corrupt values, mirroring TestConfigFlagTypes.
//
//   - --exclude-path must be StringArray: the globs it carries can contain
//     characters a CSV-parsing slice flag would mangle, and repeated flags
//     must accumulate rather than be re-split.
//   - --vmid/--exclude must stay plain String: unlike the selector flags
//     elsewhere in the CLI, these values are stored verbatim in jobs.cfg, so
//     a slice type would join-and-resplit them into a different wire value.
func TestBackupJobFlagTypes(t *testing.T) {
	cases := []struct {
		cmd  *cobra.Command
		flag string
		want string
	}{
		{clusterBackupJobCreateCmd, "exclude-path", "stringArray"},
		{clusterBackupJobUpdateCmd, "exclude-path", "stringArray"},
		{clusterBackupJobCreateCmd, "vmid", "string"},
		{clusterBackupJobUpdateCmd, "vmid", "string"},
		{clusterBackupJobCreateCmd, "exclude", "string"},
		{clusterBackupJobUpdateCmd, "exclude", "string"},
		{clusterBackupJobDeleteCmd, "id", "string"},
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

// TestBackupJobExcludePathAccumulates checks the repeatable flag actually
// accumulates rather than only checking its declared type.
func TestBackupJobExcludePathAccumulates(t *testing.T) {
	fs := pflag.NewFlagSet("create", pflag.ContinueOnError)
	fs.AddFlag(clusterBackupJobCreateCmd.Flags().Lookup("exclude-path"))

	if err := fs.Parse([]string{
		"--exclude-path", "/var/log",
		"--exclude-path", "/tmp/*",
		"--exclude-path", "+ /var/lib/foo",
	}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	got, err := fs.GetStringArray("exclude-path")
	if err != nil {
		t.Fatalf("GetStringArray failed: %v", err)
	}
	want := []string{"/var/log", "/tmp/*", "+ /var/lib/foo"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestBackupJobEnableDisableTriState guards the cobra bool-flag trap: a plain
// --enabled bool cannot distinguish "not passed" from "passed false", which
// update needs (leave unchanged / enable / disable). backupJobFromFlags
// implements this with an --enable/--disable pair.
func TestBackupJobEnableDisableTriState(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		backupJobCreateFlags(cmd.Flags())
		return cmd
	}

	t.Run("neither flag leaves Enabled nil", func(t *testing.T) {
		cmd := newCmd()
		if err := cmd.Flags().Parse([]string{"--vmid", "4001"}); err != nil {
			t.Fatal(err)
		}
		job, err := backupJobFromFlags(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if job.Enabled != nil {
			t.Fatalf("Enabled = %v, want nil (leave unchanged)", *job.Enabled)
		}
	})

	t.Run("--enable sets pointer true", func(t *testing.T) {
		cmd := newCmd()
		if err := cmd.Flags().Parse([]string{"--enable"}); err != nil {
			t.Fatal(err)
		}
		job, err := backupJobFromFlags(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if job.Enabled == nil || !*job.Enabled {
			t.Fatalf("Enabled = %v, want pointer to true", job.Enabled)
		}
	})

	t.Run("--disable sets pointer false", func(t *testing.T) {
		cmd := newCmd()
		if err := cmd.Flags().Parse([]string{"--disable"}); err != nil {
			t.Fatal(err)
		}
		job, err := backupJobFromFlags(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if job.Enabled == nil || *job.Enabled {
			t.Fatalf("Enabled = %v, want pointer to false", job.Enabled)
		}
	})

	t.Run("both flags are rejected", func(t *testing.T) {
		cmd := newCmd()
		if err := cmd.Flags().Parse([]string{"--enable", "--disable"}); err != nil {
			t.Fatal(err)
		}
		if _, err := backupJobFromFlags(cmd); err == nil {
			t.Fatal("expected an error for --enable together with --disable")
		}
	})
}

// TestBackupJobDeleteDoesNotAutoDetectBatchMode is the guard for the trap
// documented in cmd_cluster_backupjob.go: autoDetectBatchMode sets batchMode
// whenever more than one target is selected, and confirmExactText hard-fails
// in batch mode without -y — so a call here would make this command's own
// typed-ID prompt unreachable. Mirrors TestConfigSetDoesNotAutoDetectBatchMode.
func TestBackupJobDeleteDoesNotAutoDetectBatchMode(t *testing.T) {
	src, err := os.ReadFile("cmd_cluster_backupjob.go")
	if err != nil {
		t.Fatalf("cannot read cmd_cluster_backupjob.go: %v", err)
	}

	// Self-check: the probe only proves something if the typed confirmation
	// this guard protects is still present.
	if !strings.Contains(string(src), "confirmExactText(") {
		t.Fatal("sanity check failed: expected a confirmExactText( call in " +
			"cmd_cluster_backupjob.go — the typed-ID guard is missing, so this test proves nothing")
	}

	if strings.Contains(string(src), "autoDetectBatchMode(") {
		t.Fatal("cmd_cluster_backupjob.go calls autoDetectBatchMode: this sets batchMode " +
			"which makes confirmExactText hard-fail instead of prompting")
	}
}
