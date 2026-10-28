package main

import (
	"strings"
	"testing"
)

// Batch mode must accept either a complete token pair OR PVE_PASSWORD — the
// same token-or-password contract as normal authentication. Host and user
// remain mandatory in every case.
func TestBatchAuthenticationRequirements(t *testing.T) {
	setCreds := func(t *testing.T, host, user, tokenName, tokenValue, password string) {
		t.Helper()
		set := func(k, v string) {
			if v == "" {
				t.Setenv(k, "")
			} else {
				t.Setenv(k, v)
			}
		}
		set("PVE_HOST", host)
		set("PVE_USER", user)
		set("PVE_TOKEN_NAME", tokenName)
		set("PVE_TOKEN_VALUE", tokenValue)
		set("PVE_PASSWORD", password)
	}

	cases := []struct {
		name         string
		host, user   string
		tokenName    string
		tokenValue   string
		password     string
		wantErr      bool
		wantMentions []string
	}{
		{
			name: "complete token pair passes", host: "pve1", user: "root@pam",
			tokenName: "tok", tokenValue: "secret",
		},
		{
			name: "password only passes", host: "pve1", user: "root@pam",
			password: "hunter2",
		},
		{
			name: "incomplete token with password passes", host: "pve1", user: "root@pam",
			tokenName: "tok", password: "hunter2",
		},
		{
			name: "incomplete token without password fails", host: "pve1", user: "root@pam",
			tokenName:    "tok",
			wantErr:      true,
			wantMentions: []string{"PVE_TOKEN_NAME", "PVE_PASSWORD"},
		},
		{
			name: "no credentials fails", host: "pve1", user: "root@pam",
			wantErr:      true,
			wantMentions: []string{"PVE_TOKEN_NAME", "PVE_PASSWORD"},
		},
		{
			name: "missing host fails", host: "", user: "root@pam",
			tokenName: "tok", tokenValue: "secret",
			wantErr:      true,
			wantMentions: []string{"PVE_HOST"},
		},
		{
			name: "missing user fails", host: "pve1", user: "",
			tokenName: "tok", tokenValue: "secret",
			wantErr:      true,
			wantMentions: []string{"PVE_USER"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setCreds(t, tc.host, tc.user, tc.tokenName, tc.tokenValue, tc.password)

			err := validateBatchAuth()
			if tc.wantErr {
				if err == nil {
					t.Fatal("err=nil, want batch auth error")
				}
				for _, mention := range tc.wantMentions {
					if !strings.Contains(err.Error(), mention) {
						t.Errorf("error %q does not mention %s", err, mention)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("err=%v, want nil", err)
			}
		})
	}
}
