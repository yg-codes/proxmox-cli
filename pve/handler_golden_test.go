package main

// Golden handler tests (issue #27): each maps to a remediation finding and
// pins the user-visible contract through the REAL command tree —
// rootCmd.Execute → initializeApp → RunE — against the harness fixture.
//
// These are regression nets for existing behavior: they must pass on main,
// and the sabotage acceptance (recorded in the issue) requires that each can
// be made to fail by reverting the specific fix it guards.

import (
	"strings"
	"testing"
	"time"
)

// 1. Bulk start, one of three VMs failing server-side: the command must exit
// non-zero naming the failed VM, print the full summary first, and actually
// have started the two healthy VMs server-side.
func TestHandlerBulkStartPartialFailure(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "stopped")
	f.addVM("101", "vm-101", "stopped")
	f.addVM("102", "vm-102", "stopped")
	f.failStart["102"] = true

	out, err := execCLI(t, f, "vm", "start", "--vmid", "100,101,102", "-y")

	if err == nil {
		t.Fatal("vm start with one failed target: err=nil, want non-zero exit")
	}
	if !strings.Contains(err.Error(), "102") {
		t.Errorf("exit error %q does not name failed VM 102", err)
	}
	if !strings.Contains(out, "BULK OPERATION SUMMARY") {
		t.Error("summary not printed (summary-first contract violated)")
	}
	for _, vmid := range []string{"100", "101"} {
		if status, ok := f.vmStatus(vmid); !ok || status != "running" {
			t.Errorf("VM %s status=%q ok=%v, want running (healthy targets must complete)", vmid, status, ok)
		}
	}
	if status, _ := f.vmStatus("102"); status != "stopped" {
		t.Errorf("VM 102 status=%q, want stopped (failed start must not mutate)", status)
	}
}

// 2. Multi-VM backup loop, one of two failing: non-zero exit naming the VM,
// per-target output and the x/y count line printed, both backups attempted.
func TestHandlerBackupCreatePartialFailure(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "running")
	f.addVM("101", "vm-101", "running")
	f.failVzdump["101"] = true

	out, err := execCLI(t, f, "vm", "backup", "create",
		"--vmid", "100,101", "--storage", "local", "--mode", "snapshot", "-y")

	if err == nil {
		t.Fatal("backup create with one failed target: err=nil, want non-zero exit")
	}
	if !strings.Contains(err.Error(), "101") {
		t.Errorf("exit error %q does not name failed VM 101", err)
	}
	for _, want := range []string{"Creating backups for 2 VMs", "🔄 Creating backup"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q", want)
		}
	}
	if f.vzdumpCalls["100"] != 1 || f.vzdumpCalls["101"] != 1 {
		t.Errorf("vzdump calls: 100=%d 101=%d, want 1/1 (loop must attempt all targets)",
			f.vzdumpCalls["100"], f.vzdumpCalls["101"])
	}
}

// 3. Snapshot delete --all, one VM's deletion failing: joined error names the
// VM; the other VM's snapshots are actually gone server-side.
func TestHandlerSnapshotDeleteAllPartialFailure(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "running", "snap-a", "snap-b")
	f.addVM("101", "vm-101", "running", "snap-a")
	f.failSnapDel["101:snap-a"] = true

	out, err := execCLI(t, f, "vm", "snapshot", "delete", "--vmid", "100,101", "--all", "-y")

	if err == nil {
		t.Fatal("snapshot delete --all with one failed VM: err=nil, want non-zero exit")
	}
	if !strings.Contains(err.Error(), "101") {
		t.Errorf("exit error %q does not name failed VM 101", err)
	}
	if !strings.Contains(out, "Failed to delete all snapshots") {
		t.Error("stdout missing the per-VM failure line")
	}
	if got := f.snaps("100"); len(got) != 0 {
		t.Errorf("VM 100 snaps=%v, want all deleted", got)
	}
	if got := f.snaps("101"); len(got) != 1 {
		t.Errorf("VM 101 snaps=%v, want failed deletion preserved", got)
	}
}

