package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// The transport must invert VerifySSL: verification on means no insecure
// skip, and the compatibility default (false) must keep skipping — the CLI
// must remain usable against self-signed certificates out of the box.
func TestClientVerifySSLTransport(t *testing.T) {
	cases := []struct {
		name      string
		verifySSL bool
		wantSkip  bool
	}{
		{name: "verification enabled", verifySSL: true, wantSkip: false},
		{name: "verification disabled (self-signed compatibility)", verifySSL: false, wantSkip: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(&ClientConfig{
				Host:       "pve.example.com",
				Username:   "root@pam",
				TokenName:  "t",
				TokenValue: "v",
				VerifySSL:  tc.verifySSL,
				Timeout:    time.Second,
				Logger:     logrus.New(),
			})
			transport, ok := c.httpClient.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport is %T, want *http.Transport", c.httpClient.Transport)
			}
			if transport.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if got := transport.TLSClientConfig.InsecureSkipVerify; got != tc.wantSkip {
				t.Errorf("InsecureSkipVerify=%v, want %v (VerifySSL=%v)", got, tc.wantSkip, tc.verifySSL)
			}
		})
	}
}
