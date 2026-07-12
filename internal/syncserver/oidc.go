package syncserver

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDC implements Single Sign-On against any OpenID Connect provider (Zitadel,
// Authentik, Keycloak, …). The sync server is a confidential OIDC client: it
// runs the Authorization Code flow with PKCE, exchanges the code server-side
// with its client secret, and reads the user's identity from the provider's
// UserInfo endpoint (reached over TLS with the freshly-obtained access token).
//
// The desktop app never handles OIDC directly — it opens the system browser at
// /api/sso/start and receives the server's own session token on a loopback
// redirect once sign-in completes.
type OIDC struct {
	name         string
	clientID     string
	clientSecret string
	redirectURI  string // this server's callback, registered at the IdP
	scopes       string
	ep           endpoints
	httpc        *http.Client

	mu      sync.Mutex
	pending map[string]pendingAuth // keyed by OIDC state
}

type endpoints struct {
	Issuer      string `json:"issuer"`
	AuthURL     string `json:"authorization_endpoint"`
	TokenURL    string `json:"token_endpoint"`
	UserInfoURL string `json:"userinfo_endpoint"`
}

type pendingAuth struct {
	appRedirect string // loopback URL to hand the token back to the app
	appState    string // app's CSRF token, echoed back
	verifier    string // PKCE code_verifier
	exp         time.Time
}

// UserIdentity is the minimal identity extracted from the IdP.
type UserIdentity struct {
	Subject  string // stable, unique: "<issuer>|<sub>"
	Username string // display label
}

// NewOIDC discovers the provider's endpoints and builds the client. publicURL
// is this server's externally reachable base URL (used to form the callback the
// IdP redirects to).
func NewOIDC(name, issuer, clientID, clientSecret, publicURL, scopes string) (*OIDC, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if issuer == "" || clientID == "" || publicURL == "" {
		return nil, fmt.Errorf("OIDC needs an issuer, client id, and public URL")
	}
	if scopes == "" {
		scopes = "openid profile email"
	}
	httpc := &http.Client{Timeout: 15 * time.Second}
	ep, err := discover(httpc, issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	if name == "" {
		name = "SSO"
	}
	return &OIDC{
		name:         name,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  publicURL + "/api/sso/callback",
		scopes:       scopes,
		ep:           ep,
		httpc:        httpc,
		pending:      map[string]pendingAuth{},
	}, nil
}

func discover(httpc *http.Client, issuer string) (endpoints, error) {
	var ep endpoints
	resp, err := httpc.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		return ep, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ep, fmt.Errorf("discovery returned %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return ep, err
	}
	if ep.AuthURL == "" || ep.TokenURL == "" || ep.UserInfoURL == "" {
		return ep, fmt.Errorf("discovery document is missing required endpoints")
	}
	return ep, nil
}

// Authorize validates the app's loopback redirect, records a pending flow, and
// returns the provider's authorization URL to send the browser to.
func (o *OIDC) Authorize(appRedirect, appState string) (string, error) {
	if !isLoopbackURL(appRedirect) {
		return "", fmt.Errorf("app redirect must be a loopback URL")
	}
	state := randToken()
	verifier := randToken()
	challenge := pkceChallenge(verifier)

	o.mu.Lock()
	o.sweep()
	o.pending[state] = pendingAuth{appRedirect: appRedirect, appState: appState, verifier: verifier, exp: time.Now().Add(10 * time.Minute)}
	o.mu.Unlock()

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", o.clientID)
	q.Set("redirect_uri", o.redirectURI)
	q.Set("scope", o.scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return o.ep.AuthURL + "?" + q.Encode(), nil
}

// Exchange completes the flow: it looks up the pending state, swaps the code
// for tokens, and reads the user's identity from UserInfo. It returns the
// identity plus where to send the browser next (the app's loopback).
func (o *OIDC) Exchange(state, code string) (UserIdentity, string, string, error) {
	o.mu.Lock()
	p, ok := o.pending[state]
	delete(o.pending, state)
	o.mu.Unlock()
	if !ok || time.Now().After(p.exp) {
		return UserIdentity{}, "", "", fmt.Errorf("unknown or expired sign-in state")
	}

	accessToken, err := o.exchangeCode(code, p.verifier)
	if err != nil {
		return UserIdentity{}, "", "", err
	}
	id, err := o.userInfo(accessToken)
	if err != nil {
		return UserIdentity{}, "", "", err
	}
	return id, p.appRedirect, p.appState, nil
}

func (o *OIDC) exchangeCode(code, verifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", o.redirectURI)
	form.Set("client_id", o.clientID)
	if o.clientSecret != "" {
		form.Set("client_secret", o.clientSecret)
	}
	form.Set("code_verifier", verifier)

	resp, err := o.httpc.PostForm(o.ep.TokenURL, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (%d)", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token response had no access_token")
	}
	return tok.AccessToken, nil
}

func (o *OIDC) userInfo(accessToken string) (UserIdentity, error) {
	req, _ := http.NewRequest("GET", o.ep.UserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := o.httpc.Do(req)
	if err != nil {
		return UserIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UserIdentity{}, fmt.Errorf("userinfo returned %d", resp.StatusCode)
	}
	var claims struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return UserIdentity{}, err
	}
	if claims.Sub == "" {
		return UserIdentity{}, fmt.Errorf("userinfo had no subject")
	}
	name := firstNonEmpty(claims.PreferredUsername, claims.Email, claims.Name, claims.Sub)
	return UserIdentity{Subject: o.ep.Issuer + "|" + claims.Sub, Username: name}, nil
}

// sweep drops expired pending flows. Caller holds the lock.
func (o *OIDC) sweep() {
	now := time.Now()
	for k, v := range o.pending {
		if now.After(v.exp) {
			delete(o.pending, k)
		}
	}
}

// isLoopbackURL reports whether raw is an http(s) URL pointing at localhost —
// the only redirect target we'll hand a session token to, preventing token
// exfiltration via an open redirect.
func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

var b64url = base64.RawURLEncoding

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b64url.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return b64url.EncodeToString(sum[:])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
