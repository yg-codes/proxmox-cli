package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yg-codes/proxmox-cli/pkg/onepassword"
)

// allResolvedConfig returns a Config whose Proxmox credentials are non-empty
// plain values, satisfying Validate's required-field checks.
func allPlainConfig() *Config {
	c := &Config{}
	c.Proxmox.Host = "pve.example.com"
	c.Proxmox.Username = "root@pam"
	c.Proxmox.TokenName = "tok"
	c.Proxmox.TokenValue = "secret"
	c.Operations.SnapshotNameMaxLength = 40
	c.Operations.MaxConcurrentSnapshots = 1
	c.Operations.MaxConcurrentVMOps = 1
	return c
}

// TestResolveSecretsSkipsPlainFields verifies that when no PVE_* value is an
// op:// reference, ResolveSecrets leaves everything unchanged and never
// invokes the resolver (no op CLI call).
func TestResolveSecretsSkipsPlainFields(t *testing.T) {
	calls := 0
	restore := onepassword.WithResolver(func(string) (string, error) {
		calls++
		return "should-not-be-used", nil
	})
	defer restore()

	c := allPlainConfig()
	before := *c
	if err := c.ResolveSecrets(); err != nil {
		t.Fatalf("ResolveSecrets with plain values: unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("resolver invoked %d times, want 0 (no op:// refs present)", calls)
	}
	if c.Proxmox != before.Proxmox {
		t.Fatalf("ResolveSecrets mutated plain config:\n got  %+v\n want %+v", c.Proxmox, before.Proxmox)
	}
}

// TestResolveSecretsResolvesAllRefs verifies each ref field is replaced with
// the resolver's output.
func TestResolveSecretsResolvesAllRefs(t *testing.T) {
	restore := onepassword.WithResolver(func(ref string) (string, error) {
		return "plain:" + ref, nil
	})
	defer restore()

	c := &Config{}
	c.Proxmox.Host = "op://vault/item/host"
	c.Proxmox.Username = "op://vault/item/user"
	c.Proxmox.Password = "op://vault/item/password"
	c.Proxmox.TokenName = "op://vault/item/token_name"
	c.Proxmox.TokenValue = "op://vault/item/token_value"

	if err := c.ResolveSecrets(); err != nil {
		t.Fatalf("ResolveSecrets: unexpected error: %v", err)
	}
	want := map[string]string{
		"Host":       "plain:op://vault/item/host",
		"Username":   "plain:op://vault/item/user",
		"Password":   "plain:op://vault/item/password",
		"TokenName":  "plain:op://vault/item/token_name",
		"TokenValue": "plain:op://vault/item/token_value",
	}
	got := map[string]string{
		"Host": c.Proxmox.Host, "Username": c.Proxmox.Username, "Password": c.Proxmox.Password,
		"TokenName": c.Proxmox.TokenName, "TokenValue": c.Proxmox.TokenValue,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
}

// TestResolveSecretsResolvesMixed verifies plain and ref values coexist: only
// the ref is rewritten, the plain value is untouched.
func TestResolveSecretsMixed(t *testing.T) {
	restore := onepassword.WithResolver(func(string) (string, error) {
		return "resolved-token", nil
	})
	defer restore()

	c := &Config{}
	c.Proxmox.Host = "pve.example.com"                  // plain
	c.Proxmox.Username = "root@pam"                     // plain
	c.Proxmox.TokenName = "tok"                         // plain
	c.Proxmox.TokenValue = "op://vault/item/credential" // ref

	if err := c.ResolveSecrets(); err != nil {
		t.Fatalf("ResolveSecrets: unexpected error: %v", err)
	}
	if c.Proxmox.Host != "pve.example.com" {
		t.Errorf("plain Host mutated to %q", c.Proxmox.Host)
	}
	if c.Proxmox.TokenValue != "resolved-token" {
		t.Errorf("ref TokenValue = %q, want %q", c.Proxmox.TokenValue, "resolved-token")
	}
}

// TestResolveSecretsNamesFailingField verifies the error message identifies
// which PVE_* field failed resolution.
func TestResolveSecretsNamesFailingField(t *testing.T) {
	restore := onepassword.WithResolver(func(string) (string, error) {
		return "", errors.New("not signed in")
	})
	defer restore()

	c := &Config{}
	c.Proxmox.Host = "pve.example.com"
	c.Proxmox.Username = "root@pam"
	c.Proxmox.TokenName = "tok"
	c.Proxmox.TokenValue = "op://vault/item/credential" // first ref → fails

	err := c.ResolveSecrets()
	if err == nil {
		t.Fatal("ResolveSecrets with failing resolver: want error, got nil")
	}
	if !strings.Contains(err.Error(), "PVE_TOKEN_VALUE") {
		t.Errorf("error %q does not name the failing field PVE_TOKEN_VALUE", err)
	}
	if !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("error %q does not surface the underlying op error", err)
	}
}

// --- Task 4: PVE_VERIFY_SSL parsing ----------------------------------------

func TestLoadConfigVerifySSL(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    bool
		wantErr bool
	}{
		{name: "unset defaults to true (secure default)", env: "", want: true},
		{name: "true enables verification", env: "true", want: true},
		{name: "TRUE enables verification", env: "TRUE", want: true},
		{name: "false disables verification", env: "false", want: false},
		{name: "0 disables verification", env: "0", want: false},
		{name: "invalid value fails configuration loading", env: "yes-please", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Always set, even to "": LoadConfig skips empty values, and
			// inheriting the ambient environment would make this test fail on
			// a developer shell that exports the variable.
			t.Setenv("PVE_VERIFY_SSL", tc.env)
			cfg, err := LoadConfig("")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("PVE_VERIFY_SSL=%q: err=nil, want actionable error", tc.env)
				}
				if !strings.Contains(err.Error(), "PVE_VERIFY_SSL") {
					t.Errorf("error %q does not name PVE_VERIFY_SSL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("PVE_VERIFY_SSL=%q: unexpected error: %v", tc.env, err)
			}
			if cfg.Proxmox.VerifySSL != tc.want {
				t.Errorf("PVE_VERIFY_SSL=%q: VerifySSL=%v, want %v", tc.env, cfg.Proxmox.VerifySSL, tc.want)
			}
		})
	}
}

