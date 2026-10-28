package main

import "testing"

// TestRejectMultiVMSelector guards the narrowest part of the destroy command.
//
// Every other VM command accepts ranges, lists, globs and keywords. For destroy
// each of those is a way to delete more than was meant, and there is no undo —
// so a selector that slips through here is unrecoverable, not merely wrong.
func TestRejectMultiVMSelector(t *testing.T) {
	mustReject := []struct {
		name string
		sel  string
	}{
		{"comma list", "4001,4002"},
		{"range", "4001-4002"},
		{"trailing wildcard", "40*"},
		{"single-char wildcard", "9730?"},
		{"all keyword", "all"},
		{"running keyword", "running"},
		{"stopped keyword", "stopped"},
		{"interactive selector", "i"},
		{"vm name rather than id", "clone-from-vm"},
		{"empty-ish whitespace", " "},
		{"negative", "-4001"},
	}

	for _, tc := range mustReject {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := rejectMultiVMSelector(tc.sel); err == nil {
				t.Fatalf("selector %q must be rejected: destroy is irreversible and has no bulk form", tc.sel)
			}
		})
	}

	mustAccept := []string{"100", "4001", "999999"}
	for _, sel := range mustAccept {
		t.Run("accepts "+sel, func(t *testing.T) {
			if err := rejectMultiVMSelector(sel); err != nil {
				t.Fatalf("plain VMID %q should be accepted, got: %v", sel, err)
			}
		})
	}
}

// TestDiskEntries checks what destroy reports as "about to be deleted". Getting
// this wrong in either direction misleads at the worst moment: omitting a real
// disk hides data loss, and listing a cdrom implies an ISO is destroyed with the
// VM when it is not.
func TestDiskEntries(t *testing.T) {
	config := map[string]interface{}{
		"name":      "test",
		"cores":     float64(2),
		"scsi0":     "local-zfs:vm-100-disk-0,size=32G",
		"scsi1":     "local-ceph:vm-100-disk-1,size=100G",
		"efidisk0":  "local-zfs:vm-100-disk-2,efitype=4m,size=528K",
		"ide2":      "local:iso/rocky10.iso,media=cdrom",
		"ide0":      "none,media=cdrom",
		"net0":      "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0",
		"unused0":   "local-zfs:vm-100-disk-3",
		"tpmstate0": "local-zfs:vm-100-disk-4,size=4M,version=v2.0",
	}

	got := diskEntries(config)
	devices := make(map[string]bool, len(got))
	for _, d := range got {
		devices[d.device] = true
	}

	for _, want := range []string{"scsi0", "scsi1", "efidisk0", "unused0", "tpmstate0"} {
		if !devices[want] {
			t.Errorf("%s holds VM data and must be listed as destroyed", want)
		}
	}

	// A cdrom references an ISO on shared storage; it is not destroyed with the
	// VM, so listing it would overstate what is lost.
	for _, unwanted := range []string{"ide2", "ide0", "net0", "name", "cores"} {
		if devices[unwanted] {
			t.Errorf("%s must not be listed as a destroyed disk", unwanted)
		}
	}

	// An unused volume still occupies space and is removed with the VM, so it
	// belongs in the list — this is the entry most easily forgotten.
	if !devices["unused0"] {
		t.Error("unused0 is a real volume and must be reported")
	}
}
