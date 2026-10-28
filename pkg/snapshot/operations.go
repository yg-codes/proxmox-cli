package snapshot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// Snapshot represents a Proxmox VM snapshot
type Snapshot struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	VMState     bool      `json:"vmstate"`
	Parent      string    `json:"parent"`
	SnapTime    int64     `json:"snaptime"`
	CreatedAt   time.Time `json:"created_at"`
}

// Operations handles snapshot operations
type Operations struct {
	client *api.Client
	vmOps  *vm.Operations
	logger *logrus.Logger

	// Configuration
	maxSnapshotNameLength int
	vmstateKeywords       []string
}

// NewOperations creates a new snapshot operations instance
func NewOperations(client *api.Client, vmOps *vm.Operations, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client:                client,
		vmOps:                 vmOps,
		logger:                logger,
		maxSnapshotNameLength: 40,
		vmstateKeywords: []string{
			"vmstate", "RAM", "with vmstate", "RAM included",
			"with VM state", "VM state included",
		},
	}
}

// GetSnapshots retrieves all snapshots for a VM
func (ops *Operations) GetSnapshots(vmid string) ([]*Snapshot, error) {
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return nil, fmt.Errorf("VM %s not found: %w", vmid, err)
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/snapshot", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots for VM %s: %w", vmid, err)
	}

	var snapshots []*Snapshot
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if snapMap, ok := item.(map[string]interface{}); ok {
				snapshot := &Snapshot{}

				if name, ok := snapMap["name"].(string); ok {
					snapshot.Name = name
				}
				if desc, ok := snapMap["description"].(string); ok {
					snapshot.Description = desc
				}
				if vmstate, ok := snapMap["vmstate"].(float64); ok {
					snapshot.VMState = vmstate == 1
				}
				if parent, ok := snapMap["parent"].(string); ok {
					snapshot.Parent = parent
				}
				if snaptime, ok := snapMap["snaptime"].(float64); ok {
					snapshot.SnapTime = int64(snaptime)
					snapshot.CreatedAt = time.Unix(int64(snaptime), 0)
				}

				snapshots = append(snapshots, snapshot)
			}
		}
	}

	// Sort snapshots by creation time (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].SnapTime > snapshots[j].SnapTime
	})

	return snapshots, nil
}

// CreateSnapshot creates a new snapshot for a VM
func (ops *Operations) CreateSnapshot(ctx context.Context, vmid, nameOrPrefix string, useExactName, saveVMState bool) error {
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return fmt.Errorf("VM %s not found: %w", vmid, err)
	}

	// Get VM info for naming
	vmInfo, err := ops.vmOps.GetVMStatus(vmid)
	if err != nil {
		ops.logger.Warnf("Could not get VM info for naming: %v", err)
	}

	var snapshotName string
	if useExactName {
		snapshotName = ops.validateSnapshotName(nameOrPrefix)
	} else {
		snapshotName = ops.generateSnapshotName(nameOrPrefix, vmInfo)
	}

	// Check for vmstate keywords in the name/prefix
	if !saveVMState {
		for _, keyword := range ops.vmstateKeywords {
			if strings.Contains(strings.ToLower(nameOrPrefix), strings.ToLower(keyword)) {
				saveVMState = true
				ops.logger.Infof("Detected vmstate keyword '%s' in name, enabling vmstate", keyword)
				break
			}
		}
	}

	// Check if snapshot already exists
	existingSnapshots, _ := ops.GetSnapshots(vmid)
	for _, snap := range existingSnapshots {
		if snap.Name == snapshotName {
			return fmt.Errorf("snapshot '%s' already exists for VM %s", snapshotName, vmid)
		}
	}

	// Create snapshot
	ops.logger.Infof("Creating snapshot '%s' for VM %s (vmstate: %v)", snapshotName, vmid, saveVMState)

	data := url.Values{
		"snapname": {snapshotName},
		"vmstate":  {fmt.Sprintf("%d", boolToInt(saveVMState))},
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/snapshot", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Monitor the task if UPID is returned
	if upid, ok := resp["data"].(string); ok && upid != "" {
		ops.logger.Infof("Monitoring snapshot creation task...")
		if err := ops.vmOps.MonitorTask(ctx, node, upid); err != nil {
			return fmt.Errorf("snapshot creation failed: %w", err)
		}
		ops.logger.Infof("✅ Snapshot '%s' created successfully for VM %s", snapshotName, vmid)
	} else {
		ops.logger.Infof("✅ Snapshot '%s' created for VM %s", snapshotName, vmid)
	}

	return nil
}

