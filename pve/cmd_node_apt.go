package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/apt"
)

var (
	aptOps        *apt.Operations
	aptNodeFlag   string
	aptHandleFlag string
	aptPathFlag   string
	aptIndexFlag  int
	aptDigestFlag string
)

// nodeAptCmd manages APT repositories via /nodes/{node}/apt/repositories.
// Reads need Sys.Audit on /nodes; add/toggle need Sys.Modify on /nodes/{node}.
var nodeAptCmd = &cobra.Command{
	Use:   "apt",
	Short: "APT repository management",
	Long: `Manage APT repositories on a node via /nodes/{node}/apt/repositories.

Reading needs Sys.Audit on /nodes. Adding or toggling repositories needs
Sys.Modify on /nodes/{node} — note PVEAdmin does NOT include Sys.Modify; use
PVESysAdmin or grant Sys.Modify directly.

API shape worth knowing:
  - 'repos add' only accepts standard handles (listed by 'repos list');
    arbitrary repo URLs cannot be added through the API.
  - Existing entries can be enabled/disabled but NOT deleted — removing a
    line means editing the file on the node.
  - 'repos add' on a handle that is configured but disabled enables it
    (this is also how the web UI enables standard repos).

Examples:
  pve node apt repos list --node pve1
  pve node apt repos add --node pve1 --handle no-subscription
  pve node apt repos enable --node pve1 --handle enterprise
  pve node apt repos disable --node pve1 --handle ceph-tentacle-no-subscription
  pve node apt repos disable --node pve1 --path /etc/apt/sources.list.d/ceph.sources --index 0`,
}

var aptReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List and manage APT repositories",
	Long:  `List configured repositories, add standard handles, and enable/disable entries.`,
}

var aptReposListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured repositories and standard handles",
	Long: `Show every configured repository entry with its file and index (what
--path/--index refer to), plus the standard handles 'repos add' accepts and
any warnings the node reports about its apt configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if aptNodeFlag == "" {
			return fmt.Errorf("--node flag is required")
		}
		repos, err := aptOps.GetRepositories(aptNodeFlag)
		if err != nil {
			return err
		}
		apt.DisplayRepositories(repos)
		return nil
	},
}

var aptReposAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a standard repository (enables it if already configured but disabled)",
	Long: `Add a standard repository by handle, e.g. no-subscription, enterprise,
test, ceph-tentacle-no-subscription. Valid handles are listed by 'repos list'.
If the repository is already configured but disabled, the API enables it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if aptNodeFlag == "" || aptHandleFlag == "" {
			return fmt.Errorf("both --node and --handle flags are required")
		}

		repos, err := aptOps.GetRepositories(aptNodeFlag)
		if err != nil {
			return err
		}

		if !isStandardHandle(repos, aptHandleFlag) {
			return fmt.Errorf("%q is not a standard repository handle on node %s — run 'repos list' to see valid handles; arbitrary URLs cannot be added via the API",
				aptHandleFlag, aptNodeFlag)
		}

		// Already configured and enabled? Then there is nothing to add.
		if r, err := apt.ResolveHandle(repos, aptHandleFlag); err == nil && r.Enabled {
			fmt.Printf("✅ %s is already configured and enabled at %s index %d — nothing to do\n",
				aptHandleFlag, r.FilePath, r.Index)
			return nil
		}

		digest := aptDigestFlag
		if digest == "" {
			digest = repos.Digest
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would add/enable standard repository %q on node %s\n", aptHandleFlag, aptNodeFlag)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  Add/enable standard repository %q on node %s? (yes/no): ", aptHandleFlag, aptNodeFlag)
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		if err := aptOps.AddRepository(aptNodeFlag, aptHandleFlag, digest); err != nil {
			return err
		}
		fmt.Printf("✅ Added/enabled standard repository %q on node %s\n", aptHandleFlag, aptNodeFlag)
		fmt.Println("Note: run 'apt update' on the node (or pve node apt repos list to verify) to refresh package indexes")
		return nil
	},
}

var aptReposEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable a repository entry",
	Long: `Enable a repository entry, addressed either by its standard handle or
directly by --path and --index (as shown by 'repos list').`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoToggle(true)
	},
}

var aptReposDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable a repository entry",
	Long: `Disable a repository entry, addressed either by its standard handle or
directly by --path and --index (as shown by 'repos list').`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoToggle(false)
	},
}

