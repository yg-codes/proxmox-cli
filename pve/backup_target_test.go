package main

import (
	"strings"
	"testing"
)

// Backup restore and delete mutate one VM's backups; a list of IDs must be
// rejected instead of silently operating on the first.
func TestBackupTargetRequiresSingleVMID(t *testing.T) {
	t.Run("exactly one ID returns it", func(t *testing.T) {
		vmid, err := requireSingleVMID([]string{"4103"})
		if err != nil {
			t.Fatalf("err=%v, want nil", err)
		}
		if vmid != "4103" {
			t.Errorf("vmid=%q, want 4103", vmid)
		}
	})

	t.Run("zero IDs error", func(t *testing.T) {
		_, err := requireSingleVMID(nil)
		if err == nil {
			t.Fatal("err=nil, want single-target error")
		}
		if !strings.Contains(err.Error(), "exactly one VM ID") {
			t.Errorf("error %q does not state the single-ID requirement", err)
		}
	})

	t.Run("multiple IDs error and name every ID", func(t *testing.T) {
		_, err := requireSingleVMID([]string{"4103", "4104"})
		if err == nil {
			t.Fatal("err=nil, want single-target error")
		}
		for _, want := range []string{"exactly one VM ID", "4103", "4104"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %s", err, want)
			}
		}
	})
}
