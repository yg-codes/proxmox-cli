package snapshot

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// ClusterSnapshot pairs a snapshot with the VM that owns it, so a cluster-wide
// result set can be rendered and acted on as a flat list.
type ClusterSnapshot struct {
	VM       *vm.VM
	Snapshot *Snapshot
}

// ScanResult is the outcome of a cluster-wide snapshot scan.
//
// Scanned and Failed are not decoration: they are what distinguishes "no
// snapshots exist" from "the scan read nothing". Without them an empty result
// set is indistinguishable from a total read failure.
type ScanResult struct {
	Snapshots []ClusterSnapshot
	// Scanned is how many VMs were successfully queried.
	Scanned int
	// WithSnapshots is how many of those had at least one real snapshot.
	WithSnapshots int
	// Failed maps VMID to the error returned for that VM.
	Failed map[string]error
}

// ScanFilter narrows a cluster scan. A zero-valued filter matches everything.
type ScanFilter struct {
	// OlderThan keeps only snapshots created longer ago than this duration.
	OlderThan time.Duration
	// NamePattern keeps only snapshots whose name matches this glob.
	NamePattern string
}

// IsEmpty reports whether the filter would narrow anything at all.
func (f ScanFilter) IsEmpty() bool {
	return f.OlderThan == 0 && f.NamePattern == ""
}

// matches reports whether a snapshot passes the filter, evaluated against the
// supplied "now" so a scan is consistent across its whole run.
func (f ScanFilter) matches(snap *Snapshot, now time.Time) (bool, error) {
	if f.OlderThan > 0 {
		// A snapshot with no usable timestamp cannot be proven old enough, so
		// exclude it rather than risk deleting something on a bad reading.
		if snap.SnapTime <= 0 {
			return false, nil
		}
		if now.Sub(snap.CreatedAt) <= f.OlderThan {
			return false, nil
		}
	}

	if f.NamePattern != "" {
		ok, err := filepath.Match(f.NamePattern, snap.Name)
		if err != nil {
			return false, fmt.Errorf("invalid --name-pattern %q: %w", f.NamePattern, err)
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

// ParseDuration parses a retention age. It accepts Go duration syntax plus a
// "d" (day) suffix, since "30d" is the natural way to express snapshot age and
// time.ParseDuration rejects it.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	if rest, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		if math.IsNaN(days) || math.IsInf(days, 0) {
			return 0, fmt.Errorf("invalid duration %q: must be a finite number of days", s)
		}
		if days < 0 {
			return 0, fmt.Errorf("invalid duration %q: must not be negative", s)
		}
		if maxDays := float64(math.MaxInt64) / float64(24*time.Hour); days > maxDays {
			return 0, fmt.Errorf("invalid duration %q: exceeds the maximum supported duration", s)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (try 30d, 12h): %w", s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid duration %q: must not be negative", s)
	}
	return d, nil
}

