package protection

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// Operations handles VM protection operations
type Operations struct {
	client *api.Client
	vmOps  *vm.Operations
	logger *logrus.Logger
}

// NewOperations creates a new protection operations handler
func NewOperations(client *api.Client, vmOps *vm.Operations, logger *logrus.Logger) *Operations {
	return &Operations{
		client: client,
		vmOps:  vmOps,
		logger: logger,
	}
}

// IsProtected checks if a VM is protected
func (ops *Operations) IsProtected(vmid string) (bool, error) {
	// Find VM node
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return false, fmt.Errorf("failed to find VM node: %w", err)
	}

	// Get VM config
	resp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/qemu/%s/config", node, vmid), nil)
	if err != nil {
		return false, fmt.Errorf("failed to get VM config: %w", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid config response")
	}

	// Check protection field
	if protection, ok := data["protection"]; ok {
		switch v := protection.(type) {
		case float64:
			return v == 1, nil
		case string:
			return v == "1", nil
		case bool:
			return v, nil
		}
	}

	return false, nil
}

// CheckAndWarn checks VM protection and warns the user
func (ops *Operations) CheckAndWarn(vmid string, operation string) (bool, error) {
	protected, err := ops.IsProtected(vmid)
	if err != nil {
		return false, err
	}

	if protected {
		fmt.Printf("\n⚠️  WARNING: VM %s is PROTECTED!\n", vmid)
		fmt.Printf("  This operation (%s) may affect a protected VM.\n", operation)
		fmt.Printf("  Protected VMs require extra caution.\n")
		return true, nil
	}

	return false, nil
}

// SetProtection sets or unsets VM protection
func (ops *Operations) SetProtection(vmid string, protect bool) error {
	// Find VM node
	node, err := ops.vmOps.FindVMNode(vmid)
	if err != nil {
		return fmt.Errorf("failed to find VM node: %w", err)
	}

	value := "0"
	if protect {
		value = "1"
	}

	data := map[string]interface{}{
		"protection": value,
	}

	_, err = ops.client.Put(fmt.Sprintf("/nodes/%s/qemu/%s/config", node, vmid), data)
	if err != nil {
		return fmt.Errorf("failed to set protection: %w", err)
	}

	if protect {
		fmt.Printf("✅ VM %s is now protected\n", vmid)
	} else {
		fmt.Printf("✅ VM %s protection removed\n", vmid)
	}

	return nil
}

// CheckAndOfferDisable checks VM protection and offers to disable it
// Returns: (isProtected, shouldProceed, error)
// isProtected: true if VM was protected
// shouldProceed: true if operation should proceed (either not protected or user chose to disable)
func (ops *Operations) CheckAndOfferDisable(vmid string, operation string) (bool, bool, error) {
	protected, err := ops.IsProtected(vmid)
	if err != nil {
		return false, false, err
	}

	if !protected {
		return false, true, nil
	}

	fmt.Printf("\n⚠️  VM PROTECTION DETECTED\n")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("VM %s has protection mode enabled, which prevents:\n", vmid)
	fmt.Println("  • VM deletion")
	fmt.Println("  • Configuration changes")
	fmt.Printf("  • %s operations\n", operation)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("1. Disable protection and continue")
	fmt.Println("2. Cancel operation")
	fmt.Println()

	fmt.Print("Select option (1-2): ")
	var choice string
	_, _ = fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	if choice == "1" {
		fmt.Printf("\n🔓 Disabling VM protection for %s...\n", vmid)
		if err := ops.SetProtection(vmid, false); err != nil {
			return true, false, fmt.Errorf("failed to disable protection: %w", err)
		}
		fmt.Println("✅ VM protection disabled successfully")
		return true, true, nil
	}

	fmt.Println("Operation cancelled")
	return true, false, nil
}
