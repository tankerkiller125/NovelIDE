// Package syncserver implements NovelIDE's optional multi-account sync server.
//
// It is entirely standalone: the desktop app never requires it and works fully
// offline. A user who wants cross-device sync points the app at a server they
// (or someone they trust) run. Each account's workspaces are isolated on disk,
// and a workspace is synced as a content-addressed set of files with a
// per-workspace revision for optimistic concurrency.
package syncserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"novelide/internal/syncproto"
)

// Auth modes.
const (
	AuthPassword = "password" // username/password only (default)
	AuthSSO      = "sso"      // OIDC single sign-on only
	AuthBoth     = "both"     // both methods offered
)

// Config controls a running server.
type Config struct {
	DataDir           string
	Secret            []byte        // HMAC signing secret for tokens
	AllowRegistration bool          // if false, password /api/register is disabled
	TokenTTL          time.Duration // how long issued tokens stay valid
	MaxBlobSize       int64         // max bytes per uploaded blob
	AuthMode          string        // AuthPassword | AuthSSO | AuthBoth
	OIDC              *OIDC         // configured when SSO is enabled
}

// Server is the HTTP sync service.
type Server struct {
	store *Store
	cfg   Config
	log   *log.Logger
}

const maxJSONBody = 32 << 20 // 32 MiB — generous for large manifests

// New creates a Server, opening the data store under cfg.DataDir.
func New(cfg Config) (*Server, error) {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = 30 * 24 * time.Hour
	}
	if cfg.MaxBlobSize <= 0 {
		cfg.MaxBlobSize = 100 << 20 // 100 MiB
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = AuthPassword
	}
	store, err := NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	return &Server{store: store, cfg: cfg, log: log.New(os.Stderr, "syncserver ", log.LstdFlags)}, nil
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/auth/config", s.authConfig)
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/sso/start", s.ssoStart)
	mux.HandleFunc("GET /api/sso/callback", s.ssoCallback)
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/workspaces", s.auth(s.listWorkspaces))
	mux.HandleFunc("GET /api/workspaces/{id}/manifest", s.auth(s.manifest))
	mux.HandleFunc("POST /api/workspaces/{id}/commit", s.auth(s.commit))
	mux.HandleFunc("HEAD /api/workspaces/{id}/blobs/{hash}", s.auth(s.headBlob))
	mux.HandleFunc("PUT /api/workspaces/{id}/blobs/{hash}", s.auth(s.putBlob))
	mux.HandleFunc("GET /api/workspaces/{id}/blobs/{hash}", s.auth(s.getBlob))
	return mux
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, syncproto.ErrorResponse{Error: msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if v, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

type authHandler func(w http.ResponseWriter, r *http.Request, accountID string)

// auth wraps a handler, requiring a valid bearer token whose account still
// exists, and passes the account id through.
func (s *Server) auth(h authHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accID, err := verifyToken(s.cfg.Secret, bearer(r), time.Now())
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}
		if _, err := s.store.AccountByID(accID); err != nil {
			writeErr(w, http.StatusUnauthorized, "account no longer exists")
			return
		}
		h(w, r, accID)
	}
}

// --- handlers ---

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) passwordEnabled() bool {
	return s.cfg.AuthMode == AuthPassword || s.cfg.AuthMode == AuthBoth
}

func (s *Server) ssoEnabled() bool {
	return (s.cfg.AuthMode == AuthSSO || s.cfg.AuthMode == AuthBoth) && s.cfg.OIDC != nil
}

