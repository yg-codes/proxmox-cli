package api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ProxmoxAPIError represents an error from the Proxmox API
type ProxmoxAPIError struct {
	Message      string
	StatusCode   int
	ResponseData map[string]interface{}
}

func (e *ProxmoxAPIError) Error() string {
	return e.Message
}

// AuthMethod represents the authentication method
type AuthMethod int

const (
	AuthPassword AuthMethod = iota
	AuthToken
)

// Client represents a Proxmox API client
type Client struct {
	host       string
	port       int
	baseURL    string
	httpClient *http.Client
	logger     *logrus.Logger

	// Authentication
	authMethod AuthMethod
	username   string
	password   string
	tokenName  string
	tokenValue string
	ticket     string
	csrfToken  string
}

// ClientConfig holds configuration for the Proxmox API client
type ClientConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	TokenName  string
	TokenValue string
	VerifySSL  bool
	CACert     *x509.CertPool // optional RootCAs for private-CA verification; nil = system roots
	Timeout    time.Duration
	Logger     *logrus.Logger
}

// NewClient creates a new Proxmox API client
func NewClient(config *ClientConfig) *Client {
	if config.Port == 0 {
		config.Port = 8006
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	client := &Client{
		host:    config.Host,
		port:    config.Port,
		baseURL: fmt.Sprintf("https://%s:%d/api2/json", config.Host, config.Port),
		logger:  config.Logger,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !config.VerifySSL,
					RootCAs:            config.CACert,
				},
			},
		},
	}

	// Set authentication method and credentials
	if config.TokenName != "" && config.TokenValue != "" {
		client.authMethod = AuthToken
		client.username = config.Username
		client.tokenName = config.TokenName
		client.tokenValue = config.TokenValue
	} else {
		client.authMethod = AuthPassword
		client.username = config.Username
		client.password = config.Password
	}

	return client
}

// Connect authenticates with the Proxmox API
func (c *Client) Connect() error {
	switch c.authMethod {
	case AuthToken:
		return c.authenticateToken()
	case AuthPassword:
		return c.authenticatePassword()
	default:
		return fmt.Errorf("unknown authentication method")
	}
}

// authenticateToken authenticates using API token
func (c *Client) authenticateToken() error {
	c.logger.Debug("Authenticating with API token")
	// Token authentication doesn't require a separate auth request
	// The token is sent with each request in the Authorization header
	return nil
}

// authenticatePassword authenticates using username/password
func (c *Client) authenticatePassword() error {
	c.logger.Debug("Authenticating with password")

	authData := url.Values{
		"username": {c.username},
		"password": {c.password},
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/access/ticket", authData)
	if err != nil {
		return &ProxmoxAPIError{
			Message: fmt.Sprintf("authentication request failed: %v", err),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &ProxmoxAPIError{
			Message:    fmt.Sprintf("authentication failed with status: %s", resp.Status),
			StatusCode: resp.StatusCode,
		}
	}

	var result struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &ProxmoxAPIError{
			Message: fmt.Sprintf("failed to parse authentication response: %v", err),
		}
	}

	if result.Data.Ticket == "" {
		return &ProxmoxAPIError{
			Message: "authentication failed: no ticket received",
		}
	}

	c.ticket = result.Data.Ticket
	c.csrfToken = result.Data.CSRFPreventionToken

	c.logger.Debug("Authentication successful")
	return nil
}

// Request makes an HTTP request to the Proxmox API
func (c *Client) Request(method, path string, data interface{}, params map[string]string) (map[string]interface{}, error) {
	fullURL := c.baseURL + "/" + strings.TrimLeft(path, "/")

	// Add query parameters
	if params != nil {
		u, err := url.Parse(fullURL)
		if err != nil {
			return nil, &ProxmoxAPIError{Message: fmt.Sprintf("invalid URL: %v", err)}
		}
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	var body io.Reader
	if data != nil {
		switch method {
		case "GET":
			// For GET requests, data should be in query params
		default:
			// For POST/PUT/DELETE, encode as form data
			if formData, ok := data.(url.Values); ok {
				body = strings.NewReader(formData.Encode())
			} else {
				jsonData, err := json.Marshal(data)
				if err != nil {
					return nil, &ProxmoxAPIError{Message: fmt.Sprintf("failed to encode request data: %v", err)}
				}
				body = bytes.NewReader(jsonData)
			}
		}
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, &ProxmoxAPIError{Message: fmt.Sprintf("failed to create request: %v", err)}
	}

	// Set authentication headers
	switch c.authMethod {
	case AuthToken:
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s!%s=%s", c.username, c.tokenName, c.tokenValue))
	case AuthPassword:
		if c.ticket != "" {
			req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", c.ticket))
			if c.csrfToken != "" {
				req.Header.Set("CSRFPreventionToken", c.csrfToken)
			}
		}
	}

	// Set content type for non-GET requests
	if method != "GET" && body != nil {
		if _, ok := data.(url.Values); ok {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	c.logger.Debugf("Making %s request to %s", method, fullURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ProxmoxAPIError{Message: fmt.Sprintf("request failed: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProxmoxAPIError{Message: fmt.Sprintf("failed to read response body: %v", err)}
	}

	if resp.StatusCode >= 400 {
		var errorResp map[string]interface{}
		_ = json.Unmarshal(respBody, &errorResp)

		return nil, &ProxmoxAPIError{
			Message:      fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(respBody)),
			StatusCode:   resp.StatusCode,
			ResponseData: errorResp,
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, &ProxmoxAPIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}

	// Extract data field if it exists
	if data, ok := result["data"]; ok {
		if dataMap, ok := data.(map[string]interface{}); ok {
			return dataMap, nil
		}
		if dataSlice, ok := data.([]interface{}); ok {
			// For array responses, wrap in a map
			return map[string]interface{}{"items": dataSlice}, nil
		}
	}

	return result, nil
}

// Get makes a GET request to the Proxmox API
func (c *Client) Get(path string, params map[string]string) (map[string]interface{}, error) {
	return c.Request("GET", path, nil, params)
}

// Post makes a POST request to the Proxmox API
func (c *Client) Post(path string, data interface{}) (map[string]interface{}, error) {
	return c.Request("POST", path, data, nil)
}

// Put makes a PUT request to the Proxmox API
func (c *Client) Put(path string, data interface{}) (map[string]interface{}, error) {
	return c.Request("PUT", path, data, nil)
}

// Delete makes a DELETE request to the Proxmox API
func (c *Client) Delete(path string) (map[string]interface{}, error) {
	return c.Request("DELETE", path, nil, nil)
}
