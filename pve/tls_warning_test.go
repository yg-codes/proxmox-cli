package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// With verification disabled (the harness pins PVE_VERIFY_SSL=false), the
// insecure-TLS warning must reach the operator on stderr and must NOT appear
// on stdout: batch consumers parse stdout as command output, and the CLI
// logger writes to stdout by default.
func TestInsecureTLSWarningGoesToStderrNotStdout(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "running")

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	out, execErr := execCLI(t, f, "--batch", "vm", "agent", "osinfo", "--vmid", "100")

	_ = w.Close()
	os.Stderr = oldStderr
	stderr := <-done

	if execErr != nil {
		t.Fatalf("osinfo: %v (stdout=%q)", execErr, out)
	}
	if !json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("stdout is not valid JSON:\n%s", out)
	}
	if strings.Contains(out, "verification is DISABLED") {
		t.Errorf("TLS warning leaked to stdout:\n%s", out)
	}
	if !strings.Contains(stderr, "verification is DISABLED") {
		t.Errorf("TLS warning missing from stderr; stderr=%q", stderr)
	}
}
