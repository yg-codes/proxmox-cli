package main

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/config"
)

// buildClientConfig is the cfg→client join: every Proxmox field must reach
// the ClientConfig, or a config parsed correctly still produces a client
// configured wrong (the PVE_VERIFY_SSL wiring was exactly this kind of gap).
func TestBuildClientConfigJoins(t *testing.T) {
	cfg := &config.Config{}
	cfg.Proxmox = config.ProxmoxConfig{
		Host:       "pve.example.com",
		Port:       8443,
		Username:   "root@pam",
		Password:   "pw",
		TokenName:  "tok",
		TokenValue: "secret",
		VerifySSL:  true,
		Timeout:    11 * time.Second,
	}

	cc := buildClientConfig(cfg, logrus.New())

	if cc.Host != "pve.example.com" || cc.Port != 8443 {
		t.Errorf("endpoint join wrong: host=%q port=%d", cc.Host, cc.Port)
	}
	if cc.Username != "root@pam" || cc.Password != "pw" {
		t.Errorf("password-auth join wrong: user=%q password=%q", cc.Username, cc.Password)
	}
	if cc.TokenName != "tok" || cc.TokenValue != "secret" {
		t.Errorf("token join wrong: name=%q value=%q", cc.TokenName, cc.TokenValue)
	}
	if !cc.VerifySSL {
		t.Error("VerifySSL=true did not reach ClientConfig (verification would be skipped)")
	}
	if cc.Timeout != 11*time.Second {
		t.Errorf("Timeout=%v, want 11s", cc.Timeout)
	}
	if cc.Logger == nil {
		t.Error("Logger not wired")
	}
}