// --- Task 5: PVE_TASK_TIMEOUT parsing and validation -----------------------

func TestLoadConfigTaskTimeout(t *testing.T) {
	t.Run("unset keeps the five-minute default", func(t *testing.T) {
		// Set explicitly to empty: unset would inherit the ambient shell.
		t.Setenv("PVE_TASK_TIMEOUT", "")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Operations.TaskTimeout != 5*time.Minute {
			t.Errorf("TaskTimeout=%v, want 5m default", cfg.Operations.TaskTimeout)
		}
	})

	t.Run("supported duration is honored", func(t *testing.T) {
		t.Setenv("PVE_TASK_TIMEOUT", "90s")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Operations.TaskTimeout != 90*time.Second {
			t.Errorf("TaskTimeout=%v, want 90s", cfg.Operations.TaskTimeout)
		}
	})

	t.Run("invalid value fails configuration loading", func(t *testing.T) {
		t.Setenv("PVE_TASK_TIMEOUT", "banana")
		_, err := LoadConfig("")
		if err == nil {
			t.Fatal("PVE_TASK_TIMEOUT=banana: err=nil, want actionable error")
		}
		if !strings.Contains(err.Error(), "PVE_TASK_TIMEOUT") {
			t.Errorf("error %q does not name PVE_TASK_TIMEOUT", err)
		}
	})

	t.Run("zero and negative values fail validation", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			d    time.Duration
		}{
			{"zero", 0},
			{"negative", -time.Minute},
		} {
			c := allPlainConfig()
			c.Proxmox.Port = 8006
			c.Operations.TaskTimeout = tc.d
			if err := c.Validate(); err == nil {
				t.Errorf("%s task timeout: Validate passed, want rejection", tc.name)
			} else if !strings.Contains(err.Error(), "task_timeout") {
				t.Errorf("%s: error %q does not name task_timeout", tc.name, err)
			}
		}
	})
}

// --- PVE_PORT parsing (issue #27 step 0) -------------------------------------

func TestLoadConfigPort(t *testing.T) {
	t.Run("unset keeps the default 8006", func(t *testing.T) {
		t.Setenv("PVE_PORT", "")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Proxmox.Port != 8006 {
			t.Errorf("Port=%d, want 8006 default", cfg.Proxmox.Port)
		}
	})

	t.Run("port override is honored", func(t *testing.T) {
		t.Setenv("PVE_PORT", "8443")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Proxmox.Port != 8443 {
			t.Errorf("Port=%d, want 8443", cfg.Proxmox.Port)
		}
	})

	t.Run("non-integer fails configuration loading", func(t *testing.T) {
		t.Setenv("PVE_PORT", "https")
		_, err := LoadConfig("")
		if err == nil {
			t.Fatal("PVE_PORT=https: err=nil, want actionable error")
		}
		if !strings.Contains(err.Error(), "PVE_PORT") {
			t.Errorf("error %q does not name PVE_PORT", err)
		}
	})

	t.Run("out-of-range values pass loading and fail Validate", func(t *testing.T) {
		t.Setenv("PVE_PORT", "0")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("loading should parse, not range-check: %v", err)
		}
		c := allPlainConfig()
		c.Proxmox.Port = cfg.Proxmox.Port
		c.Operations.TaskTimeout = 5 * time.Minute
		if err := c.Validate(); err == nil {
			t.Error("Port=0 passed Validate, want range rejection")
		}
	})
}

