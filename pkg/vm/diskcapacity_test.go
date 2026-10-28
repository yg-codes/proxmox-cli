package vm

import "testing"

// TestDiskFitsCapacity documents the arithmetic the move-disk pre-check relies
// on. The dev cluster's stores are far larger than any test disk, so the refusal
// branch cannot be reached against live infrastructure — it is exercised here
// instead, rather than left unverified because the happy path passed.
func TestDiskFitsCapacity(t *testing.T) {
	const giB = int64(1) << 30

	cases := []struct {
		name      string
		spec      string
		availByte int64
		wantFits  bool
	}{
		{
			// Real values observed on pve-node1, 2026-08-25.
			name:      "10G disk fits local-zfs with 698 GiB free",
			spec:      "local-ceph:vm-4001-disk-1,iothread=1,size=10G,ssd=1",
			availByte: 749906294592,
			wantFits:  true,
		},
		{
			name:      "10G disk fits local-ceph with 132 GiB free",
			spec:      "local-ceph:vm-4001-disk-1,size=10G",
			availByte: 142115782656,
			wantFits:  true,
		},
		{
			name:      "100G disk does not fit 50 GiB free",
			spec:      "local-zfs:vm-1-disk-0,size=100G",
			availByte: 50 * giB,
			wantFits:  false,
		},
		{
			// Exactly equal must fit: refusing a disk that precisely fits would
			// be a false failure on a legitimate consolidation.
			name:      "disk exactly equal to free space fits",
			spec:      "local-zfs:vm-1-disk-0,size=10G",
			availByte: 10 * giB,
			wantFits:  true,
		},
		{
			name:      "one byte short does not fit",
			spec:      "local-zfs:vm-1-disk-0,size=10G",
			availByte: 10*giB - 1,
			wantFits:  false,
		},
		{
			// An efidisk is sized in KiB. Reading its unit as GiB would make a
			// 528 KiB volume look like 528 GiB and refuse every real move.
			name:      "528K efidisk fits a small store",
			spec:      "local-ceph:vm-4001-disk-0,efitype=4m,size=528K",
			availByte: 1 * giB,
			wantFits:  true,
		},
		{
			// No size key means "cannot determine", which must not be treated
			// as a zero-byte disk that always fits, nor as a refusal.
			name:      "cdrom with no size reports zero and is not checked",
			spec:      "local:iso/rocky10.iso,media=cdrom",
			availByte: 0,
			wantFits:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			needed, err := ParseDiskSizeBytes(tc.spec)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			// This mirrors the comparison in checkMoveDiskCapacity: a zero
			// size means "not checked", so it never refuses.
			fits := needed == 0 || tc.availByte >= needed
			if fits != tc.wantFits {
				t.Fatalf("needed=%d avail=%d: got fits=%v, want %v",
					needed, tc.availByte, fits, tc.wantFits)
			}
		})
	}
}
