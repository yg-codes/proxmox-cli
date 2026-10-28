package main

import (
	"testing"

	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// TestIsTemplateConfig matters because the clone summary tells the user
// whether they are getting a linked or a full clone. Proxmox only linked-clones
// templates — a normal VM is always fully copied regardless of --full — so
// misreading this key would misstate both the duration and the disk usage of
// the operation the user is about to confirm.
func TestIsTemplateConfig(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		// The API returns JSON numbers, so this is the real-world shape.
		{"template as number 1", map[string]interface{}{"template": float64(1)}, true},
		{"template as number 0", map[string]interface{}{"template": float64(0)}, false},
		// A normal VM omits the key entirely.
		{"key absent", map[string]interface{}{"cores": float64(4)}, false},
		{"empty config", map[string]interface{}{}, false},
		{"template as bool", map[string]interface{}{"template": true}, true},
		{"template as string", map[string]interface{}{"template": "1"}, true},
		{"unexpected type is not a template", map[string]interface{}{"template": []string{}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTemplateConfig(tc.config); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{512, "512 B"},
		{4 * 1024, "4.0 KiB"},
		{20 * 1024 * 1024 * 1024, "20.0 GiB"},
		{1024 * 1024, "1.0 MiB"},
		// A local disk large enough to matter for a migration decision.
		{2 * 1024 * 1024 * 1024 * 1024, "2.0 TiB"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := formatBytes(tc.bytes); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribeLocalDisks(t *testing.T) {
	disks := []vm.LocalDisk{
		{DriveName: "scsi0", Size: 20 * 1024 * 1024 * 1024},
		{DriveName: "efidisk0", Size: 1024 * 1024},
	}

	got := describeLocalDisks(disks)
	want := "scsi0 (20.0 GiB), efidisk0 (1.0 MiB)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestContains(t *testing.T) {
	nodes := []string{"pve-node4", "pve-node5"}

	if !contains(nodes, "pve-node5") {
		t.Fatal("expected the node to be found")
	}
	// A target absent from allowed_nodes must not be treated as allowed, or
	// the migration gate would pass a move the cluster will reject.
	if contains(nodes, "pve-node6") {
		t.Fatal("expected an absent node to be reported as missing")
	}
	if contains(nil, "anything") {
		t.Fatal("an empty set contains nothing")
	}
}