// authConfig reports which sign-in methods this server offers.
func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	cfg := syncproto.AuthConfig{
		PasswordEnabled: s.passwordEnabled(),
		SSOEnabled:      s.ssoEnabled(),
	}
	if cfg.SSOEnabled {
		cfg.SSOName = s.cfg.OIDC.name
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, accID string) {
	acc, err := s.store.AccountByID(accID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, syncproto.MeResponse{AccountID: acc.ID, Username: acc.Username})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.passwordEnabled() {
		writeErr(w, http.StatusForbidden, "password sign-in is disabled; use SSO")
		return
	}
	if !s.cfg.AllowRegistration {
		writeErr(w, http.StatusForbidden, "registration is disabled on this server")
		return
	}
	var req syncproto.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	acc, err := s.store.CreateAccount(req.Username, req.Password, time.Now())
	switch {
	case errors.Is(err, ErrExists):
		writeErr(w, http.StatusConflict, "username already taken")
		return
	case errors.Is(err, ErrInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		s.log.Printf("register: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	s.issue(w, acc)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.passwordEnabled() {
		writeErr(w, http.StatusForbidden, "password sign-in is disabled; use SSO")
		return
	}
	var req syncproto.LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	acc, err := s.store.Authenticate(req.Username, req.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.issue(w, acc)
}

func (s *Server) issue(w http.ResponseWriter, acc Account) {
	token := signToken(s.cfg.Secret, acc.ID, s.cfg.TokenTTL, time.Now())
	writeJSON(w, http.StatusOK, syncproto.AuthResponse{
		Token:     token,
		AccountID: acc.ID,
		Username:  acc.Username,
	})
}

// ssoStart begins an OIDC sign-in: it records the app's loopback redirect and
// sends the browser to the identity provider.
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	if !s.ssoEnabled() {
		writeErr(w, http.StatusForbidden, "SSO is not enabled on this server")
		return
	}
	appRedirect := r.URL.Query().Get("app_redirect")
	appState := r.URL.Query().Get("app_state")
	authURL, err := s.cfg.OIDC.Authorize(appRedirect, appState)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// ssoCallback is the redirect target registered at the IdP. It completes the
// exchange, provisions the account, and hands the app its session token back
// on the loopback redirect.
func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if !s.ssoEnabled() {
		ssoErrorPage(w, "SSO is not enabled on this server")
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		ssoErrorPage(w, "Sign-in was cancelled or failed: "+e)
		return
	}
	id, appRedirect, appState, err := s.cfg.OIDC.Exchange(q.Get("state"), q.Get("code"))
	if err != nil {
		s.log.Printf("sso callback: %v", err)
		ssoErrorPage(w, "Sign-in could not be completed. You can close this window and try again.")
		return
	}
	acc, err := s.store.UpsertOIDCAccount(id.Subject, id.Username, time.Now())
	if err != nil {
		s.log.Printf("sso upsert: %v", err)
		ssoErrorPage(w, "Could not provision your account.")
		return
	}
	token := signToken(s.cfg.Secret, acc.ID, s.cfg.TokenTTL, time.Now())

	u, err := url.Parse(appRedirect)
	if err != nil { // Authorize already validated it's a loopback URL
		ssoErrorPage(w, "Invalid application redirect.")
		return
	}
	rq := u.Query()
	rq.Set("token", token)
	rq.Set("state", appState)
	u.RawQuery = rq.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func ssoErrorPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Sign-in</title>"+
		"<body style=\"font-family:system-ui;padding:3rem;text-align:center\">"+
		"<h2>NovelIDE sign-in</h2><p>%s</p></body>", htmlEscape(msg))
}

// htmlEscape is a tiny escaper for the fixed error strings above.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request, accID string) {
	ws, err := s.store.ListWorkspaces(accID)
	if err != nil {
		s.log.Printf("list: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not list workspaces")
		return
	}
	writeJSON(w, http.StatusOK, syncproto.WorkspaceList{Workspaces: ws})
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request, accID string) {
	m, err := s.store.Manifest(accID, r.PathValue("id"))
	if errors.Is(err, ErrInvalid) {
		writeErr(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	if err != nil {
		s.log.Printf("manifest: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not read manifest")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) commit(w http.ResponseWriter, r *http.Request, accID string) {
	var req syncproto.CommitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.store.Commit(accID, r.PathValue("id"), req, time.Now())
	if errors.Is(err, ErrInvalid) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.log.Printf("commit: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not commit")
		return
	}
	switch {
	case res.Conflict:
		writeJSON(w, http.StatusConflict, res) // client should merge against Current
	case len(res.Missing) > 0:
		writeJSON(w, http.StatusUnprocessableEntity, res) // upload blobs, then retry
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *Server) headBlob(w http.ResponseWriter, r *http.Request, accID string) {
	if s.store.HasBlob(accID, r.PathValue("id"), r.PathValue("hash")) {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) putBlob(w http.ResponseWriter, r *http.Request, accID string) {
	wsID, hash := r.PathValue("id"), r.PathValue("hash")
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBlobSize)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "blob exceeds the size limit")
		return
	}
	if err := s.store.PutBlob(accID, wsID, hash, data); err != nil {
		if errors.Is(err, ErrInvalid) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.log.Printf("putBlob: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not store blob")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) getBlob(w http.ResponseWriter, r *http.Request, accID string) {
	data, err := s.store.GetBlob(accID, r.PathValue("id"), r.PathValue("hash"))
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "blob not found")
		return
	}
	if errors.Is(err, ErrInvalid) {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err != nil {
		s.log.Printf("getBlob: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not read blob")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

// LoadOrCreateSecret returns the token-signing secret. If env is non-empty it
// is used directly; otherwise a random secret is generated once and persisted
// under dir so tokens survive restarts.
func LoadOrCreateSecret(dir, env string) ([]byte, error) {
	if env != "" {
		return []byte(env), nil
	}
	path := filepath.Join(dir, "secret.key")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, secret); err != nil {
		return nil, err
	}
	return secret, nil
}