// throwawayCAPEM is a self-signed certificate (CN=pve.example.com) generated
// for tests only; it validates PEM parsing, not trust.
const throwawayCAPEM = `-----BEGIN CERTIFICATE-----
MIIDFTCCAf2gAwIBAgIUbCAYkcFJsaS4+tkxrefy7WXgDNMwDQYJKoZIhvcNAQEL
BQAwGjEYMBYGA1UEAwwPcHZlLmV4YW1wbGUuY29tMB4XDTI2MTAyODA3NTgwNloX
DTI2MTAyOTA3NTgwNlowGjEYMBYGA1UEAwwPcHZlLmV4YW1wbGUuY29tMIIBIjAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyp+/jspvYb/GtlRWhY0tWMKlceJJ
jUVgnE3/Z/+vJPdTD9njZnhEFma2+k0hJr3pgLyxvmoI+w88aUD7mfIlcW9jwUGl
dhzV1fTkc5FQHVVIluvVtAdwfz/7EfBE+a/bn5KGPBl2OHs+nSeaKPPOD6lWWBea
CsUS+KON8fcP9wHnEATimQudgsQlXYodi9W3RnsaZmhxZvREFgJn5iCMuzAUHLmK
YmtCa+bgEoTXvXUQNIOj0VXX/hVlKUWmzYrCyVjUehvg337/SN3hNfZ7VOM4jWjo
DA771JllBU6hzt/LBFKwx2bjMcEyizGk8+xPsDm/XNZmYaXHbQVDa4tKvwIDAQAB
o1MwUTAdBgNVHQ4EFgQUhI03zMlsYWJVrtQm1CvPO3PJ4BowHwYDVR0jBBgwFoAU
hI03zMlsYWJVrtQm1CvPO3PJ4BowDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B
AQsFAAOCAQEAQ910sJRYeNGqTzhuEe7olKGaAHWOYH4MTiEzJEdpFxyulN9BTdcv
yomjikBKRFovXdUKO3OMRyrH+xRFLYyBWVNzLrkNVxGBacdjlr9bIDWh893U/9b8
PEwO3hXcLG0AgxTrCBPc0Fsv9iVKoKcmySAicj1QDzpiNe+DEg7RTYWQ0lF7DCvM
iKN8ZmbL3iQJEIwcyEJzUAiDEy8GWV9ww1TvThYkAjvQiOqu2WuKpaGbb9wqL+nx
vuiOuKgGMEs9Bzg/E5Xefqro+8uhBOzF5mIS9HD/7ZRLc6FpnNI1CPKqSZFz60UM
aZWPy5/OfFhc4on6jdWkhdPzL2ykPtnD5w==
-----END CERTIFICATE-----
`

func TestLoadConfigCACert(t *testing.T) {
	t.Run("unset leaves CA empty", func(t *testing.T) {
		t.Setenv("PVE_CA_CERT", "")
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("unset PVE_CA_CERT: unexpected error: %v", err)
		}
		if cfg.Proxmox.CACert != "" || cfg.Proxmox.CACertPool != nil {
			t.Errorf("unset PVE_CA_CERT: CACert=%q pool=%v, want empty/nil", cfg.Proxmox.CACert, cfg.Proxmox.CACertPool)
		}
	})

	t.Run("valid PEM loads pool", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(path, []byte(throwawayCAPEM), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		t.Setenv("PVE_CA_CERT", path)
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("valid PVE_CA_CERT: unexpected error: %v", err)
		}
		if cfg.Proxmox.CACert != path {
			t.Errorf("CACert=%q, want %q", cfg.Proxmox.CACert, path)
		}
		if cfg.Proxmox.CACertPool == nil {
			t.Error("CACertPool=nil, want parsed pool")
		}
	})

	t.Run("missing file fails with actionable error", func(t *testing.T) {
		t.Setenv("PVE_CA_CERT", "/nonexistent/ca.pem")
		_, err := LoadConfig("")
		if err == nil {
			t.Fatal("missing CA file: err=nil, want error")
		}
		if !strings.Contains(err.Error(), "PVE_CA_CERT") {
			t.Errorf("error %q does not name PVE_CA_CERT", err)
		}
	})

	t.Run("non-PEM file fails", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(path, []byte("this is not a certificate"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		t.Setenv("PVE_CA_CERT", path)
		_, err := LoadConfig("")
		if err == nil {
			t.Fatal("non-PEM CA file: err=nil, want error")
		}
		if !strings.Contains(err.Error(), "no PEM certificates") {
			t.Errorf("error %q does not explain the PEM problem", err)
		}
	})
}