// 4. Backup delete with two --vmid values: rejected with the exactly-one
// error BEFORE any API call reaches the server.
func TestHandlerBackupDeleteRejectsMultipleVMIDs(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "stopped")

	out, err := execCLI(t, f, "vm", "backup", "delete",
		"--vmid", "100,101", "--pattern", "nomatch*", "-y")

	if err == nil {
		t.Fatal("backup delete with 2 VMIDs: err=nil, want exactly-one error")
	}
	if !strings.Contains(err.Error(), "exactly one VM ID") {
		t.Errorf("exit error %q does not state the single-ID requirement", err)
	}
	if n := f.requests(); n != 0 {
		t.Errorf("fixture saw %d requests, want 0 (rejection must precede any API call)", n)
	}
	_ = out
}

// 5. Snapshot delete of multiple named snapshots, one failing: the joined
// error names the failing snapshot; the healthy one is deleted server-side.
func TestHandlerSnapshotDeleteSpecificPartialFailure(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "running", "keep-me", "kill-me")
	f.failSnapDel["100:kill-me"] = true

	_, err := execCLI(t, f, "vm", "snapshot", "delete",
		"--vmid", "100", "--snapshot", "keep-me,kill-me", "-y")

	if err == nil {
		t.Fatal("snapshot delete with one failed snapshot: err=nil, want non-zero exit")
	}
	if !strings.Contains(err.Error(), "kill-me") {
		t.Errorf("exit error %q does not name failed snapshot kill-me", err)
	}
	if got := f.snaps("100"); len(got) != 1 || got[0] != "kill-me" {
		t.Errorf("VM 100 snaps=%v, want only kill-me left (healthy deletion preserved)", got)
	}
}

// 6. Cancellation mid-bulk (the in-process Manager.Cancel flag — the SIGINT
// path itself exits the process and is untestable here): server-side delays
// guarantee work remains when Cancel fires after the first observed start;
// the command must exit non-zero with the summary still printed.
func TestHandlerBulkStartCancelledMidFlight(t *testing.T) {
	f := newAPIFixture(t)
	for i := 100; i < 110; i++ {
		f.addVM(itoa(i), "vm", "stopped")
	}
	f.startDelay = 120 * time.Millisecond // 10 VMs / 3 workers ⇒ ≥480ms server-side

	cancelAfterFirstStart := func(vmid string) {
		t.Logf("cancelling after first observed start (vm %s)", vmid)
		bulkMgr.Cancel()
	}
	f.onStartHook = cancelAfterFirstStart

	res := <-execCLIAsync(t, f, "vm", "start", "--vmid", "100-102,103-109", "-y")

	if res.err == nil {
		t.Fatal("cancelled bulk start: err=nil, want non-zero exit")
	}
	// The harness exercises the Cancel()-only path (SIGINT would exit the
	// process), so the contract string is deterministic — see the split
	// described in the Error and Exit Contract.
	if !strings.Contains(res.err.Error(), "operation cancelled after") {
		t.Errorf("err=%q, want the Cancel()-only contract string", res.err)
	}
	if !strings.Contains(res.stdout, "BULK OPERATION SUMMARY") {
		t.Error("summary not printed on cancellation")
	}
	started, notStarted := 0, 0
	for i := 100; i < 110; i++ {
		if s, _ := f.vmStatus(itoa(i)); s == "running" {
			started++
		} else {
			notStarted++
		}
	}
	if started == 0 {
		t.Error("no VM started before cancellation — fixture delay not exercised")
	}
	if notStarted == 0 {
		t.Error("all VMs completed before Cancel() — cancellation not actually mid-flight")
	}
}

// 7. Multi-VM shutdown loop, one failing: the counter-based error contract.
func TestHandlerShutdownPartialFailure(t *testing.T) {
	f := newAPIFixture(t)
	f.addVM("100", "vm-100", "running")
	f.addVM("101", "vm-101", "running")
	f.failShutdown["101"] = true

	out, err := execCLI(t, f, "vm", "shutdown", "--vmid", "100,101", "-y")

	if err == nil {
		t.Fatal("shutdown with one failed VM: err=nil, want non-zero exit")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("exit error %q does not carry the failure count", err)
	}
	if !strings.Contains(out, "VM 100 shutdown initiated") {
		t.Error("stdout missing the successful shutdown line")
	}
	if s, _ := f.vmStatus("100"); s != "stopped" {
		t.Errorf("VM 100 status=%q, want stopped", s)
	}
	if s, _ := f.vmStatus("101"); s != "running" {
		t.Errorf("VM 101 status=%q, want running (failed shutdown must not mutate)", s)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}
