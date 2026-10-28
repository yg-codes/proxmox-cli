package main

import (
	"testing"

	"github.com/yg-codes/proxmox-cli/pkg/access"
)

// TestGrantToTokenIsNoop guards the one thing 'pve access grant --to-token'
// exists to warn about.
//
// This cannot be covered by an end-to-end run: reading the token list requires
// User.Modify, which the very credential being repaired typically lacks, so the
// warning path is unreachable with a VM-scoped token. The decision is therefore
// factored out and tested directly.
func TestGrantToTokenIsNoop(t *testing.T) {
	tokens := []*access.APIToken{
		{Userid: "cli-user@pve", Tokenid: "snapshot", Privsep: false},
		{Userid: "cli-user@pve", Tokenid: "separated", Privsep: true},
	}

	t.Run("privsep disabled means the grant is a no-op", func(t *testing.T) {
		if !grantToTokenIsNoop(tokens, "snapshot") {
			t.Fatal("expected a warning for a privsep-0 token: the grant is silently ignored by Proxmox")
		}
	})

	t.Run("privsep enabled means the grant is effective", func(t *testing.T) {
		if grantToTokenIsNoop(tokens, "separated") {
			t.Fatal("a privsep-1 token honours its own ACL; warning would be wrong")
		}
	})

	// A false warning on a working command teaches operators to ignore
	// warnings, so an unknown token must stay silent rather than guess.
	t.Run("unknown token does not warn", func(t *testing.T) {
		if grantToTokenIsNoop(tokens, "does-not-exist") {
			t.Fatal("must not warn about a token that is not in the list")
		}
	})

	t.Run("empty token list does not warn", func(t *testing.T) {
		if grantToTokenIsNoop(nil, "snapshot") {
			t.Fatal("must not warn when the token list could not be read")
		}
	})
}
