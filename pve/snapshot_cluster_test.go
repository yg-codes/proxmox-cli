package main

import (
	"testing"

	"github.com/yg-codes/proxmox-cli/pkg/snapshot"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// TestPruneRefusesWithoutFilter is the load-bearing guard for prune.
//
// Without it, `pve vm snapshot prune` with no arguments would resolve to every
// VM in the cluster and every snapshot on them. The refusal is the only thing
// standing between a bare command and total snapshot loss, so it is pinned
// here rather than trusted to the handler's shape.
func TestPruneRefusesWithoutFilter(t *testing.T) {
	// Mirrors the guard in runSnapshotPruneCommand: refuse when the filter is
	// empty AND no VM selection was given.
	refuses := func(filter snapshot.ScanFilter, vmids, vmnames []string) bool {
		return filter.IsEmpty() && len(vmids) == 0 && len(vmnames) == 0
	}

	cases := []struct {
		name        string
		filter      snapshot.ScanFilter
		vmids       []string
		vmnames     []string
		wantRefusal bool
	}{
		{
			name:        "no filter at all",
			wantRefusal: true,
		},
		{
			name:        "older-than only",
			filter:      snapshot.ScanFilter{OlderThan: 1},
			wantRefusal: false,
		},
		{
			name:        "name-pattern only",
			filter:      snapshot.ScanFilter{NamePattern: "x-*"},
			wantRefusal: false,
		},
		{
			name:        "vmid only",
			vmids:       []string{"4001"},
			wantRefusal: false,
		},
		{
			name:        "vmname only",
			vmnames:     []string{"web01"},
			wantRefusal: false,
		},
		{
			name:        "vmid all still counts as a selection",
			vmids:       []string{"all"},
			wantRefusal: false,
		},
	}

	for _, c := range cases {
		got := refuses(c.filter, c.vmids, c.vmnames)
		if got != c.wantRefusal {
			t.Errorf("%s: refuses = %v, want %v", c.name, got, c.wantRefusal)
		}
	}
}

// TestCountDistinctVMs pins the count shown in the confirmation prompt. The
// operator consents on the strength of these numbers, so double-counting a VM
// with several matched snapshots would misstate the blast radius.
func TestCountDistinctVMs(t *testing.T) {
	mk := func(vmid string) snapshot.ClusterSnapshot {
		return snapshot.ClusterSnapshot{
			VM:       &vm.VM{VMID: vmid},
			Snapshot: &snapshot.Snapshot{Name: "s"},
		}
	}

	got := countDistinctVMs([]snapshot.ClusterSnapshot{
		mk("100"), mk("100"), mk("100"), mk("200"),
	})
	if got != 2 {
		t.Errorf("countDistinctVMs = %d, want 2 (one VM with 3 snapshots is still one VM)", got)
	}

	if n := countDistinctVMs(nil); n != 0 {
		t.Errorf("countDistinctVMs(nil) = %d, want 0", n)
	}
}

// TestResolveVMsSelectorsReachParseVMSelection pins the fix for the defect that
// motivated this work: resolveVMs used to call FindVMByNameOrID per token,
// which understands only IDs and names. Keywords, wildcards and ranges were
// silently unreachable despite being documented.
//
// This exercises Selector.ParseVMSelection directly with the same inputs
// resolveVMs now feeds it, so a regression back to the old resolver shows up
// as these selectors returning nothing.
func TestResolveVMsSelectorsReachParseVMSelection(t *testing.T) {
	all := []*vm.VM{
		{VMID: "4001", Name: "rocky-vm01", Running: true},
		{VMID: "4002", Name: "rocky-vm02", Running: false},
		{VMID: "5001", Name: "tmpl-base", Running: false},
	}

	sel := vm.NewSelector(nil, nil)

	cases := []struct {
		selection string
		wantIDs   []string
	}{
		{"all", []string{"4001", "4002", "5001"}},
		{"running", []string{"4001"}},
		{"stopped", []string{"4002", "5001"}},
		{"40*", []string{"4001", "4002"}},
		{"4001-4002", []string{"4001", "4002"}},
		{"4001", []string{"4001"}},
		{"rocky-vm01", []string{"4001"}},
	}

	for _, c := range cases {
		result, err := sel.ParseVMSelection(c.selection, all)
		if err != nil {
			t.Errorf("selector %q: unexpected error: %v", c.selection, err)
			continue
		}
		if len(result.VMs) != len(c.wantIDs) {
			t.Errorf("selector %q: got %d VMs, want %d",
				c.selection, len(result.VMs), len(c.wantIDs))
			continue
		}
		got := make(map[string]bool, len(result.VMs))
		for _, v := range result.VMs {
			got[v.VMID] = true
		}
		for _, want := range c.wantIDs {
			if !got[want] {
				t.Errorf("selector %q: missing VM %s", c.selection, want)
			}
		}
	}

	// A genuinely absent VM must still error rather than silently match none.
	if _, err := sel.ParseVMSelection("99999", all); err == nil {
		t.Error("absent VM must return an error, not an empty match")
	}
}

// TestHyphenatedVMNamesAreNotRanges guards a regression introduced when
// resolveVMs was routed through ParseVMSelection: the dispatcher treated any
// selection containing "-" as a numeric range, which broke every hyphenated VM
// name. Real names in this fleet are overwhelmingly hyphenated
// (db01, rocky10-vm01), so this is the common case, not an edge.
func TestHyphenatedVMNamesAreNotRanges(t *testing.T) {
	all := []*vm.VM{
		{VMID: "22001", Name: "db01", Running: true},
		{VMID: "4001", Name: "rocky10-vm01", Running: true},
		{VMID: "4002", Name: "rocky10-vm02", Running: false},
	}

	sel := vm.NewSelector(nil, nil)

	for _, name := range []string{"db01", "rocky10-vm01"} {
		result, err := sel.ParseVMSelection(name, all)
		if err != nil {
			t.Errorf("hyphenated name %q must resolve, got error: %v", name, err)
			continue
		}
		if len(result.VMs) != 1 || result.VMs[0].Name != name {
			t.Errorf("hyphenated name %q resolved to %d VM(s), want exactly itself",
				name, len(result.VMs))
		}
	}

	// A genuine numeric range must still work.
	result, err := sel.ParseVMSelection("4001-4002", all)
	if err != nil {
		t.Fatalf("numeric range must still work: %v", err)
	}
	if len(result.VMs) != 2 {
		t.Errorf("numeric range matched %d VMs, want 2", len(result.VMs))
	}
}
