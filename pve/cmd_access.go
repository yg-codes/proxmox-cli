package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/access"
)

// accessFlags holds the shared flags for the token/user lifecycle subcommands.
var (
	accessUser       string
	accessToken      string
	accessRealm      string
	accessRole       string
	accessNodesAudit bool
	accessDryRun     bool

	// grant-specific flags. The role flag is separate from --role so that
	// 'grant' cannot silently inherit create's PVEVMAdmin default — a
	// permission mutation must name its role explicitly.
	accessPath      string
	accessGrantRole string
	accessToToken   bool
)

// accessCmd is the "access" group: manage API users, tokens, and ACLs via the
// /access/* REST endpoints. It is the no-SSH successor to scripts/pve-token.sh.
//
// Mutations require root-level privileges (User.Modify + Permissions.Modify).
// Authenticate as root@pve via PVE_PASSWORD, not a VM-scoped token.
var accessCmd = &cobra.Command{
	Use:   "access",
	Short: "Manage API users, tokens, and ACLs",
	Long: `Manage Proxmox API users, tokens, and role assignments via the REST API
(the same operations the pveum CLI performs, without SSH).

These commands hit the /access/* endpoints, which require User.Modify and
Permissions.Modify privileges. Authenticate as root@pve (set PVE_PASSWORD) or a
PVEUserAdmin service account — a VM-scoped API token cannot create users or
modify ACLs.

Examples:
  # Create a user + token, grant PVEVMAdmin at /
  pve access create --user automation --token prod

  # Grant /nodes audit too (enables pve node disks * reporting)
  pve access create --user snapshot --token t1 --nodes-audit

  # Add a second token to an existing user
  pve access add-token --user automation --token ci

  # Revoke one token (user kept)
  pve access revoke-token --user automation --token ci

  # Remove a user and all its tokens
  pve access remove --user automation

  # List a user's tokens
  pve access list --user automation

  # Grant a role to an EXISTING user without reissuing its token
  pve access grant --user snapshot --path / --grant-role PVEAuditor

  # Show the ACL (all entries, or filtered to one user and its tokens)
  pve access acl-list --user snapshot`,
}

// resolveUserID splits an optional embedded @realm out of --user, honoring the
// precedence: explicit --realm > embedded @realm > default. Returns the full
// user@realm id.
func resolveUserID(cmd *cobra.Command) string {
	name := accessUser
	realm := accessRealm
	realmExplicit := false
	if cmd != nil {
		realmExplicit = cmd.Flags().Changed("realm")
	}
	if strings.Contains(name, "@") {
		at := strings.LastIndex(name, "@")
		embedded := name[at+1:]
		name = name[:at]
		if !realmExplicit {
			realm = embedded
		}
	}
	return name + "@" + realm
}

// tokenFullID returns user@realm!tokenid.
func tokenFullID(cmd *cobra.Command) string {
	return resolveUserID(cmd) + "!" + accessToken
}

func hostForDisplay() string {
	if h := os.Getenv("PVE_HOST"); h != "" {
		return h
	}
	return "<PVE_HOST>"
}

// ----- create --------------------------------------------------------------

var accessCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a user (if missing) and an API token with role assignment",
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)
		tokenID := tokenFullID(cmd)

		fmt.Printf("%sCreating API token for %s ...%s\n", "\033[0;34m", userid, "\033[0m")

		// User: create only if missing.
		if accessDryRun {
			fmt.Printf("  [DRY-RUN] would ensure user %s exists\n", userid)
		} else {
			created, err := accessOps.CreateUser(userid, "PVE CLI admin user")
			if err != nil {
				return err
			}
			if created {
				fmt.Printf("  ✓ Created user %s\n", userid)
			} else {
				fmt.Printf("  • User %s already exists, skipping creation\n", userid)
			}
		}
		return createAndAssignToken(userid, tokenID)
	},
}

// createAndAssignToken is shared by `create` and `add-token`: it generates the
// token, assigns roles, and prints the result block. It assumes the user
// already exists (create ensures that; add-token verifies it).
func createAndAssignToken(userid, tokenID string) error {
	var tokenValue string
	if accessDryRun {
		fmt.Printf("  [DRY-RUN] would add token %s (privsep 0)\n", accessToken)
		fmt.Printf("  [DRY-RUN] would assign %s to token at /\n", accessRole)
		fmt.Printf("  [DRY-RUN] token value would be shown here\n")
		return nil
	}

	value, err := accessOps.CreateToken(userid, accessToken, false) // privsep 0
	if err != nil {
		return fmt.Errorf("failed to create token: %w\n  If the token already exists, revoke it first:\n  pve access revoke-token --user %s --token %s",
			err, accessUser, accessToken)
	}
	tokenValue = value
	fmt.Printf("  ✓ Created token %s\n", accessToken)

	// Assign role to token (and user) at /.
	if err := accessOps.UpdateACL("/", tokenID, "", accessRole, true); err != nil {
		return err
	}
	fmt.Printf("  ✓ Assigned %s to token at /\n", accessRole)
	if err := accessOps.UpdateACL("/", "", userid, accessRole, true); err != nil {
		return err
	}
	fmt.Printf("  ✓ Assigned %s to user at /\n", accessRole)

	// Opt-in: PVEAuditor on /nodes for ZFS/disk/SMART reporting.
	if accessNodesAudit {
		if err := accessOps.UpdateACL("/nodes", tokenID, "", "PVEAuditor", true); err != nil {
			return err
		}
		fmt.Printf("  ✓ Assigned PVEAuditor to token at /nodes (disk reporting)\n")
		if err := accessOps.UpdateACL("/nodes", "", userid, "PVEAuditor", true); err != nil {
			return err
		}
		fmt.Printf("  ✓ Assigned PVEAuditor to user at /nodes (disk reporting)\n")
	}

	access.DisplayTokenResult(&access.TokenResult{
		Host:       hostForDisplay(),
		Userid:     userid,
		TokenName:  accessToken,
		TokenID:    tokenID,
		TokenValue: tokenValue,
		Role:       accessRole,
	})
	return nil
}

