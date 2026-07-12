package syncserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// hashPassword returns a bcrypt hash suitable for storage.
func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// checkPassword reports whether pw matches the stored bcrypt hash.
func checkPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// tokenClaims is the payload carried inside a signed bearer token.
type tokenClaims struct {
	Sub string `json:"sub"` // account id
	Exp int64  `json:"exp"` // unix seconds
}

var b64 = base64.RawURLEncoding

// signToken issues a stateless bearer token: base64url(payload).base64url(HMAC).
// No server-side session storage is needed — validity is verified purely from
// the signature and expiry.
func signToken(secret []byte, accountID string, ttl time.Duration, now time.Time) string {
	payload, _ := json.Marshal(tokenClaims{Sub: accountID, Exp: now.Add(ttl).Unix()})
	body := b64.EncodeToString(payload)
	return body + "." + b64.EncodeToString(sign(secret, body))
}

// verifyToken checks a token's signature and expiry, returning the account id.
func verifyToken(secret []byte, token string, now time.Time) (string, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", fmt.Errorf("malformed token")
	}
	want := b64.EncodeToString(sign(secret, body))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return "", fmt.Errorf("bad signature")
	}
	raw, err := b64.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("bad payload")
	}
	var c tokenClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", fmt.Errorf("bad payload")
	}
	if now.Unix() >= c.Exp {
		return "", fmt.Errorf("token expired")
	}
	if c.Sub == "" {
		return "", fmt.Errorf("empty subject")
	}
	return c.Sub, nil
}

func sign(secret []byte, msg string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// randomSecret generates a cryptographically-random signing secret.
func randomSecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
