// Package syncclient talks to an optional NovelIDE sync server (see
// internal/syncserver) and merges a local workspace with its remote copy. It
// is only used when the user configures a server; the app is otherwise fully
// offline.
package syncclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"novelide/internal/syncproto"
)

// Client is an authenticated connection to a sync server.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client for baseURL. Token may be empty for register/login.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) url(path string) string { return c.BaseURL + path }

func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.url(path), body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.HTTP.Do(req)
}

// apiError extracts the server's error message from a non-2xx response.
func apiError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e syncproto.ErrorResponse
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s (%d)", e.Error, resp.StatusCode)
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}

func (c *Client) postJSON(path string, in, out any) error {
	b, _ := json.Marshal(in)
	resp, err := c.do("POST", path, bytes.NewReader(b), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Register creates an account and stores the returned token on the client.
func (c *Client) Register(username, password string) (syncproto.AuthResponse, error) {
	var out syncproto.AuthResponse
	err := c.postJSON("/api/register", syncproto.RegisterRequest{Username: username, Password: password}, &out)
	if err == nil {
		c.Token = out.Token
	}
	return out, err
}

// Login authenticates and stores the returned token on the client.
func (c *Client) Login(username, password string) (syncproto.AuthResponse, error) {
	var out syncproto.AuthResponse
	err := c.postJSON("/api/login", syncproto.LoginRequest{Username: username, Password: password}, &out)
	if err == nil {
		c.Token = out.Token
	}
	return out, err
}

// AuthConfig reports which sign-in methods the server offers (no auth needed).
func (c *Client) AuthConfig() (syncproto.AuthConfig, error) {
	var out syncproto.AuthConfig
	resp, err := c.do("GET", "/api/auth/config", nil, "")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, apiError(resp)
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// Me returns the authenticated account (used after SSO to learn the username).
func (c *Client) Me() (syncproto.MeResponse, error) {
	var out syncproto.MeResponse
	resp, err := c.do("GET", "/api/me", nil, "")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, apiError(resp)
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// Health checks that the server is reachable.
func (c *Client) Health() error {
	resp, err := c.do("GET", "/healthz", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return nil
}

// Workspaces lists the account's remote workspaces.
func (c *Client) Workspaces() ([]syncproto.WorkspaceMeta, error) {
	resp, err := c.do("GET", "/api/workspaces", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}
	var out syncproto.WorkspaceList
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Workspaces, nil
}

// Manifest fetches a workspace's current file set.
func (c *Client) Manifest(wsID string) (syncproto.Manifest, error) {
	var m syncproto.Manifest
	resp, err := c.do("GET", "/api/workspaces/"+wsID+"/manifest", nil, "")
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return m, apiError(resp)
	}
	return m, json.NewDecoder(resp.Body).Decode(&m)
}

// HasBlob reports whether the server already has a blob.
func (c *Client) HasBlob(wsID, hash string) (bool, error) {
	resp, err := c.do("HEAD", "/api/workspaces/"+wsID+"/blobs/"+hash, nil, "")
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, apiError(resp)
	}
}

// PutBlob uploads a blob.
func (c *Client) PutBlob(wsID, hash string, data []byte) error {
	resp, err := c.do("PUT", "/api/workspaces/"+wsID+"/blobs/"+hash, bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return nil
}

// GetBlob downloads a blob.
func (c *Client) GetBlob(wsID, hash string) ([]byte, error) {
	resp, err := c.do("GET", "/api/workspaces/"+wsID+"/blobs/"+hash, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}
	return io.ReadAll(resp.Body)
}

// Commit sets a workspace's file set. The returned CommitResult carries the
// outcome: a new Revision, a Conflict (with Current), or a Missing blob list.
// Only genuine transport/protocol failures return a non-nil error.
func (c *Client) Commit(wsID string, req syncproto.CommitRequest) (syncproto.CommitResult, error) {
	var res syncproto.CommitResult
	b, _ := json.Marshal(req)
	resp, err := c.do("POST", "/api/workspaces/"+wsID+"/commit", bytes.NewReader(b), "application/json")
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusConflict, http.StatusUnprocessableEntity:
		return res, json.NewDecoder(resp.Body).Decode(&res)
	default:
		return res, apiError(resp)
	}
}
