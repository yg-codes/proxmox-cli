package snapshot

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"0d", 0, false},
		{"12h", 12 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"1.5d", 36 * time.Hour, false},
		{"106751d", 106751 * 24 * time.Hour, false},
		{"1e100d", 0, true},
		{"NaNd", 0, true},
		{"+Infd", 0, true},
		{"-Infd", 0, true},
		{"-5d", 0, true},
		{"-12h", 0, true},
		{"banana", 0, true},
		{"d", 0, true},
		{"30days", 0, true},
	}

	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseDuration(%q) = %v, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestFilterOlderThanBoundary pins the age comparison. The boundary matters:
// a snapshot exactly at the threshold must NOT be selected for deletion.
func TestFilterOlderThanBoundary(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	filter := ScanFilter{OlderThan: 30 * 24 * time.Hour}

	mk := func(age time.Duration) *Snapshot {
		created := now.Add(-age)
		return &Snapshot{
			Name:      "snap",
			SnapTime:  created.Unix(),
			CreatedAt: created,
		}
	}

	cases := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"well over threshold", 60 * 24 * time.Hour, true},
		{"just over threshold", 30*24*time.Hour + time.Minute, true},
		{"exactly at threshold", 30 * 24 * time.Hour, false},
		{"just under threshold", 30*24*time.Hour - time.Minute, false},
		{"brand new", 0, false},
	}

	for _, c := range cases {
		got, err := filter.matches(mk(c.age), now)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s (age %v): matches = %v, want %v", c.name, c.age, got, c.want)
		}
	}
}

// TestFilterExcludesUnknownTimestamp pins the fail-safe direction: a snapshot
// whose age cannot be established must never be selected by an age filter.
// Selecting it would mean deleting on the strength of a bad reading.
func TestFilterExcludesUnknownTimestamp(t *testing.T) {
	now := time.Now()
	filter := ScanFilter{OlderThan: time.Hour}

	snap := &Snapshot{Name: "no-timestamp", SnapTime: 0}
	got, err := filter.matches(snap, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("snapshot with unknown timestamp matched an age filter; must be excluded")
	}
}

func TestFilterNamePattern(t *testing.T) {
	now := time.Now()

	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"pre-update-*", "pre-update-20260812", true},
		{"pre-update-*", "post-update-20260812", false},
		{"*-20260812", "pre-update-20260812", true},
		{"backup", "backup", true},
		{"backup", "backup-1", false},
		{"*", "anything", true},
	}

	for _, c := range cases {
		filter := ScanFilter{NamePattern: c.pattern}
		snap := &Snapshot{Name: c.name, SnapTime: now.Unix(), CreatedAt: now}
		got, err := filter.matches(snap, now)
		if err != nil {
			t.Fatalf("pattern %q: unexpected error: %v", c.pattern, err)
		}
		if got != c.want {
			t.Errorf("pattern %q vs name %q: matches = %v, want %v",
				c.pattern, c.name, got, c.want)
		}
	}
}

// TestFilterCombinesWithAnd pins that multiple filters narrow rather than
// widen. A snapshot matching only one of two filters must not be selected.
func TestFilterCombinesWithAnd(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-60 * 24 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	filter := ScanFilter{
		OlderThan:   30 * 24 * time.Hour,
		NamePattern: "keep-*",
	}

	cases := []struct {
		name    string
		snap    *Snapshot
		want    bool
		comment string
	}{
		{
			name:    "old and matching",
			snap:    &Snapshot{Name: "keep-1", SnapTime: old.Unix(), CreatedAt: old},
			want:    true,
			comment: "satisfies both",
		},
		{
			name:    "old but not matching",
			snap:    &Snapshot{Name: "other-1", SnapTime: old.Unix(), CreatedAt: old},
			want:    false,
			comment: "age alone must not select",
		},
		{
			name:    "matching but not old",
			snap:    &Snapshot{Name: "keep-1", SnapTime: recent.Unix(), CreatedAt: recent},
			want:    false,
			comment: "name alone must not select",
		},
	}

	for _, c := range cases {
		got, err := filter.matches(c.snap, now)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: matches = %v, want %v (%s)", c.name, got, c.want, c.comment)
		}
	}
}

// TestScanFilterIsEmpty pins the guard that prune's no-filter refusal depends
// on. If IsEmpty ever returns false for a zero filter, an unfiltered prune
// becomes reachable.
func TestScanFilterIsEmpty(t *testing.T) {
	if !(ScanFilter{}).IsEmpty() {
		t.Error("zero ScanFilter must report IsEmpty; prune's refusal depends on it")
	}
	if (ScanFilter{OlderThan: time.Hour}).IsEmpty() {
		t.Error("filter with OlderThan must not report IsEmpty")
	}
	if (ScanFilter{NamePattern: "x-*"}).IsEmpty() {
		t.Error("filter with NamePattern must not report IsEmpty")
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		age      time.Duration
		snapTime int64
		want     string
	}{
		{"minutes", 30 * time.Minute, 1, "30m"},
		{"hours", 5 * time.Hour, 1, "5h"},
		{"days", 16 * 24 * time.Hour, 1, "16d"},
		{"unknown timestamp", 0, 0, "unknown"},
	}

	for _, c := range cases {
		created := now.Add(-c.age)
		got := FormatAge(created, c.snapTime, now)
		if got != c.want {
			t.Errorf("%s: FormatAge = %q, want %q", c.name, got, c.want)
		}
	}
}
