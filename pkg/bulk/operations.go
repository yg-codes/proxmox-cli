package bulk

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/snapshot"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// OperationResult represents the result of a bulk operation on a single VM
type OperationResult struct {
	VMID      string        `json:"vmid"`
	VMName    string        `json:"vm_name"`
	Operation string        `json:"operation"`
	Success   bool          `json:"success"`
	Message   string        `json:"message"`
	Duration  time.Duration `json:"duration"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Error     error         `json:"error,omitempty"`
}

// ProgressUpdate represents a progress update for bulk operations
type ProgressUpdate struct {
	Completed  int
	Total      int
	Successful int
	Failed     int
	Current    string
	Progress   float64
}

// Manager handles bulk operations with progress tracking and concurrent execution
type Manager struct {
	vmOps       *vm.Operations
	snapshotOps *snapshot.Operations
	logger      *logrus.Logger
	maxWorkers  int
	// maxVMWorkers bounds VM power operations (start/stop/shutdown/reboot/reset),
	// which are configured separately from snapshot operations.
	maxVMWorkers int

	// Progress tracking
	results      []OperationResult
	resultsChan  chan OperationResult
	progressChan chan ProgressUpdate
	mu           sync.RWMutex
	cancelled    bool
	progressWG   sync.WaitGroup
}

// NewManager creates a new bulk operations manager
func NewManager(vmOps *vm.Operations, snapshotOps *snapshot.Operations, logger *logrus.Logger) *Manager {
	if logger == nil {
		logger = logrus.New()
	}

	return &Manager{
		vmOps:        vmOps,
		snapshotOps:  snapshotOps,
		logger:       logger,
		maxWorkers:   3, // Default concurrent operations
		maxVMWorkers: 3, // Default concurrent VM power operations
		results:      make([]OperationResult, 0),
		resultsChan:  make(chan OperationResult, 100),
		progressChan: make(chan ProgressUpdate, 100),
	}
}

// SetMaxWorkers sets the maximum number of concurrent snapshot workers
func (m *Manager) SetMaxWorkers(maxWorkers int) {
	if maxWorkers > 0 {
		m.maxWorkers = maxWorkers
	}
}

// SetMaxVMWorkers sets the maximum number of concurrent VM power operation workers
func (m *Manager) SetMaxVMWorkers(maxVMWorkers int) {
	if maxVMWorkers > 0 {
		m.maxVMWorkers = maxVMWorkers
	}
}

// Cancel cancels ongoing operations
func (m *Manager) Cancel() {
	m.mu.Lock()
	m.cancelled = true
	m.mu.Unlock()
}

// IsCancelled checks if operations are cancelled
func (m *Manager) IsCancelled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cancelled
}

// GetResults returns a copy of the current results
func (m *Manager) GetResults() []OperationResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resultsCopy := make([]OperationResult, len(m.results))
	copy(resultsCopy, m.results)
	return resultsCopy
}

// GetProgress returns current progress statistics
func (m *Manager) GetProgress() (int, int, int, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	completed := len(m.results)
	successful := 0
	failed := 0

	for _, result := range m.results {
		if result.Success {
			successful++
		} else {
			failed++
		}
	}

	return completed, successful, failed, completed
}

// CreateSnapshots creates snapshots for multiple VMs concurrently
func (m *Manager) CreateSnapshots(ctx context.Context, vms []*vm.VM, nameOrPrefix string, useExactName, saveVMState bool) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk snapshot creation for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxWorkers, func(opCtx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
		m.createSnapshotWorker(opCtx, wg, jobs, nameOrPrefix, useExactName, saveVMState)
	})
}

// DeleteSnapshots deletes snapshots from multiple VMs concurrently
func (m *Manager) DeleteSnapshots(ctx context.Context, vms []*vm.VM, snapshotName string) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk snapshot deletion for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxWorkers, func(opCtx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
		m.deleteSnapshotWorker(opCtx, wg, jobs, snapshotName)
	})
}

// SnapshotRef names one specific snapshot on one specific VM.
type SnapshotRef struct {
	VM           *vm.VM
	SnapshotName string
}

// DeleteSnapshotPairs deletes an explicit list of (VM, snapshot) pairs.
//
// This differs from DeleteSnapshots, which broadcasts a single snapshot name to
// every VM. Pruning needs per-VM target sets — each VM has its own matching
// snapshots — and one VM may legitimately appear several times.
func (m *Manager) DeleteSnapshotPairs(ctx context.Context, pairs []SnapshotRef) error {
	if len(pairs) == 0 {
		return fmt.Errorf("no snapshots provided")
	}

	m.logger.Infof("Starting bulk snapshot deletion for %d snapshot(s)", len(pairs))
	m.resetResults()

	return runOperation(m, ctx, pairs, m.maxWorkers, m.deleteSnapshotPairWorker)
}

func (m *Manager) deleteSnapshotPairWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan SnapshotRef) {
	defer wg.Done()

	for pair := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      pair.VM.VMID,
			VMName:    pair.VM.Name,
			Operation: "delete_snapshot",
			StartTime: time.Now(),
		}

		err := m.snapshotOps.DeleteSnapshot(ctx, pair.VM.VMID, pair.SnapshotName)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = fmt.Sprintf("Snapshot '%s': %v", pair.SnapshotName, err)
		} else {
			result.Message = fmt.Sprintf("Snapshot '%s' deleted successfully", pair.SnapshotName)
		}

		m.resultsChan <- result
	}
}

// RollbackSnapshots rolls back multiple VMs to a snapshot concurrently
func (m *Manager) RollbackSnapshots(ctx context.Context, vms []*vm.VM, snapshotName string) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk snapshot rollback for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxWorkers, func(opCtx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
		m.rollbackSnapshotWorker(opCtx, wg, jobs, snapshotName)
	})
}

// StartVMs starts multiple VMs concurrently
func (m *Manager) StartVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM start for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.startVMWorker)
}

// StopVMs stops multiple VMs concurrently
func (m *Manager) StopVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM stop for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.stopVMWorker)
}

// ShutdownVMs gracefully shuts down multiple VMs concurrently
func (m *Manager) ShutdownVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM shutdown for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.shutdownVMWorker)
}

// RebootVMs reboots multiple VMs concurrently
func (m *Manager) RebootVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM reboot for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.rebootVMWorker)
}

// SuspendVMs suspends multiple VMs concurrently.
//
// toDisk and stateStorage are passed through to vm.Operations.SuspendVM: with
// toDisk false the VMs are paused in RAM, with it true their state is written
// to storage. They are parameters rather than Manager fields so concurrent
// callers cannot race on shared option state.
func (m *Manager) SuspendVMs(ctx context.Context, vms []*vm.VM, toDisk bool, stateStorage string) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM suspend for %d VMs (todisk=%v)", len(vms), toDisk)
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, func(opCtx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
		m.suspendVMWorker(opCtx, wg, jobs, toDisk, stateStorage)
	})
}

// ResumeVMs resumes multiple suspended VMs concurrently
func (m *Manager) ResumeVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM resume for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.resumeVMWorker)
}

// ResetVMs hard-resets multiple VMs concurrently
func (m *Manager) ResetVMs(ctx context.Context, vms []*vm.VM) error {
	if len(vms) == 0 {
		return fmt.Errorf("no VMs provided")
	}

	m.logger.Infof("Starting bulk VM reset for %d VMs", len(vms))
	m.resetResults()

	return runOperation(m, ctx, vms, m.maxVMWorkers, m.resetVMWorker)
}

// Worker functions

func (m *Manager) createSnapshotWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM, nameOrPrefix string, useExactName, saveVMState bool) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "create_snapshot",
			StartTime: time.Now(),
		}

		err := m.snapshotOps.CreateSnapshot(ctx, vmInstance.VMID, nameOrPrefix, useExactName, saveVMState)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "Snapshot created successfully"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) deleteSnapshotWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM, snapshotName string) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "delete_snapshot",
			StartTime: time.Now(),
		}

		err := m.snapshotOps.DeleteSnapshot(ctx, vmInstance.VMID, snapshotName)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = fmt.Sprintf("Snapshot '%s' deleted successfully", snapshotName)
		}

		m.resultsChan <- result
	}
}

func (m *Manager) rollbackSnapshotWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM, snapshotName string) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "rollback_snapshot",
			StartTime: time.Now(),
		}

		err := m.snapshotOps.RollbackSnapshot(ctx, vmInstance.VMID, snapshotName)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = fmt.Sprintf("Rolled back to snapshot '%s' successfully", snapshotName)
		}

		m.resultsChan <- result
	}
}

func (m *Manager) startVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "start_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.StartVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM started successfully"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) stopVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "stop_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.StopVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM stopped successfully"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) shutdownVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "shutdown_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.ShutdownVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM shutdown successfully"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) rebootVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "reboot_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.RebootVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM reboot initiated"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) suspendVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM, toDisk bool, stateStorage string) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "suspend_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.SuspendVM(vmInstance.VMID, toDisk, stateStorage)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else if toDisk {
			result.Message = "VM suspend to disk initiated"
		} else {
			result.Message = "VM paused"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) resumeVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "resume_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.ResumeVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM resumed"
		}

		m.resultsChan <- result
	}
}

func (m *Manager) resetVMWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan *vm.VM) {
	defer wg.Done()

	for vmInstance := range jobs {
		if m.IsCancelled() || ctx.Err() != nil {
			return
		}

		result := OperationResult{
			VMID:      vmInstance.VMID,
			VMName:    vmInstance.Name,
			Operation: "reset_vm",
			StartTime: time.Now(),
		}

		err := m.vmOps.ResetVM(vmInstance.VMID)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = err == nil
		if err != nil {
			result.Error = err
			result.Message = err.Error()
		} else {
			result.Message = "VM reset initiated"
		}

		m.resultsChan <- result
	}
}

// progressMonitor monitors progress and collects results
func (m *Manager) progressMonitor(total int) {
	defer m.progressWG.Done()

	for result := range m.resultsChan {
		m.mu.Lock()
		m.results = append(m.results, result)

		completed := len(m.results)
		successful := 0
		failed := 0

		for _, r := range m.results {
			if r.Success {
				successful++
			} else {
				failed++
			}
		}

		progress := float64(completed) / float64(total) * 100
		m.mu.Unlock()

		// Send progress update
		update := ProgressUpdate{
			Completed:  completed,
			Total:      total,
			Successful: successful,
			Failed:     failed,
			Current:    fmt.Sprintf("VM %s", result.VMID),
			Progress:   progress,
		}

		select {
		case m.progressChan <- update:
		default:
			// Progress channel full, skip
		}

		// Log result
		if result.Success {
			m.logger.Infof("✅ VM %s (%s): %s (%.2fs)", result.VMID, result.VMName, result.Message, result.Duration.Seconds())
		} else {
			m.logger.Errorf("❌ VM %s (%s): %s (%.2fs)", result.VMID, result.VMName, result.Message, result.Duration.Seconds())
		}
	}
}

// aggregateFailures returns nil when every operation succeeded, otherwise
// one error naming each failed target (sorted by VMID for stable output) so
// command handlers can print the per-target summary and still exit non-zero.
func (m *Manager) aggregateFailures() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var msgs []string
	for _, r := range m.results {
		if !r.Success {
			msgs = append(msgs, fmt.Sprintf("%s (%s): %s", r.VMID, r.Operation, r.Message))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	sort.Strings(msgs)
	return fmt.Errorf("%d of %d operation(s) failed: %s", len(msgs), len(m.results), strings.Join(msgs, "; "))
}

// GetProgressChan returns the progress channel for real-time updates
func (m *Manager) GetProgressChan() <-chan ProgressUpdate {
	return m.progressChan
}

// PrintSummary prints a summary of the bulk operation results
func (m *Manager) PrintSummary() {
	results := m.GetResults()
	if len(results) == 0 {
		fmt.Println("No operations completed.")
		return
	}

	// Sort results by VM ID for consistent display
	sort.Slice(results, func(i, j int) bool {
		return results[i].VMID < results[j].VMID
	})

	completed, successful, failed, _ := m.GetProgress()

	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("BULK OPERATION SUMMARY\n")
	fmt.Printf(strings.Repeat("=", 60) + "\n")
	fmt.Printf("Total Operations: %d\n", completed)
	fmt.Printf("Successful: %d (%.1f%%)\n", successful, float64(successful)/float64(completed)*100)
	fmt.Printf("Failed: %d (%.1f%%)\n", failed, float64(failed)/float64(completed)*100)

	if failed > 0 {
		fmt.Printf("\nFAILED OPERATIONS:\n")
		fmt.Printf(strings.Repeat("-", 30) + "\n")
		for _, result := range results {
			if !result.Success {
				fmt.Printf("VM %s (%s): %s\n", result.VMID, result.VMName, result.Message)
			}
		}
	}

	if successful > 0 {
		fmt.Printf("\nSUCCESSFUL OPERATIONS:\n")
		fmt.Printf(strings.Repeat("-", 30) + "\n")
		for _, result := range results {
			if result.Success {
				fmt.Printf("VM %s (%s): %s (%.2fs)\n", result.VMID, result.VMName, result.Message, result.Duration.Seconds())
			}
		}
	}

	fmt.Printf(strings.Repeat("=", 60) + "\n")
}

// resetResults clears previous results and installs fresh channels for the
// operation about to start. The previous operation closed its result channel,
// and a worker sending on a closed channel panics — so channels are recreated
// per operation, never drained and reused. Replacing them also drops any
// results a cancelled operation left buffered.
func (m *Manager) resetResults() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.results = make([]OperationResult, 0)
	m.cancelled = false
	m.resultsChan = make(chan OperationResult, 100)
	m.progressChan = make(chan ProgressUpdate, 100)
}

// runOperation executes one complete bulk-operation lifecycle against targets:
// it starts the progress monitor and a worker pool no larger than the target
// count, submits every target, and finalizes everything the operation created.
//
// Finalization runs on every exit path — including cancellation mid-submission
// — because a Manager is reused across operations: jobs are closed so every
// worker stops, the result channel is closed exactly once after the last
// worker finishes (its closure is also the monitor's completion signal), and
// the monitor is waited for before returning. Channel closure after wg.Wait
// replaces result-count polling: a worker that observes cancellation
// legitimately produces no result, so a counted wait could never terminate.
// A cancelled operation returns ctx.Err() while preserving completed results.
func runOperation[T any](m *Manager, ctx context.Context, targets []T, maxWorkers int, worker func(ctx context.Context, wg *sync.WaitGroup, jobs <-chan T)) error {
	m.progressWG.Add(1)
	go m.progressMonitor(len(targets))

	jobs := make(chan T, len(targets))

	workers := maxWorkers
	if workers > len(targets) {
		workers = len(targets)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs)
	}

	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(m.resultsChan)
			m.progressWG.Wait()
			return ctx.Err()
		}
	}
	close(jobs)

	wg.Wait()
	close(m.resultsChan)
	m.progressWG.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Manager.Cancel() alone (no context cancellation) still means the
	// operation stopped early: report the incompleteness instead of letting
	// an all-successes-so-far run read as success.
	if m.IsCancelled() {
		completed := len(m.GetResults())
		if completed < len(targets) {
			return fmt.Errorf("operation cancelled after %d of %d target(s)", completed, len(targets))
		}
	}
	return m.aggregateFailures()
}
