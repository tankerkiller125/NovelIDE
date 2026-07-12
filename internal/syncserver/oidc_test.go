package syncserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"novelide/internal/syncproto"
)

// fakeIdP is a minimal stand-in OpenID Connect provider: discovery, token, and
// userinfo endpoints, enough to drive the server's SSO flow end to end.
func fakeIdP(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 base,
			"authorization_endpoint": base + "/authorize",
			"token_endpoint":         base + "/token",
			"userinfo_endpoint":      base + "/userinfo",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "good-code" ||
			r.Form.Get("code_verifier") == "" || r.Form.Get("client_id") != "client-x" {
			http.Error(w, "bad token request", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "AT-123", "token_type": "Bearer"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer AT-123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"sub": "user-123", "preferred_username": "alice"})
	})
	ts := httptest.NewServer(mux)
	base = ts.URL
	t.Cleanup(ts.Close)
	return ts
}

func ssoServer(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	idp := fakeIdP(t)
	oidc, err := NewOIDC("TestSSO", idp.URL, "client-x", "secret-x", "http://sync.local", "")
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}
	srv, err := New(Config{
		DataDir:           t.TempDir(),
		Secret:            []byte("test"),
		AuthMode:          mode,
		OIDC:              oidc,
		AllowRegistration: true,
		TokenTTL:          time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func TestSSOFlow(t *testing.T) {
	ts := ssoServer(t, AuthSSO)
	client := noRedirect()

	// auth/config advertises SSO only.
	resp, _ := http.Get(ts.URL + "/api/auth/config")
	var cfg syncproto.AuthConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	resp.Body.Close()
	if cfg.PasswordEnabled || !cfg.SSOEnabled || cfg.SSOName != "TestSSO" {
		t.Fatalf("auth config wrong: %+v", cfg)
	}

	// Password endpoints are disabled in sso mode.
	pr, _ := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"username":"x","password":"12345678"}`))
	if pr.StatusCode != http.StatusForbidden {
		t.Fatalf("login should be 403 in sso mode, got %d", pr.StatusCode)
	}

	// Start the flow — expect a redirect to the IdP with a state.
	appRedirect := "http://127.0.0.1:59999/callback"
	start := fmt.Sprintf("%s/api/sso/start?app_redirect=%s&app_state=APPSTATE", ts.URL, url.QueryEscape(appRedirect))
	resp, err := client.Get(start)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start should redirect, got %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	state := loc.Query().Get("state")
	if state == "" || loc.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL missing state/PKCE: %s", loc)
	}

	// The IdP would now redirect the browser to our callback with a code.
	cb := fmt.Sprintf("%s/api/sso/callback?code=good-code&state=%s", ts.URL, url.QueryEscape(state))
	resp, err = client.Get(cb)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback should redirect to the app, got %d", resp.StatusCode)
	}
	back, _ := url.Parse(resp.Header.Get("Location"))
	if got := back.Scheme + "://" + back.Host + back.Path; got != appRedirect {
		t.Fatalf("callback redirected to %q, want %q", got, appRedirect)
	}
	token := back.Query().Get("token")
	if token == "" || back.Query().Get("state") != "APPSTATE" {
		t.Fatalf("callback didn't return token+state: %s", back)
	}

	// The token identifies the auto-provisioned account.
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ = http.DefaultClient.Do(req)
	var me syncproto.MeResponse
	json.NewDecoder(resp.Body).Decode(&me)
	resp.Body.Close()
	if me.Username != "alice" || me.AccountID == "" {
		t.Fatalf("me wrong: %+v", me)
	}

	// A stale/unknown state is rejected (CSRF / replay protection).
	resp, _ = client.Get(fmt.Sprintf("%s/api/sso/callback?code=good-code&state=nonsense", ts.URL))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown state should fail, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSSOStartRejectsNonLoopback(t *testing.T) {
	ts := ssoServer(t, AuthBoth)
	client := noRedirect()
	// An external redirect target must be refused (open-redirect protection).
	resp, _ := client.Get(ts.URL + "/api/sso/start?app_redirect=" + url.QueryEscape("https://evil.example.com/steal") + "&app_state=x")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-loopback redirect should be 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// In "both" mode, password login still works.
	if !ssoServerBothPasswordWorks(t, ts) {
		t.Error("password auth should work in both mode")
	}
}

func ssoServerBothPasswordWorks(t *testing.T, ts *httptest.Server) bool {
	r, _ := http.Post(ts.URL+"/api/register", "application/json",
		strings.NewReader(`{"username":"bob","password":"password12"}`))
	defer r.Body.Close()
	return r.StatusCode == http.StatusOK
}
