package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	jwt.RegisteredClaims
	Email       string `json:"email"`
	AAL         string `json:"aal"`
	SessionID   string `json:"sid"`
	AuthVersion int    `json:"ver"`
}

type Principal struct {
	UserID, Email, AAL, SessionID string
	AuthVersion                   int
}

type Verifier struct {
	secret           []byte
	issuer, audience string
}

type Signer struct {
	secret           []byte
	issuer, audience string
	accessTTL        time.Duration
}

func validateSecret(secret string) error {
	if len(strings.TrimSpace(secret)) < 32 {
		return errors.New("AUTH_JWT_SECRET must be at least 32 characters")
	}
	return nil
}

func NewVerifier(secret, issuer, audience string) (*Verifier, error) {
	if err := validateSecret(secret); err != nil {
		return nil, err
	}
	return &Verifier{secret: []byte(secret), issuer: issuer, audience: audience}, nil
}

func NewSigner(secret, issuer, audience string, accessTTL time.Duration) (*Signer, error) {
	if err := validateSecret(secret); err != nil {
		return nil, err
	}
	if accessTTL < time.Minute || accessTTL > 24*time.Hour {
		return nil, errors.New("access token TTL must be between 1 minute and 24 hours")
	}
	return &Signer{secret: []byte(secret), issuer: issuer, audience: audience, accessTTL: accessTTL}, nil
}

func (s *Signer) AccessToken(userID, email, sessionID, aal string, authVersion int) (string, time.Time, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return "", time.Time{}, fmt.Errorf("invalid user id: %w", err)
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		return "", time.Time{}, fmt.Errorf("invalid session id: %w", err)
	}
	if aal == "" {
		aal = "aal1"
	}
	now := time.Now().UTC()
	exp := now.Add(s.accessTTL)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{s.audience},
			ExpiresAt: jwt.NewNumericDate(exp),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		Email:       email,
		AAL:         aal,
		SessionID:   sessionID,
		AuthVersion: authVersion,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	raw, err := t.SignedString(s.secret)
	return raw, exp, err
}

func (v *Verifier) Verify(_ context.Context, raw string) (Principal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8192 {
		return Principal{}, errors.New("unauthorized")
	}
	c := &Claims{}
	t, err := jwt.ParseWithClaims(raw, c, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil || t == nil || !t.Valid || c.Subject == "" || c.SessionID == "" || c.AuthVersion < 1 {
		return Principal{}, errors.New("unauthorized")
	}
	if _, err := uuid.Parse(c.Subject); err != nil {
		return Principal{}, errors.New("unauthorized")
	}
	if _, err := uuid.Parse(c.SessionID); err != nil {
		return Principal{}, errors.New("unauthorized")
	}
	aal := c.AAL
	if aal == "" {
		aal = "aal1"
	}
	if aal != "aal1" && aal != "aal2" {
		return Principal{}, errors.New("unauthorized")
	}
	return Principal{UserID: c.Subject, Email: c.Email, AAL: aal, SessionID: c.SessionID, AuthVersion: c.AuthVersion}, nil
}

func NewOpaqueToken(bytesLen int) (string, error) {
	if bytesLen < 16 {
		bytesLen = 16
	}
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
