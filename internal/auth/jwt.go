package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"
)

// DeriveSubkey returns an HKDF-SHA256 derived 32-byte subkey from master
// using the given purpose label as info. Salt is empty (acceptable for
// cryptographically-strong master secrets).
//
// Use this whenever a new cryptographic purpose is introduced so that
// rotating or compromising one subkey does not affect another. Today it
// is used for JWT signing; existing AES-GCM credential KEK and API key
// HMAC continue to use master_secret directly to preserve compatibility
// with stored ciphertexts and key hashes.
func DeriveSubkey(master, purpose string) []byte {
	r := hkdf.New(sha256.New, []byte(master), nil, []byte(purpose))
	out := make([]byte, 32)
	_, _ = io.ReadFull(r, out)
	return out
}

// ensure hmac/sha256 stay imported even when not all helpers below use them.
var _ = hmac.Equal

type JWTManager struct {
	secret []byte
	expiry time.Duration
}

type Claims struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"tv,omitempty"`
	jwt.RegisteredClaims
}

// jwtSubkeyPurpose is the HKDF info label for JWT signing keys. Including
// 'v1' lets future versions migrate by rotating the label.
const jwtSubkeyPurpose = "simpledeploy-jwt-v1"

// NewJWTManager derives a per-purpose subkey from the operator-provided
// master secret via HKDF. Compromise or rotation of the JWT key now does
// not require re-encrypting registry credentials or invalidating API keys.
func NewJWTManager(secret string, expiry time.Duration) *JWTManager {
	return &JWTManager{secret: DeriveSubkey(secret, jwtSubkeyPurpose), expiry: expiry}
}

// JWTIssuer/Audience bind tokens to this product, defending against
// secret reuse across services should the same master_secret ever be
// shared (e.g. operator copies prod config to staging).
const (
	JWTIssuer   = "simpledeploy"
	JWTAudience = "simpledeploy-dashboard"
)

func (m *JWTManager) Generate(userID int64, username, role string, tokenVersion int64) (string, error) {
	claims := &Claims{
		UserID:       userID,
		Username:     username,
		Role:         role,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    JWTIssuer,
			Audience:  jwt.ClaimStrings{JWTAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *JWTManager) Validate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(JWTIssuer), jwt.WithAudience(JWTAudience))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