// runRepoToggle implements enable/disable: resolve the target (handle or
// path+index), skip if already in the desired state, confirm, then PUT with
// the current digest.
func runRepoToggle(enabled bool) error {
	if aptNodeFlag == "" {
		return fmt.Errorf("--node flag is required")
	}
	if aptHandleFlag == "" && aptPathFlag == "" {
		return fmt.Errorf("either --handle or --path (with --index) is required")
	}
	if aptHandleFlag != "" && aptPathFlag != "" {
		return fmt.Errorf("--handle and --path are mutually exclusive")
	}

	repos, err := aptOps.GetRepositories(aptNodeFlag)
	if err != nil {
		return err
	}

	var target *apt.Repository
	if aptHandleFlag != "" {
		target, err = apt.ResolveHandle(repos, aptHandleFlag)
		if err != nil {
			return err
		}
	} else {
		if aptIndexFlag < 0 {
			return fmt.Errorf("--index is required together with --path (see 'repos list' for indexes)")
		}
		target = findRepoEntry(repos, aptPathFlag, aptIndexFlag)
		if target == nil {
			return fmt.Errorf("no repository entry at %s index %s on node %s — check 'repos list'",
				aptPathFlag, strconv.Itoa(aptIndexFlag), aptNodeFlag)
		}
	}

	verb := "Enable"
	if !enabled {
		verb = "Disable"
	}

	fmt.Printf("%s:\n", verb)
	apt.DisplayRepository(target)

	if target.Enabled == enabled {
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		fmt.Printf("✅ Already %s — nothing to do\n", state)
		return nil
	}

	digest := aptDigestFlag
	if digest == "" {
		digest = repos.Digest
	}

	if dryRun {
		fmt.Printf("🔍 DRY-RUN: Would %s %s index %d on node %s\n",
			strings.ToLower(verb), target.FilePath, target.Index, aptNodeFlag)
		return nil
	}

	if !autoConfirm && !batchMode {
		fmt.Printf("⚠️  %s this repository on node %s? (yes/no): ", verb, aptNodeFlag)
		if !confirmAction() {
			fmt.Println("❌ Operation cancelled")
			return nil
		}
	}

	if err := aptOps.SetRepositoryEnabled(aptNodeFlag, target.FilePath, target.Index, enabled, digest); err != nil {
		return err
	}
	fmt.Printf("✅ %sd %s index %d on node %s\n", verb, target.FilePath, target.Index, aptNodeFlag)
	return nil
}

func isStandardHandle(repos *apt.Repositories, handle string) bool {
	for _, s := range repos.StandardRepos {
		if s.Handle == handle {
			return true
		}
	}
	return false
}

func findRepoEntry(repos *apt.Repositories, path string, index int) *apt.Repository {
	for _, f := range repos.Files {
		if f.Path != path {
			continue
		}
		for _, r := range f.Repos {
			if r.Index == index {
				return r
			}
		}
	}
	return nil
}

func initNodeAptCommand() {
	nodeCmd.AddCommand(nodeAptCmd)
	nodeAptCmd.AddCommand(aptReposCmd)

	aptReposCmd.AddCommand(aptReposListCmd)
	aptReposCmd.AddCommand(aptReposAddCmd)
	aptReposCmd.AddCommand(aptReposEnableCmd)
	aptReposCmd.AddCommand(aptReposDisableCmd)

	aptReposListCmd.Flags().StringVar(&aptNodeFlag, "node", "", "Node name (required)")
	_ = aptReposListCmd.MarkFlagRequired("node")

	aptReposAddCmd.Flags().StringVar(&aptNodeFlag, "node", "", "Node name (required)")
	aptReposAddCmd.Flags().StringVar(&aptHandleFlag, "handle", "", "Standard repository handle, e.g. no-subscription (required)")
	_ = aptReposAddCmd.MarkFlagRequired("node")
	_ = aptReposAddCmd.MarkFlagRequired("handle")

	for _, c := range []*cobra.Command{aptReposEnableCmd, aptReposDisableCmd} {
		c.Flags().StringVar(&aptNodeFlag, "node", "", "Node name (required)")
		c.Flags().StringVar(&aptHandleFlag, "handle", "", "Standard repository handle (mutually exclusive with --path)")
		c.Flags().StringVar(&aptPathFlag, "path", "", "Repository file path, as shown by 'repos list'")
		c.Flags().IntVar(&aptIndexFlag, "index", -1, "Index within the file, as shown by 'repos list'")
		c.Flags().StringVar(&aptDigestFlag, "digest", "", "Digest from a previous 'repos list' (default: fetch fresh)")
		_ = c.MarkFlagRequired("node")
	}

	aptReposAddCmd.Flags().StringVar(&aptDigestFlag, "digest", "", "Digest from a previous 'repos list' (default: fetch fresh)")
}