// ScanSnapshots queries every supplied VM for its snapshots concurrently and
// returns the flattened, filtered result.
//
// A per-VM failure is recorded in Failed rather than aborting the scan — one
// unreachable VM must not hide the other forty — but it is never silently
// dropped, because a caller about to delete needs to know its view is partial.
func (ops *Operations) ScanSnapshots(vms []*vm.VM, filter ScanFilter, maxWorkers int) (*ScanResult, error) {
	if maxWorkers <= 0 {
		maxWorkers = 3
	}

	// Evaluate every age comparison against one instant, so a slow scan cannot
	// classify two equally-old snapshots differently.
	now := time.Now()

	// Validate the pattern once up front rather than once per snapshot, so a
	// malformed glob fails immediately instead of after a full cluster scan.
	if filter.NamePattern != "" {
		if _, err := filepath.Match(filter.NamePattern, "probe"); err != nil {
			return nil, fmt.Errorf("invalid --name-pattern %q: %w", filter.NamePattern, err)
		}
	}

	result := &ScanResult{Failed: make(map[string]error)}

	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan *vm.VM)

	worker := func() {
		defer wg.Done()
		for vmInstance := range jobs {
			snaps, err := ops.GetSnapshots(vmInstance.VMID)

			mu.Lock()
			if err != nil {
				result.Failed[vmInstance.VMID] = err
				mu.Unlock()
				continue
			}
			result.Scanned++

			matched := 0
			for _, snap := range snaps {
				// "current" is the live state, not a snapshot.
				if snap.Name == "current" {
					continue
				}
				ok, matchErr := filter.matches(snap, now)
				if matchErr != nil || !ok {
					continue
				}
				result.Snapshots = append(result.Snapshots, ClusterSnapshot{
					VM:       vmInstance,
					Snapshot: snap,
				})
				matched++
			}
			if matched > 0 {
				result.WithSnapshots++
			}
			mu.Unlock()
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	for _, vmInstance := range vms {
		jobs <- vmInstance
	}
	close(jobs)
	wg.Wait()

	// Stable ordering: VMID ascending (numerically), then newest snapshot first.
	sort.Slice(result.Snapshots, func(i, j int) bool {
		a, b := result.Snapshots[i], result.Snapshots[j]
		if a.VM.VMID != b.VM.VMID {
			ai, errA := strconv.Atoi(a.VM.VMID)
			bi, errB := strconv.Atoi(b.VM.VMID)
			if errA == nil && errB == nil {
				return ai < bi
			}
			return a.VM.VMID < b.VM.VMID
		}
		return a.Snapshot.SnapTime > b.Snapshot.SnapTime
	})

	return result, nil
}

// FormatAge renders a snapshot age compactly for table output.
func FormatAge(createdAt time.Time, snapTime int64, now time.Time) string {
	if snapTime <= 0 {
		return "unknown"
	}

	d := now.Sub(createdAt)
	if d < 0 {
		return "0m"
	}

	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// DisplayClusterSnapshots renders a scan as a flat table.
//
// The trailing summary line always reports the denominator (VMs scanned, VMs
// with none) and any read failures, so an empty table can be read as "nothing
// to do" rather than "something went wrong and I could not tell".
func DisplayClusterSnapshots(result *ScanResult) {
	now := time.Now()

	if len(result.Snapshots) > 0 {
		fmt.Printf("%-9s %-34s %-16s %-42s %-8s %s\n",
			"VMID", "NAME", "NODE", "SNAPSHOT", "AGE", "RAM")
		fmt.Println(strings.Repeat("-", 118))

		for _, cs := range result.Snapshots {
			ram := "no"
			if cs.Snapshot.VMState {
				ram = "yes"
			}
			fmt.Printf("%-9s %-34s %-16s %-42s %-8s %s\n",
				cs.VM.VMID,
				truncate(cs.VM.Name, 34),
				truncate(cs.VM.Node, 16),
				truncate(cs.Snapshot.Name, 42),
				FormatAge(cs.Snapshot.CreatedAt, cs.Snapshot.SnapTime, now),
				ram)
		}
		fmt.Println()
	}

	withNone := result.Scanned - result.WithSnapshots
	fmt.Printf("%d snapshot(s) across %d VM(s) (%d VMs scanned, %d with none)\n",
		len(result.Snapshots), result.WithSnapshots, result.Scanned, withNone)

	if len(result.Failed) > 0 {
		fmt.Printf("\n⚠️  %d VM(s) could not be read — this view is INCOMPLETE:\n", len(result.Failed))
		vmids := make([]string, 0, len(result.Failed))
		for vmid := range result.Failed {
			vmids = append(vmids, vmid)
		}
		sort.Strings(vmids)
		for _, vmid := range vmids {
			fmt.Printf("   VM %s: %v\n", vmid, result.Failed[vmid])
		}
	}
}

// truncate shortens s to at most n characters, marking any elision.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