// DeleteSnapshot deletes a snapshot from a VM
func (ops *Operations) DeleteSnapshot(ctx context.Context, vmid, snapshotName string) error {
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return fmt.Errorf("VM %s not found: %w", vmid, err)
	}

	// Verify snapshot exists
	snapshots, err := ops.GetSnapshots(vmid)
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	found := false
	for _, snap := range snapshots {
		if snap.Name == snapshotName {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("snapshot '%s' not found for VM %s", snapshotName, vmid)
	}

	ops.logger.Infof("Deleting snapshot '%s' from VM %s", snapshotName, vmid)

	path := fmt.Sprintf("/nodes/%s/qemu/%s/snapshot/%s", node, vmid, snapshotName)
	resp, err := ops.client.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	// Monitor the task if UPID is returned
	if upid, ok := resp["data"].(string); ok && upid != "" {
		ops.logger.Infof("Monitoring snapshot deletion task...")
		if err := ops.vmOps.MonitorTask(ctx, node, upid); err != nil {
			return fmt.Errorf("snapshot deletion failed: %w", err)
		}
		ops.logger.Infof("✅ Snapshot '%s' deleted successfully from VM %s", snapshotName, vmid)
	} else {
		ops.logger.Infof("✅ Snapshot '%s' deleted from VM %s", snapshotName, vmid)
	}

	return nil
}

// RollbackSnapshot rolls back a VM to a specific snapshot
func (ops *Operations) RollbackSnapshot(ctx context.Context, vmid, snapshotName string) error {
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return fmt.Errorf("VM %s not found: %w", vmid, err)
	}

	// Verify snapshot exists
	snapshots, err := ops.GetSnapshots(vmid)
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	found := false
	for _, snap := range snapshots {
		if snap.Name == snapshotName {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("snapshot '%s' not found for VM %s", snapshotName, vmid)
	}

	ops.logger.Infof("Rolling back VM %s to snapshot '%s'", vmid, snapshotName)

	data := url.Values{
		"snapname": {snapshotName},
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/snapshot/%s/rollback", node, vmid, snapshotName)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return fmt.Errorf("failed to rollback snapshot: %w", err)
	}

	// Monitor the task if UPID is returned
	if upid, ok := resp["data"].(string); ok && upid != "" {
		ops.logger.Infof("Monitoring snapshot rollback task...")
		if err := ops.vmOps.MonitorTask(ctx, node, upid); err != nil {
			return fmt.Errorf("snapshot rollback failed: %w", err)
		}
		ops.logger.Infof("✅ VM %s rolled back to snapshot '%s' successfully", vmid, snapshotName)
	} else {
		ops.logger.Infof("✅ VM %s rolled back to snapshot '%s'", vmid, snapshotName)
	}

	return nil
}

// ListSnapshots displays snapshots for a VM in a formatted way
func (ops *Operations) ListSnapshots(vmid string) error {
	snapshots, err := ops.GetSnapshots(vmid)
	if err != nil {
		return err
	}

	if len(snapshots) == 0 {
		fmt.Printf("No snapshots found for VM %s\n", vmid)
		return nil
	}

	// Get VM info for display
	vmInfo, err := ops.vmOps.GetVMStatus(vmid)
	if err != nil {
		ops.logger.Warnf("Could not get VM info: %v", err)
	}

	// Display VM info
	if vmInfo != nil {
		fmt.Printf("VM %s: %s\n", vmid, vmInfo.Name)
		status := "🟢 running"
		if !vmInfo.Running {
			status = "🔴 stopped"
		}
		fmt.Printf("Status: %s\n", status)
	} else {
		fmt.Printf("VM %s\n", vmid)
	}

	// "current" is a pseudo-entry for the live state, not a real snapshot.
	// Filter it out before counting, or a VM with no snapshots reports
	// "1 total" and then lists nothing.
	real := make([]*Snapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.Name != "current" {
			real = append(real, snap)
		}
	}

	if len(real) == 0 {
		fmt.Printf("\nNo snapshots found for VM %s\n", vmid)
		return nil
	}

	fmt.Printf("\nSnapshots (%d total):\n", len(real))
	fmt.Println("=====================")

	for i, snap := range real {
		fmt.Printf("%d. %s\n", i+1, snap.Name)
		if snap.Description != "" {
			fmt.Printf("   Description: %s\n", snap.Description)
		}
		fmt.Printf("   Created: %s\n", snap.CreatedAt.Format("2006-01-02 15:04:05"))
		if snap.VMState {
			fmt.Printf("   VM State: ✅ Included (with RAM)\n")
		} else {
			fmt.Printf("   VM State: ❌ Not included (disk only)\n")
		}
		fmt.Println()
	}

	return nil
}

