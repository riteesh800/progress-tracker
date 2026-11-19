package services

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
)

type GoogleUserInfo struct {
	Email string
	Sub   string
	Name  string
}

var VerifyGoogleTokenFunc = defaultVerifyGoogleToken

func VerifyGoogleToken(ctx context.Context, cfg *config.Settings, idToken string) (*GoogleUserInfo, error) {
	return VerifyGoogleTokenFunc(ctx, cfg, idToken)
}

var (
	jwksMu      sync.Mutex
	cachedKeys  map[string]*rsa.PublicKey
	jwksExpires time.Time
)

type jwksDoc struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func fetchGooglePublicKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	jwksMu.Lock()
	defer jwksMu.Unlock()

	if cachedKeys != nil && time.Now().Before(jwksExpires) {
		return cachedKeys, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/certs", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google jwks status %d", resp.StatusCode)
	}

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		eInt := 0
		for _, b := range eb {
			eInt = (eInt << 8) | int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nb),
			E: eInt,
		}
	}

	cachedKeys = keys
	jwksExpires = time.Now().Add(1 * time.Hour)
	return keys, nil
}

func defaultVerifyGoogleToken(ctx context.Context, cfg *config.Settings, idToken string) (*GoogleUserInfo, error) {
	if cfg.IsTest() {
		if !strings.HasPrefix(idToken, "test.") || !strings.Contains(idToken[5:], ".") {
			return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
		}
		email := idToken[5:]
		parts := strings.SplitN(email, "@", 2)
		return &GoogleUserInfo{
			Email: email,
			Sub:   "google-" + email,
			Name:  parts[0],
		}, nil
	}

	if strings.TrimSpace(cfg.GoogleClientID) == "" {
		return nil, core.NewAppError(503, "google_unconfigured", "Google Sign-In is not configured.")
	}

	if strings.HasPrefix(idToken, "test.") {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	segments := strings.Split(idToken, ".")
	if len(segments) != 3 {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(segments[0])
	if err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Alg != "RS256" || header.Kid == "" {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	keys, err := fetchGooglePublicKeys(ctx)
	if err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	pubKey, ok := keys[header.Kid]
	if !ok {
		jwksMu.Lock()
		jwksExpires = time.Time{}
		jwksMu.Unlock()
		keys, err = fetchGooglePublicKeys(ctx)
		if err != nil {
			return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
		}
		pubKey, ok = keys[header.Kid]
		if !ok {
			return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
		}
	}

	signedInput := segments[0] + "." + segments[1]
	digest := sha256.Sum256([]byte(signedInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, digest[:], sigBytes); err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	var claims struct {
		Iss           string `json:"iss"`
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Exp           int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	if claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com" {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	if claims.Aud != cfg.GoogleClientID {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	if time.Now().Unix() >= claims.Exp {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	if b, ok := claims.EmailVerified.(bool); ok && !b {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}
	if s, ok := claims.EmailVerified.(string); ok && strings.EqualFold(s, "false") {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	if strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Sub) == "" {
		return nil, core.NewAppError(401, "invalid_google_token", "Google token could not be verified.")
	}

	return &GoogleUserInfo{
		Email: claims.Email,
		Sub:   claims.Sub,
		Name:  claims.Name,
	}, nil
}
