// Command novelide-sync is NovelIDE's optional, self-hostable, multi-account
// sync server. It is not required by the desktop app — run it only if you want
// to sync workspaces across devices.
//
// Configuration is via environment variables (all optional):
//
//	NOVELIDE_SYNC_ADDR                 listen address        (default ":8787")
//	NOVELIDE_SYNC_DATA                 data directory        (default "./sync-data")
//	NOVELIDE_SYNC_SECRET               token signing secret  (default: generated + persisted)
//	NOVELIDE_SYNC_ALLOW_REGISTRATION   "true"/"false"        (default "true")
//	NOVELIDE_SYNC_MAX_BLOB_MB          per-file upload cap    (default 100)
//	NOVELIDE_SYNC_TOKEN_DAYS           token lifetime in days (default 30)
//
// Authentication (default: password only). Set the mode and, for SSO, the
// OpenID Connect details of your provider (Zitadel, Authentik, Keycloak, …):
//
//	NOVELIDE_SYNC_AUTH_MODE            "password" | "sso" | "both"  (default "password")
//	NOVELIDE_SYNC_PUBLIC_URL           this server's public base URL (required for SSO)
//	NOVELIDE_SYNC_OIDC_ISSUER          provider issuer URL
//	NOVELIDE_SYNC_OIDC_CLIENT_ID       OIDC client id
//	NOVELIDE_SYNC_OIDC_CLIENT_SECRET   OIDC client secret
//	NOVELIDE_SYNC_OIDC_SCOPES          space-separated (default "openid profile email")
//	NOVELIDE_SYNC_OIDC_NAME            display label (default "SSO")
//
// The IdP must allow <PUBLIC_URL>/api/sso/callback as a redirect URI.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"novelide/internal/syncserver"
)

func main() {
	addr := env("NOVELIDE_SYNC_ADDR", ":8787")
	dataDir := env("NOVELIDE_SYNC_DATA", "./sync-data")

	secret, err := syncserver.LoadOrCreateSecret(dataDir, os.Getenv("NOVELIDE_SYNC_SECRET"))
	if err != nil {
		log.Fatalf("sync: could not load signing secret: %v", err)
	}

	authMode := env("NOVELIDE_SYNC_AUTH_MODE", syncserver.AuthPassword)
	cfg := syncserver.Config{
		DataDir:           dataDir,
		Secret:            secret,
		AllowRegistration: envBool("NOVELIDE_SYNC_ALLOW_REGISTRATION", true),
		TokenTTL:          time.Duration(envInt("NOVELIDE_SYNC_TOKEN_DAYS", 30)) * 24 * time.Hour,
		MaxBlobSize:       int64(envInt("NOVELIDE_SYNC_MAX_BLOB_MB", 100)) << 20,
		AuthMode:          authMode,
	}

	// Configure SSO if the auth mode calls for it.
	if authMode == syncserver.AuthSSO || authMode == syncserver.AuthBoth {
		oidc, err := syncserver.NewOIDC(
			env("NOVELIDE_SYNC_OIDC_NAME", "SSO"),
			os.Getenv("NOVELIDE_SYNC_OIDC_ISSUER"),
			os.Getenv("NOVELIDE_SYNC_OIDC_CLIENT_ID"),
			os.Getenv("NOVELIDE_SYNC_OIDC_CLIENT_SECRET"),
			os.Getenv("NOVELIDE_SYNC_PUBLIC_URL"),
			os.Getenv("NOVELIDE_SYNC_OIDC_SCOPES"),
		)
		if err != nil {
			log.Fatalf("sync: SSO is enabled but misconfigured: %v", err)
		}
		cfg.OIDC = oidc
	}

	srv, err := syncserver.New(cfg)
	if err != nil {
		log.Fatalf("sync: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		// Generous body timeout so large blob uploads on slow links don't die.
		WriteTimeout: 5 * time.Minute,
		ReadTimeout:  5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	log.Printf("NovelIDE sync server listening on %s (data: %s, registration: %v)",
		addr, dataDir, cfg.AllowRegistration)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("sync: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
