package disks

import "testing"

// TestValidateCreatePoolRaidLevels covers the per-level device-count rules the
// API enforces. Catching these locally matters because the alternative is a
// rejection after the operator has already confirmed wiping the disks.
func TestValidateCreatePoolRaidLevels(t *testing.T) {
	dev := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "/dev/sd" + string(rune('b'+i))
		}
		return out
	}

	valid := []struct {
		level string
		n     int
	}{
		{"single", 1},
		{"mirror", 2},
		{"mirror", 3}, // n-way mirror is legal
		{"raid10", 4},
		{"raid10", 6},
		{"raidz", 3},
		{"raidz2", 4},
		{"raidz3", 5},
		{"draid", 3},
		{"draid2", 4},
		{"draid3", 5},
	}
	for _, tc := range valid {
		t.Run("accepts "+tc.level, func(t *testing.T) {
			opts := &CreatePoolOptions{Name: "tank", RaidLevel: tc.level, Devices: dev(tc.n)}
			if err := ValidateCreatePool(opts); err != nil {
				t.Fatalf("%s with %d devices should be valid: %v", tc.level, tc.n, err)
			}
		})
	}

	invalid := []struct {
		name  string
		level string
		n     int
	}{
		{"mirror needs 2", "mirror", 1},
		{"raid10 needs 4", "raid10", 2},
		// raid10 stripes mirrors, so an odd count cannot be laid out even when
		// it clears the minimum.
		{"raid10 rejects odd count above the minimum", "raid10", 5},
		{"raidz needs 3", "raidz", 2},
		{"raidz2 needs 4", "raidz2", 3},
		{"raidz3 needs 5", "raidz3", 4},
		{"draid needs 3", "draid", 2},
		// single means exactly one, not "at least one".
		{"single rejects 2", "single", 2},
	}
	for _, tc := range invalid {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			opts := &CreatePoolOptions{Name: "tank", RaidLevel: tc.level, Devices: dev(tc.n)}
			if err := ValidateCreatePool(opts); err == nil {
				t.Fatalf("%s with %d devices must be rejected", tc.level, tc.n)
			}
		})
	}
}

func TestValidateCreatePoolOtherFields(t *testing.T) {
	base := func() *CreatePoolOptions {
		return &CreatePoolOptions{
			Name:      "tank",
			RaidLevel: "mirror",
			Devices:   []string{"/dev/sdb", "/dev/sdc"},
		}
	}

	t.Run("unknown raid level is rejected", func(t *testing.T) {
		o := base()
		o.RaidLevel = "raidz9"
		if err := ValidateCreatePool(o); err == nil {
			t.Fatal("expected an error for an unknown raid level")
		}
	})

	t.Run("missing name is rejected", func(t *testing.T) {
		o := base()
		o.Name = ""
		if err := ValidateCreatePool(o); err == nil {
			t.Fatal("a pool name is required")
		}
	})

	t.Run("no devices is rejected", func(t *testing.T) {
		o := base()
		o.Devices = nil
		if err := ValidateCreatePool(o); err == nil {
			t.Fatal("at least one device is required")
		}
	})

	// ashift cannot be changed after creation, so a wrong value is permanent.
	for _, ashift := range []int{8, 17, 20} {
		t.Run("rejects out-of-range ashift", func(t *testing.T) {
			o := base()
			o.Ashift = ashift
			if err := ValidateCreatePool(o); err == nil {
				t.Fatalf("ashift %d is outside 9-16 and must be rejected", ashift)
			}
		})
	}
	for _, ashift := range []int{0, 9, 12, 16} {
		t.Run("accepts in-range ashift", func(t *testing.T) {
			o := base()
			o.Ashift = ashift
			if err := ValidateCreatePool(o); err != nil {
				t.Fatalf("ashift %d should be accepted (0 means unset): %v", ashift, err)
			}
		})
	}

	t.Run("unknown compression is rejected", func(t *testing.T) {
		o := base()
		o.Compression = "brotli"
		if err := ValidateCreatePool(o); err == nil {
			t.Fatal("expected an error for an unsupported compression algorithm")
		}
	})
	for _, algo := range []string{"on", "off", "gzip", "lz4", "lzjb", "zle", "zstd"} {
		t.Run("accepts compression "+algo, func(t *testing.T) {
			o := base()
			o.Compression = algo
			if err := ValidateCreatePool(o); err != nil {
				t.Fatalf("%s is a valid algorithm: %v", algo, err)
			}
		})
	}

	// dRAID settings on a non-dRAID level are silently ignored by the API,
	// which would leave the operator believing they took effect.
	t.Run("draid flags rejected on a non-draid level", func(t *testing.T) {
		o := base()
		o.DraidSpares = 1
		if err := ValidateCreatePool(o); err == nil {
			t.Fatal("draid-spares on a mirror must be rejected")
		}
	})
	t.Run("draid flags accepted on a draid level", func(t *testing.T) {
		o := &CreatePoolOptions{
			Name: "tank", RaidLevel: "draid2",
			Devices:     []string{"/dev/sdb", "/dev/sdc", "/dev/sdd", "/dev/sde"},
			DraidData:   2,
			DraidSpares: 1,
		}
		if err := ValidateCreatePool(o); err != nil {
			t.Fatalf("draid settings should be valid on draid2: %v", err)
		}
	})

	t.Run("nil options rejected", func(t *testing.T) {
		if err := ValidateCreatePool(nil); err == nil {
			t.Fatal("nil options must be rejected rather than panicking")
		}
	})
}

// TestDraidConfig checks the property-string encoding. draid-config is a single
// parameter, not separate fields, so an empty result must stay absent rather
// than being sent as an empty string.
func TestDraidConfig(t *testing.T) {
	cases := []struct {
		name string
		opts *CreatePoolOptions
		want string
	}{
		{"both set", &CreatePoolOptions{DraidData: 4, DraidSpares: 2}, "data=4,spares=2"},
		{"data only", &CreatePoolOptions{DraidData: 4}, "data=4"},
		{"spares only", &CreatePoolOptions{DraidSpares: 2}, "spares=2"},
		{"neither set yields empty", &CreatePoolOptions{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := draidConfig(tc.opts); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRaidLevelMinDisks(t *testing.T) {
	if n, ok := RaidLevelMinDisks("raidz2"); !ok || n != 4 {
		t.Fatalf("raidz2 should report a minimum of 4, got %d (ok=%v)", n, ok)
	}
	if _, ok := RaidLevelMinDisks("nonsense"); ok {
		t.Fatal("an unknown level must report ok=false")
	}
}
