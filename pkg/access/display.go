package access

import (
	"fmt"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
)

// TokenResult holds the data printed after a token is created, mirroring the
// bash script's print_token_result block.
type TokenResult struct {
	Host       string // PVE host the token authenticates against
	Userid     string // user@realm
	TokenName  string
	TokenID    string // user@realm!tokenid
	TokenValue string
	Role       string
}

// DisplayTokenResult prints the success banner, the secret (highlighted), and
// ready-to-use shell-export + .env blocks.
func DisplayTokenResult(r *TokenResult) {
	fmt.Printf("\n%s================================================%s\n", colorGreen, colorReset)
	fmt.Printf("%s  API Token Created Successfully%s\n", colorGreen, colorReset)
	fmt.Printf("%s================================================%s\n", colorGreen, colorReset)
	fmt.Println()
	fmt.Printf("  Host:        %s\n", r.Host)
	fmt.Printf("  User:        %s\n", r.Userid)
	fmt.Printf("  Token:       %s\n", r.TokenName)
	fmt.Printf("  Token ID:    %s\n", r.TokenID)
	fmt.Printf("  Token Value: %s%s%s\n", colorYellow, r.TokenValue, colorReset)
	fmt.Printf("  Role:        %s\n", r.Role)
	fmt.Println()
	fmt.Printf("%sSave the token value now - it cannot be retrieved later!%s\n", colorRed, colorReset)
	fmt.Println()
	fmt.Printf("%s--- Shell exports (add to ~/.bashrc) ---%s\n", colorBlue, colorReset)
	fmt.Printf("export PVE_HOST=%q\n", r.Host)
	fmt.Printf("export PVE_USER=%q\n", r.Userid)
	fmt.Printf("export PVE_TOKEN_NAME=%q\n", r.TokenName)
	fmt.Printf("export PVE_TOKEN_VALUE=%q\n", r.TokenValue)
	fmt.Println()
	fmt.Printf("%s--- .env file format ---%s\n", colorBlue, colorReset)
	fmt.Printf("PVE_HOST=%s\n", r.Host)
	fmt.Printf("PVE_USER=%s\n", r.Userid)
	fmt.Printf("PVE_TOKEN_NAME=%s\n", r.TokenName)
	fmt.Printf("PVE_TOKEN_VALUE=%s\n", r.TokenValue)
	fmt.Println()
	fmt.Printf("%s--- Quick test ---%s\n", colorBlue, colorReset)
	fmt.Println("  pve vm list")
	fmt.Println("  pve node list")
}

// DisplayACL prints ACL entries in a table. When filter is non-empty, only
// entries whose subject contains it are shown (matching both a user and its
// tokens, since a token id is "user@realm!name").
func DisplayACL(entries []*ACLEntry, filter string) {
	shown := make([]*ACLEntry, 0, len(entries))
	for _, e := range entries {
		subject := e.Users + e.Tokens + e.Groups
		if filter == "" || strings.Contains(subject, filter) {
			shown = append(shown, e)
		}
	}

	if len(shown) == 0 {
		if filter != "" {
			fmt.Printf("\nNo ACL entries found for %s\n", filter)
		} else {
			fmt.Println("\nNo ACL entries found")
		}
		return
	}

	if filter != "" {
		fmt.Printf("\nACL entries for %s:\n", filter)
	} else {
		fmt.Println("\nAccess control list:")
	}
	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("%-14s %-14s %-7s %-32s %s\n", "PATH", "ROLE", "TYPE", "SUBJECT", "PROP")
	fmt.Println(strings.Repeat("-", 78))
	for _, e := range shown {
		subjType, subject := "user", e.Users
		switch {
		case e.Tokens != "":
			subjType, subject = "token", e.Tokens
		case e.Groups != "":
			subjType, subject = "group", e.Groups
		}
		prop := "no"
		if e.Propagate {
			prop = "yes"
		}
		fmt.Printf("%-14s %-14s %-7s %-32s %s\n", e.Path, e.Roles, subjType, subject, prop)
	}
	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("Total entries: %d\n", len(shown))
}

// DisplayTokenList prints a user's tokens in a table.
func DisplayTokenList(userid string, tokens []*APIToken) {
	if len(tokens) == 0 {
		fmt.Printf("\nNo API tokens found for %s\n", userid)
		return
	}
	fmt.Printf("\nAPI tokens for %s:\n", userid)
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-20s %-10s %-10s %-25s\n", "TOKEN", "PRIVSEP", "EXPIRE", "COMMENT")
	fmt.Println(strings.Repeat("-", 70))
	for _, t := range tokens {
		privsep := "no"
		if t.Privsep {
			privsep = "yes"
		}
		expire := "never"
		if t.Expire != 0 {
			expire = fmt.Sprintf("%d", t.Expire)
		}
		comment := t.Comment
		if len(comment) > 24 {
			comment = comment[:21] + "..."
		}
		fmt.Printf("%-20s %-10s %-10s %-25s\n", t.Tokenid, privsep, expire, comment)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("Total tokens: %d\n", len(tokens))
}
