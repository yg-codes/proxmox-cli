package main

import (
	"fmt"
	"strings"
	"testing"
)

// The exit contract for multi-target commands: no failures means success
// (nil); any failure means a non-nil error carrying the count and the joined
// per-target causes, so Cobra exits non-zero after the summary was printed.
func TestJoinTargetErrorsExit(t *testing.T) {
	if err := joinTargetErrors(3, nil); err != nil {
		t.Errorf("no failures: err=%v, want nil", err)
	}
	if err := joinTargetErrors(3, []error{}); err != nil {
		t.Errorf("empty failures: err=%v, want nil", err)
	}

	failures := []error{
		fmt.Errorf("VM 100: connection refused"),
		fmt.Errorf("VM 102: permission denied"),
	}
	err := joinTargetErrors(3, failures)
	if err == nil {
		t.Fatal("two of three targets failed: err=nil, want aggregate error")
	}
	if !strings.Contains(err.Error(), "2 of 3") {
		t.Errorf("aggregate error %q does not carry the failure count", err)
	}
	for _, target := range []string{"VM 100", "VM 102"} {
		if !strings.Contains(err.Error(), target) {
			t.Errorf("aggregate error %q does not name %s", err, target)
		}
	}
}
