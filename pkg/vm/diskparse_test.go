package vm

import "testing"

// TestParseDiskSizeBytes covers the real shapes a Proxmox disk spec takes. The
// size is one comma-separated key among several, so a parser that assumes
// position rather than the "size=" prefix will read the wrong field.
func TestParseDiskSizeBytes(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want int64
	}{
		{
			// The exact spec from test VM 4001.
			name: "real scsi0 spec, size in the middle",
			spec: "local-ceph:vm-4001-disk-1,iothread=1,size=10G,ssd=1",
			want: 10 << 30,
		},
		{
			name: "template base volume",
			spec: "local-ceph:base-5001-disk-1,iothread=1,size=10G,ssd=1",
			want: 10 << 30,
		},
		{
			// An EFI disk is sized in KiB — a parser hardcoding GiB reports
			// 528 GiB for it and would refuse a move onto any sane storage.
			name: "efidisk sized in KiB",
			spec: "local-ceph:vm-4001-disk-0,efitype=4m,ms-cert=2023,pre-enrolled-keys=1,size=528K",
			want: 528 << 10,
		},
		{name: "megabytes", spec: "local-zfs:vm-1-disk-0,size=512M", want: 512 << 20},
		{name: "terabytes", spec: "local-zfs:vm-1-disk-0,size=2T", want: 2 << 40},
		{name: "fractional gigabytes", spec: "local-zfs:vm-1-disk-0,size=1.5G", want: 1610612736},
		{name: "bare bytes, no unit", spec: "local-zfs:vm-1-disk-0,size=1073741824", want: 1 << 30},
		{name: "size is the only key", spec: "size=4G", want: 4 << 30},
		{name: "lowercase unit", spec: "local-zfs:vm-1-disk-0,size=8g", want: 8 << 30},

		// Absent size is NOT an error: a cdrom or cloudinit drive has none, and
		// erroring would block a move that Proxmox would have accepted.
		{name: "cdrom has no size", spec: "local:iso/rocky10.iso,media=cdrom", want: 0},
		{name: "cloudinit drive", spec: "local-zfs:vm-1-cloudinit,media=cdrom", want: 0},
		{name: "empty spec", spec: "", want: 0},

		// "size" must match as a key, not as a substring of another key.
		{name: "does not match a substring key", spec: "local-zfs:vm-1-disk-0,maxsize=9G", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDiskSizeBytes(tc.spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d bytes, want %d", got, tc.want)
			}
		})
	}
}

func TestParseDiskSizeBytesErrors(t *testing.T) {
	bad := []struct {
		name string
		spec string
	}{
		{"empty size value", "local-zfs:vm-1-disk-0,size="},
		{"unrecognised unit", "local-zfs:vm-1-disk-0,size=4P"},
		{"non-numeric", "local-zfs:vm-1-disk-0,size=big"},
		{"unit with no number", "local-zfs:vm-1-disk-0,size=G"},
	}

	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDiskSizeBytes(tc.spec); err == nil {
				t.Fatalf("expected an error for %q", tc.spec)
			}
		})
	}
}
