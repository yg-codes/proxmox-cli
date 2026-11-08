package api

import (
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// The zero-value ClientConfig must verify certificates: library callers that
// never mention TLS get a secure client. Skipping verification is an explicit
// opt-in, and it must be announced with a warning.
func TestClientTLSVerification(t *testing.T) {
	cases := []struct {
		name     string
		cfg      ClientConfig
		wantSkip bool
		wantWarn bool
	}{
		{name: "zero value verifies", cfg: ClientConfig{}, wantSkip: false, wantWarn: false},
		{name: "explicit insecure skips and warns", cfg: ClientConfig{InsecureSkipVerify: true}, wantSkip: true, wantWarn: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := test.NewNullLogger()
			cfg := tc.cfg
			cfg.Host = "pve.example.com"
			cfg.Username = "root@pam"
			cfg.TokenName = "t"
			cfg.TokenValue = "v"
			cfg.Timeout = time.Second
			cfg.Logger = logger

			c := NewClient(&cfg)

			transport, ok := c.httpClient.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport is %T, want *http.Transport", c.httpClient.Transport)
			}
			if transport.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if got := transport.TLSClientConfig.InsecureSkipVerify; got != tc.wantSkip {
				t.Errorf("transport InsecureSkipVerify=%v, want %v", got, tc.wantSkip)
			}

			warned := false
			for _, e := range hook.AllEntries() {
				if e.Level == logrus.WarnLevel {
					warned = true
				}
			}
			if warned != tc.wantWarn {
				t.Errorf("warning emitted=%v, want %v (entries: %d)", warned, tc.wantWarn, len(hook.AllEntries()))
			}
		})
	}
}

// End-to-end against a real self-signed TLS server: the zero-value client
// must refuse the handshake, and trusting the server's CA via CACert must
// succeed. Guards against a regression where verification silently stops
// happening (the transport-field test alone cannot see that).
func TestClientZeroValueRejectsUntrustedCert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"version":"9.0"}}`))
	}))
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatalf("split test server URL: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	logger, _ := test.NewNullLogger()

	newClient := func(ca *x509.CertPool) *Client {
		return NewClient(&ClientConfig{
			Host: host, Port: port,
			Username: "root@pam", TokenName: "t", TokenValue: "v",
			CACert: ca, Timeout: 5 * time.Second, Logger: logger,
		})
	}

	_, err = newClient(nil).Get("/version", nil)
	if err == nil {
		t.Fatal("zero-value client accepted an untrusted self-signed certificate")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("expected a certificate verification error, got: %v", err)
	}

	trusted := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	if _, err := newClient(trusted).Get("/version", nil); err != nil {
		t.Errorf("client trusting the server CA failed: %v", err)
	}
}
