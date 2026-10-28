package config

import (
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/onepassword"
)

// Config holds application configuration
type Config struct {
	// Proxmox connection settings
	Proxmox ProxmoxConfig `mapstructure:"proxmox"`

	// Operation settings
	Operations OperationsConfig `mapstructure:"operations"`

	// Logging settings
	Logging LoggingConfig `mapstructure:"logging"`

	// CLI settings
	CLI CLIConfig `mapstructure:"cli"`
}

// ProxmoxConfig holds Proxmox API configuration
type ProxmoxConfig struct {
	Host       string         `mapstructure:"host"`
	Port       int            `mapstructure:"port"`
	Username   string         `mapstructure:"username"`
	Password   string         `mapstructure:"password"`
	TokenName  string         `mapstructure:"token_name"`
	TokenValue string         `mapstructure:"token_value"`
	VerifySSL  bool           `mapstructure:"verify_ssl"`
	CACert     string         `mapstructure:"ca_cert"` // path to a PEM CA bundle; enables verified TLS against private CAs
	CACertPool *x509.CertPool `mapstructure:"-"`       // parsed from CACert at load time
	Timeout    time.Duration  `mapstructure:"timeout"`
}

// OperationsConfig holds operation-specific configuration
type OperationsConfig struct {
	MaxConcurrentSnapshots int           `mapstructure:"max_concurrent_snapshots"`
	MaxConcurrentVMOps     int           `mapstructure:"max_concurrent_vm_ops"`
	DefaultVMState         bool          `mapstructure:"default_vm_state"`
	SnapshotNameMaxLength  int           `mapstructure:"snapshot_name_max_length"`
	TaskTimeout            time.Duration `mapstructure:"task_timeout"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// CLIConfig holds CLI-specific configuration
type CLIConfig struct {
	BatchMode    bool `mapstructure:"batch_mode"`
	AutoConfirm  bool `mapstructure:"auto_confirm"`
	ColorOutput  bool `mapstructure:"color_output"`
	ProgressBars bool `mapstructure:"progress_bars"`
}

// Default configuration values
var defaultConfig = Config{
	Proxmox: ProxmoxConfig{
		Port:      8006,
		VerifySSL: true,
		Timeout:   30 * time.Second,
	},
	Operations: OperationsConfig{
		MaxConcurrentSnapshots: 2,
		MaxConcurrentVMOps:     3,
		DefaultVMState:         false,
		SnapshotNameMaxLength:  40,
		TaskTimeout:            300 * time.Second,
	},
	Logging: LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	},
	CLI: CLIConfig{
		BatchMode:    false,
		AutoConfirm:  false,
		ColorOutput:  true,
		ProgressBars: true,
	},
}

// LoadConfig loads configuration from environment variables only
// This matches the Python implementation which does not use config files
func LoadConfig(configPath string) (*Config, error) {
	// Start with default config
	config := defaultConfig

	// Load from environment variables (matching Python implementation)
	if host := os.Getenv("PVE_HOST"); host != "" {
		config.Proxmox.Host = host
	}
	if user := os.Getenv("PVE_USER"); user != "" {
		config.Proxmox.Username = user
	}
	if password := os.Getenv("PVE_PASSWORD"); password != "" {
		config.Proxmox.Password = password
	}
	if tokenName := os.Getenv("PVE_TOKEN_NAME"); tokenName != "" {
		config.Proxmox.TokenName = tokenName
	}
	if tokenValue := os.Getenv("PVE_TOKEN_VALUE"); tokenValue != "" {
		config.Proxmox.TokenValue = tokenValue
	}
	if raw := os.Getenv("PVE_TASK_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid PVE_TASK_TIMEOUT value %q: must be a Go duration (e.g. 90s, 5m): %w", raw, err)
		}
		config.Operations.TaskTimeout = d
	}
	if raw := os.Getenv("PVE_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid PVE_PORT value %q: must be an integer port number: %w", raw, err)
		}
		config.Proxmox.Port = port
	}
	if raw := os.Getenv("PVE_VERIFY_SSL"); raw != "" {
		verify, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid PVE_VERIFY_SSL value %q: must be a boolean (true/false, 1/0): %w", raw, err)
		}
		config.Proxmox.VerifySSL = verify
	}
	if raw := os.Getenv("PVE_CA_CERT"); raw != "" {
		pool, err := LoadCACertPool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid PVE_CA_CERT %q: %w", raw, err)
		}
		config.Proxmox.CACert = raw
		config.Proxmox.CACertPool = pool
	}

	// Note: configPath parameter is ignored but kept for backward compatibility
	// Config files are not supported to match Python implementation

	return &config, nil
}

// LoadCACertPool reads a PEM-encoded CA bundle from path and returns a cert
// pool usable as TLS RootCAs. This is the supported way to verify a Proxmox
// host whose certificate is signed by a private CA, instead of disabling
// verification with PVE_VERIFY_SSL=false.
func LoadCACertPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read CA bundle: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no PEM certificates found in %q", path)
	}
	return pool, nil
}

// ResolveSecrets resolves any Proxmox credential field that is a 1Password
// secret reference (op://...) to its plaintext value via the op CLI. Fields
// that are not references are left unchanged, so the op binary is only invoked
// when at least one credential is a reference.
func (c *Config) ResolveSecrets() error {
	fields := []struct {
		name string
		ptr  *string
	}{
		{"PVE_HOST", &c.Proxmox.Host},
		{"PVE_USER", &c.Proxmox.Username},
		{"PVE_PASSWORD", &c.Proxmox.Password},
		{"PVE_TOKEN_NAME", &c.Proxmox.TokenName},
		{"PVE_TOKEN_VALUE", &c.Proxmox.TokenValue},
	}

	for _, f := range fields {
		if !onepassword.IsRef(*f.ptr) {
			continue
		}
		resolved, err := onepassword.Resolve(*f.ptr)
		if err != nil {
			return fmt.Errorf("resolving %s from 1Password: %w", f.name, err)
		}
		*f.ptr = resolved
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Proxmox configuration
	if c.Proxmox.Host == "" {
		return fmt.Errorf("proxmox host is required")
	}

	if c.Proxmox.Username == "" {
		return fmt.Errorf("proxmox username is required")
	}

	// Check authentication method
	hasToken := c.Proxmox.TokenName != "" && c.Proxmox.TokenValue != ""
	hasPassword := c.Proxmox.Password != ""

	if !hasToken && !hasPassword {
		return fmt.Errorf("either token authentication or password is required")
	}

	if c.Proxmox.Port <= 0 || c.Proxmox.Port > 65535 {
		return fmt.Errorf("proxmox port must be between 1 and 65535")
	}

	// Validate operations configuration
	if c.Operations.MaxConcurrentSnapshots <= 0 {
		return fmt.Errorf("max_concurrent_snapshots must be greater than 0")
	}

	if c.Operations.MaxConcurrentVMOps <= 0 {
		return fmt.Errorf("max_concurrent_vm_ops must be greater than 0")
	}

	if c.Operations.SnapshotNameMaxLength <= 0 {
		return fmt.Errorf("snapshot_name_max_length must be greater than 0")
	}

	if c.Operations.TaskTimeout <= 0 {
		return fmt.Errorf("task_timeout must be greater than 0 (e.g. \"5m\" or \"90s\")")
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"panic": true, "fatal": true, "error": true,
		"warn": true, "info": true, "debug": true, "trace": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}

// SetupLogger configures and returns a logger based on the config
func (c *Config) SetupLogger() *logrus.Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(c.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Set log format
	if c.Logging.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     c.CLI.ColorOutput,
		})
	}

	// Set output
	switch c.Logging.Output {
	case "stderr":
		logger.SetOutput(os.Stderr)
	case "stdout":
		logger.SetOutput(os.Stdout)
	default:
		// Try to open as file
		if file, err := os.OpenFile(c.Logging.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			logger.SetOutput(file)
		} else {
			logger.SetOutput(os.Stdout)
			logger.Warnf("Could not open log file %s, using stdout: %v", c.Logging.Output, err)
		}
	}

	return logger
}

// IsBatchMode returns true if running in batch mode
func (c *Config) IsBatchMode() bool {
	return c.CLI.BatchMode
}

// IsAutoConfirm returns true if auto-confirm is enabled
func (c *Config) IsAutoConfirm() bool {
	return c.CLI.AutoConfirm
}

// GetMaxConcurrentOperations returns max concurrent operations based on operation type
func (c *Config) GetMaxConcurrentOperations(operationType string) int {
	switch operationType {
	case "snapshot":
		return c.Operations.MaxConcurrentSnapshots
	case "vm":
		return c.Operations.MaxConcurrentVMOps
	default:
		return c.Operations.MaxConcurrentSnapshots
	}
}