// ----- add-token -----------------------------------------------------------

var accessAddTokenCmd = &cobra.Command{
	Use:   "add-token",
	Short: "Add a token to an EXISTING user (user is not created)",
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)
		tokenID := tokenFullID(cmd)

		if !accessDryRun {
			exists, err := accessOps.UserExists(userid)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("user %s does not exist; use 'pve access create' to provision it first", userid)
			}
		}
		return createAndAssignToken(userid, tokenID)
	},
}

// ----- revoke-token --------------------------------------------------------

var accessRevokeCmd = &cobra.Command{
	Use:   "revoke-token",
	Short: "Revoke one token (user and other tokens are preserved)",
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)
		fmt.Printf("\033[0;31mRevoking token %s for %s ...\033[0m\n", accessToken, userid)
		if accessDryRun {
			fmt.Printf("  [DRY-RUN] would delete token %s\n", tokenFullID(cmd))
			return nil
		}
		if err := accessOps.DeleteToken(userid, accessToken); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

// ----- remove --------------------------------------------------------------

var accessRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Delete a user and all its tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)
		fmt.Printf("\033[0;31mRemoving user %s (and all its tokens) ...\033[0m\n", userid)
		if accessDryRun {
			fmt.Printf("  [DRY-RUN] would delete user %s\n", userid)
			return nil
		}
		if err := accessOps.DeleteUser(userid); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

// ----- list ----------------------------------------------------------------

var accessListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a user's API tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)
		tokens, err := accessOps.ListTokens(userid)
		if err != nil {
			return err
		}
		access.DisplayTokenList(userid, tokens)
		return nil
	},
}

// grantToTokenIsNoop reports whether granting a role directly to the named
// token would have no effect.
//
// A token created with privilege separation disabled (privsep 0) inherits the
// owning user's privileges, and Proxmox ignores the token's own ACL entries.
// The grant is accepted, the ACL row appears in 'pveum acl list', and the
// privilege never materialises — an operator then reasonably concludes the
// grant "did not take" and re-runs it. Verified on a live cluster 2026-08-25.
//
// Returns false when the token is unknown: absence of evidence is not evidence
// that the grant is pointless, and a spurious warning on a working command is
// its own failure.
func grantToTokenIsNoop(tokens []*access.APIToken, tokenName string) bool {
	for _, t := range tokens {
		if t.Tokenid == tokenName {
			return !t.Privsep
		}
	}
	return false
}

// ----- grant ---------------------------------------------------------------

var accessGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant a role to an existing user or token on a path",
	Long: `Assign a role on an ACL path without recreating the user or token.

This is the REST equivalent of 'pveum aclmod <path> -user <u> -role <r>'. Use it
to add privileges to a credential that already exists — previously the only way
to change a token's role was to revoke and reissue it, which changes the secret.

The grant targets the USER by default. That is deliberate: a token created with
privilege separation disabled (privsep 0, which 'pve access create' uses)
inherits the user's privileges, and its own token-level ACL is ignored — so a
token-only grant silently has no effect. Pass --to-token to override, and the
command warns when that combination would be a no-op.

Note that report endpoints do not all check the same path: node status, physical
disks and SMART check Sys.Audit on /nodes, while ZFS pools, cluster health and
HA status check it on /. Run 'pve node capability' to see which are reachable.

Examples:
  # Enable ZFS/cluster-health/HA reporting (these check '/')
  pve access grant --user snapshot --path / --grant-role PVEAuditor

  # Enable node status / disks / SMART (these check '/nodes')
  pve access grant --user snapshot --path /nodes --grant-role PVEAuditor

  # Grant to the token itself (only meaningful when privsep is enabled)
  pve access grant --user automation --token ci --path / --grant-role PVEAuditor --to-token`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userid := resolveUserID(cmd)

		if accessGrantRole == "" {
			return fmt.Errorf("--grant-role is required (e.g. PVEAuditor, PVEVMAdmin)")
		}
		if accessToToken && accessToken == "" {
			return fmt.Errorf("--to-token requires --token")
		}

		subject, users, tokens := userid, userid, ""
		if accessToToken {
			subject = tokenFullID(cmd)
			users, tokens = "", subject
		}

		fmt.Printf("\033[0;34mGranting %s on %s to %s ...\033[0m\n", accessGrantRole, accessPath, subject)

		// Warn before acting: granting to a privsep-0 token is a silent no-op,
		// which is exactly the failure this command exists to help avoid.
		// Reading the token list needs User.Modify, which the credential being
		// repaired often lacks — so a failed read must not block the grant.
		if accessToToken && !accessDryRun {
			if toks, err := accessOps.ListTokens(userid); err == nil {
				if grantToTokenIsNoop(toks, accessToken) {
					fmt.Printf("  \033[1;33m⚠️  Token %s has privsep disabled, so it inherits the user's\n", accessToken)
					fmt.Printf("     privileges and this token-level grant will have NO effect.\n")
					fmt.Printf("     Grant to the user instead (drop --to-token).\033[0m\n")
				}
			}
		}

		if accessDryRun {
			fmt.Printf("  [DRY-RUN] would PUT /access/acl path=%s roles=%s %s=%s propagate=1\n",
				accessPath, accessGrantRole,
				map[bool]string{true: "tokens", false: "users"}[accessToToken], subject)
			return nil
		}

		if err := accessOps.UpdateACL(accessPath, tokens, users, accessGrantRole, true); err != nil {
			return err
		}
		fmt.Printf("  ✓ Granted %s on %s\n", accessGrantRole, accessPath)

		// Show the resulting state rather than asserting success from the call.
		entries, err := accessOps.ListACL()
		if err != nil {
			fmt.Printf("  (granted, but could not read back the ACL: %v)\n", err)
			return nil
		}
		access.DisplayACL(entries, userid)
		fmt.Println("\nVerify what the credential can now reach:  pve node capability")
		return nil
	},
}

// ----- acl-list ------------------------------------------------------------

var accessACLListCmd = &cobra.Command{
	Use:   "acl-list",
	Short: "List access control entries (optionally filtered by user)",
	Long: `Show the cluster ACL (GET /access/acl).

With --user, only entries for that user and its tokens are shown. Without it,
the whole ACL is listed. Read-only.

Examples:
  pve access acl-list
  pve access acl-list --user snapshot`,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := accessOps.ListACL()
		if err != nil {
			return err
		}
		filter := ""
		if accessUser != "" {
			filter = resolveUserID(cmd)
		}
		access.DisplayACL(entries, filter)
		return nil
	},
}

func initAccessCommands() {
	rootCmd.AddCommand(accessCmd)
	accessCmd.AddCommand(accessCreateCmd)
	accessCmd.AddCommand(accessAddTokenCmd)
	accessCmd.AddCommand(accessRevokeCmd)
	accessCmd.AddCommand(accessRemoveCmd)
	accessCmd.AddCommand(accessListCmd)
	accessCmd.AddCommand(accessGrantCmd)
	accessCmd.AddCommand(accessACLListCmd)

	// Shared flags on the parent so every subcommand inherits them.
	accessCmd.PersistentFlags().StringVar(&accessUser, "user", "", "Username, or user@realm (required)")
	accessCmd.PersistentFlags().StringVar(&accessToken, "token", "", "Token name")
	accessCmd.PersistentFlags().StringVar(&accessRealm, "realm", "pve", "Auth realm: pam, pve, ldap (default pve)")
	accessCmd.PersistentFlags().StringVar(&accessRole, "role", "PVEVMAdmin", "Role to assign at /")
	accessCmd.PersistentFlags().BoolVar(&accessNodesAudit, "nodes-audit", false,
		"Also grant PVEAuditor on /nodes (adds Sys.Audit for ZFS/disk/SMART reporting)")
	accessCmd.PersistentFlags().BoolVar(&accessDryRun, "dry-run", false, "Show what would happen without executing")

	// Mark required flags per subcommand.
	_ = accessCreateCmd.MarkPersistentFlagRequired("user")
	_ = accessCreateCmd.MarkPersistentFlagRequired("token")
	_ = accessAddTokenCmd.MarkPersistentFlagRequired("user")
	_ = accessAddTokenCmd.MarkPersistentFlagRequired("token")
	_ = accessRevokeCmd.MarkPersistentFlagRequired("user")
	_ = accessRevokeCmd.MarkPersistentFlagRequired("token")
	_ = accessRemoveCmd.MarkPersistentFlagRequired("user")
	_ = accessListCmd.MarkPersistentFlagRequired("user")

	// grant: --path defaults to / (the broadest common case, and the one the
	// ZFS/health/HA endpoints check). --grant-role has no default on purpose.
	accessGrantCmd.Flags().StringVar(&accessPath, "path", "/", "ACL path to grant on (e.g. / or /nodes)")
	accessGrantCmd.Flags().StringVar(&accessGrantRole, "grant-role", "", "Role to grant (required, e.g. PVEAuditor)")
	accessGrantCmd.Flags().BoolVar(&accessToToken, "to-token", false,
		"Grant to the token instead of the user (no-op when the token has privsep disabled)")
	_ = accessGrantCmd.MarkPersistentFlagRequired("user")
}
