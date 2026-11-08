package main

import (
	"strings"
	"testing"
)

// Log lines are diagnostics: by default they must go to stderr so stdout
// carries only command output that scripts can capture or parse.
func TestLogLinesGoToStderrNotStdout(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "stopped")

	var out string
	var execErr error
	stderr := captureStderr(t, func() {
		out, execErr = execCLI(t, f, "vm", "start", "--vmid", "100", "-y")
	})

	if execErr != nil {
		t.Fatalf("vm start: %v (stdout=%q)", execErr, out)
	}
	if strings.Contains(out, "Started VM 100") {
		t.Errorf("log line leaked to stdout:\n%s", out)
	}
	if !strings.Contains(stderr, "Started VM 100") {
		t.Errorf("log line missing from stderr; stderr=%q", stderr)
	}
	if !strings.Contains(out, "BULK OPERATION SUMMARY") {
		t.Errorf("command output (summary) missing from stdout:\n%s", out)
	}
}