// DeleteAllSnapshots deletes all snapshots from a VM
func (ops *Operations) DeleteAllSnapshots(ctx context.Context, vmid string) error {
	snapshots, err := ops.GetSnapshots(vmid)
	if err != nil {
		return err
	}

	var snapshotsToDelete []*Snapshot
	for _, snap := range snapshots {
		if snap.Name != "current" {
			snapshotsToDelete = append(snapshotsToDelete, snap)
		}
	}

	if len(snapshotsToDelete) == 0 {
		ops.logger.Infof("No snapshots to delete for VM %s", vmid)
		return nil
	}

	ops.logger.Infof("Deleting %d snapshots from VM %s", len(snapshotsToDelete), vmid)

	var failures []error
	for _, snap := range snapshotsToDelete {
		if err := ops.DeleteSnapshot(ctx, vmid, snap.Name); err != nil {
			ops.logger.Errorf("Failed to delete snapshot '%s': %v", snap.Name, err)
			failures = append(failures, fmt.Errorf("snapshot '%s': %w", snap.Name, err))
			continue
		}
		ops.logger.Infof("Deleted snapshot '%s'", snap.Name)
	}

	// Attempt every deletion, then report: a partially-completed delete-all
	// must not read as success (the handler joins this with other VMs'
	// outcomes to decide the process exit code).
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d snapshot deletion(s) failed: %w", len(failures), len(snapshotsToDelete), errors.Join(failures...))
	}
	return nil
}

// generateSnapshotName generates an intelligent snapshot name
func (ops *Operations) generateSnapshotName(prefix string, vmInfo *vm.VM) string {
	timestamp := time.Now().Format("20060102-1504")

	var nameParts []string
	nameParts = append(nameParts, prefix)

	// Add VM name if available and different from prefix
	if vmInfo != nil && vmInfo.Name != "" && !strings.Contains(prefix, vmInfo.Name) {
		nameParts = append(nameParts, vmInfo.Name)
	}

	nameParts = append(nameParts, timestamp)

	fullName := strings.Join(nameParts, "-")
	return ops.validateSnapshotName(fullName)
}

// validateSnapshotName validates and cleans up snapshot name
func (ops *Operations) validateSnapshotName(name string) string {
	// Remove invalid characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	cleaned := reg.ReplaceAllString(name, "-")

	// Remove multiple consecutive dashes
	reg2 := regexp.MustCompile(`-+`)
	cleaned = reg2.ReplaceAllString(cleaned, "-")

	// Trim dashes from start and end
	cleaned = strings.Trim(cleaned, "-")

	// Ensure maximum length
	if len(cleaned) > ops.maxSnapshotNameLength {
		cleaned = cleaned[:ops.maxSnapshotNameLength]
		cleaned = strings.TrimRight(cleaned, "-")
	}

	// Ensure minimum length
	if len(cleaned) == 0 {
		cleaned = "snapshot-" + time.Now().Format("20060102-1504")
	}

	return cleaned
}

// GetSnapshotConfig retrieves the configuration of a specific snapshot
func (ops *Operations) GetSnapshotConfig(vmid, snapshotName string) (map[string]interface{}, error) {
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return nil, fmt.Errorf("VM %s not found: %w", vmid, err)
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/snapshot/%s/config", node, vmid, snapshotName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot config: %w", err)
	}

	return resp, nil
}

// CompareSnapshots compares the configuration between two snapshots
func (ops *Operations) CompareSnapshots(vmid, snap1, snap2 string) error {
	config1, err := ops.GetSnapshotConfig(vmid, snap1)
	if err != nil {
		return fmt.Errorf("failed to get config for snapshot '%s': %w", snap1, err)
	}

	config2, err := ops.GetSnapshotConfig(vmid, snap2)
	if err != nil {
		return fmt.Errorf("failed to get config for snapshot '%s': %w", snap2, err)
	}

	fmt.Printf("Comparing snapshots '%s' and '%s' for VM %s:\n", snap1, snap2, vmid)
	fmt.Println(strings.Repeat("=", 50))

	// Compare key configuration items
	keys := []string{"memory", "cores", "sockets", "ostype", "bootdisk", "net0", "scsi0"}

	for _, key := range keys {
		val1, ok1 := config1[key]
		val2, ok2 := config2[key]

		if !ok1 && !ok2 {
			continue
		}

		if !ok1 {
			fmt.Printf("%-10s: [MISSING] vs %v\n", key, val2)
		} else if !ok2 {
			fmt.Printf("%-10s: %v vs [MISSING]\n", key, val1)
		} else if fmt.Sprintf("%v", val1) != fmt.Sprintf("%v", val2) {
			fmt.Printf("%-10s: %v vs %v\n", key, val1, val2)
		} else {
			fmt.Printf("%-10s: %v (same)\n", key, val1)
		}
	}

	return nil
}

// boolToInt converts boolean to integer (0/1)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
