package main

import (
	"strings"
	"testing"

	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// resolveNameAgainst mirrors the name-matching half of resolveSingleVMID
// without needing a live cluster. The exact-match and ambiguity rules are the
// safety-relevant part, and they are what this pins.
func resolveNameAgainst(vmname string, all []*vm.VM) (string, error) {
	var matches []*vm.VM
	for _, v := range all {
		if strings.EqualFold(v.Name, vmname) {
			matches = append(matches, v)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].VMID, nil
	case 0:
		return "", errNoMatch
	default:
		return "", errAmbiguous
	}
}

var (
	errNoMatch   = &resolveErr{"no match"}
	errAmbiguous = &resolveErr{"ambiguous"}
)

type resolveErr struct{ s string }

func (e *resolveErr) Error() string { return e.s }

// TestVMNameResolutionIsExact is the load-bearing test for --vmname.
//
// Partial matching would be convenient and dangerous: `--vmname web` quietly
// resolving to `web-prod-01` on `pve vm destroy` destroys the wrong VM. The
// cluster's own names are near-misses of each other, which is why this matters
// here specifically.
func TestVMNameResolutionIsExact(t *testing.T) {
	cluster := []*vm.VM{
		{VMID: "4001", Name: "rocky10-vm01", Node: "pve91"},
		{VMID: "4002", Name: "rocky10-vm02", Node: "pve92"},
		{VMID: "5001", Name: "tmpl-rocky10-base-v20260519", Node: "pve91"},
	}

	t.Run("exact name resolves", func(t *testing.T) {
		got, err := resolveNameAgainst("rocky10-vm01", cluster)
		if err != nil || got != "4001" {
			t.Fatalf("expected 4001, got %q (err %v)", got, err)
		}
	})

	t.Run("case-insensitive exact match resolves", func(t *testing.T) {
		got, err := resolveNameAgainst("ROCKY10-VM01", cluster)
		if err != nil || got != "4001" {
			t.Fatalf("name matching should ignore case: got %q (err %v)", got, err)
		}
	})

	// Each of these is a substring of a real VM name. Resolving any of them
	// would mean acting on a VM the user did not name.
	for _, partial := range []string{
		"rocky10", "dev", "vm01", "rocky10", "tmpl",
	} {
		t.Run("refuses partial name "+partial, func(t *testing.T) {
			if _, err := resolveNameAgainst(partial, cluster); err == nil {
				t.Fatalf("%q is only a substring of a real name and must NOT resolve", partial)
			}
		})
	}

	t.Run("unknown name is refused", func(t *testing.T) {
		if _, err := resolveNameAgainst("does-not-exist", cluster); err == nil {
			t.Fatal("an unknown name must be refused")
		}
	})

	// Proxmox does not enforce unique VM names across a cluster, so a duplicate
	// is possible. Guessing between them is the one thing this must never do.
	t.Run("duplicate names are refused, not guessed", func(t *testing.T) {
		dupes := []*vm.VM{
			{VMID: "100", Name: "web01", Node: "pve91"},
			{VMID: "200", Name: "web01", Node: "pve92"},
		}
		if got, err := resolveNameAgainst("web01", dupes); err == nil {
			t.Fatalf("an ambiguous name must be refused, but it resolved to %q", got)
		}
	})
}

// TestDestroyRejectsSelectorsBeforeNameResolution documents the ordering:
// selector rejection runs on the raw --vmid, before any name lookup, so a
// pattern cannot reach the resolver and expand there.
func TestDestroyRejectsSelectorsBeforeNameResolution(t *testing.T) {
	for _, sel := range []string{"4001,4002", "40*", "all", "running"} {
		if err := rejectMultiVMSelector(sel); err == nil {
			t.Fatalf("%q must be rejected as a --vmid before resolution", sel)
		}
	}
}
