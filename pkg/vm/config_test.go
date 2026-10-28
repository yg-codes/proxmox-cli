package vm

import "testing"

func TestStringifyConfigValue(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil becomes empty", nil, ""},
		{"string passthrough", "local-zfs:vm-4103-disk-0", "local-zfs:vm-4103-disk-0"},
		// Proxmox returns numeric config values as float64 through JSON. A
		// whole number must not pick up scientific notation or a ".0" tail,
		// or the rendered diff will not match what the user typed.
		{"whole float has no decimal tail", float64(4), "4"},
		{"large whole float", float64(8192), "8192"},
		// Guards against formatting with %v, which switches to scientific
		// notation above ~1e6 and would render a 1 TiB disk size as
		// "1.099511627776e+12" instead of a usable byte count.
		{"byte-scale value is not scientific notation", float64(1099511627776), "1099511627776"},
		{"megabyte-scale value is not scientific notation", float64(16777216), "16777216"},
		{"fractional float is preserved", 1.5, "1.5"},
		{"true becomes 1", true, "1"},
		{"false becomes 0", false, "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringifyConfigValue(tc.value); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
