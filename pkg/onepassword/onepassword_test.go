package onepassword

import (
	"errors"
	"strings"
	"testing"
)

func TestIsRef(t *testing.T) {
	cases := map[string]bool{
		"op://SRE/proxmox/token_value": true,
		"op://Vault/Item/field":        true,
		"plain-token-value":            false,
		"":                             false,
		"https://example.com":          false,
	}
	for in, want := range cases {
		if got := IsRef(in); got != want {
			t.Errorf("IsRef(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveValuePassthrough(t *testing.T) {
	// Non-references must pass through unchanged without invoking the op CLI.
	const plain = "literal-secret"
	got, err := ResolveValue(plain)
	if err != nil {
		t.Fatalf("ResolveValue(%q) unexpected error: %v", plain, err)
	}
	if got != plain {
		t.Errorf("ResolveValue(%q) = %q, want unchanged", plain, got)
	}
}

// TestResolveCLINotFound confirms the not-found sentinel is returned when
// neither op.exe nor op is resolvable on PATH. We force the condition by
// installing a resolver that reports it directly — this also documents the
// contract that opBinary's failure surfaces as ErrCLINotFound.
func TestResolveCLINotFound(t *testing.T) {
	restore := WithResolver(func(string) (string, error) {
		return "", ErrCLINotFound
	})
	defer restore()

	if _, err := Resolve("op://SRE/item/field"); !errors.Is(err, ErrCLINotFound) {
		t.Fatalf("Resolve with missing CLI = %v, want ErrCLINotFound", err)
	}
}

// TestResolveViaFakeResolver verifies Resolve delegates to the installed
// resolver and surfaces both its result and its error.
func TestResolveViaFakeResolver(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		restore := WithResolver(func(ref string) (string, error) {
			if !IsRef(ref) {
				return "", errors.New("resolver called with non-ref")
			}
			return "decrypted:" + ref, nil
		})
		defer restore()

		got, err := Resolve("op://SRE/item/field")
		if err != nil {
			t.Fatalf("Resolve unexpected error: %v", err)
		}
		if want := "decrypted:op://SRE/item/field"; got != want {
			t.Errorf("Resolve = %q, want %q", got, want)
		}
	})

	t.Run("op error surfaced", func(t *testing.T) {
		const want = "not signed in"
		restore := WithResolver(func(string) (string, error) {
			return "", errors.New(want)
		})
		defer restore()

		_, err := Resolve("op://SRE/item/field")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Resolve error = %v, want substring %q", err, want)
		}
	})
}

// TestWithResolverRestores confirms the test hook swaps in a resolver and
// restores afterward, so tests do not leak state into each other. It does
// NOT invoke the production resolver (which shells out to op) — that would
// make the test depend on a 1Password session and cross WSL/Windows process
// boundaries. Instead it verifies swap/restore behaviorally with two fakes.
func TestWithResolverRestores(t *testing.T) {
	const fakeA = "A"
	const fakeB = "B"

	restoreA := WithResolver(func(string) (string, error) { return fakeA, nil })
	if got, _ := Resolve("op://x/y/z"); got != fakeA {
		t.Fatalf("first override: Resolve = %q, want %q", got, fakeA)
	}

	restoreB := WithResolver(func(string) (string, error) { return fakeB, nil })
	if got, _ := Resolve("op://x/y/z"); got != fakeB {
		t.Fatalf("nested override: Resolve = %q, want %q", got, fakeB)
	}
	restoreB()
	// Restoring B must reveal A again (LIFO), proving restore reverts one level.
	if got, _ := Resolve("op://x/y/z"); got != fakeA {
		t.Fatalf("after restoreB: Resolve = %q, want %q (A should be back)", got, fakeA)
	}

	restoreA()
	// Restoring A must remove it: a fresh override produces its own output.
	restoreC := WithResolver(func(string) (string, error) { return "C", nil })
	defer restoreC()
	if got, _ := Resolve("op://x/y/z"); got != "C" {
		t.Fatalf("after restoreA + new override: Resolve = %q, want %q", got, "C")
	}
}
