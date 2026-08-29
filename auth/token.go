// Package auth — вход и удостоверение личности. Живёт НАД доменом: core и
// прочие чистые слои о нём не знают (см. e2e/architecture_test.go).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrExpired = errors.New("auth: токен истёк")
	ErrInvalid = errors.New("auth: токен недействителен")
)

const (
	accessTTL  = 24 * time.Hour
	refreshTTL = 30 * 24 * time.Hour
)

type Identity struct{ Sub, Email, Name, Picture string }

type claims struct {
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	now    func() time.Time
}

func NewTokens(secret string, now func() time.Time) *Tokens {
	if now == nil {
		now = time.Now
	}
	return &Tokens{secret: []byte(secret), now: now}
}

func (t *Tokens) IssueAccess(userID string) (string, time.Time, error) {
	exp := t.now().Add(accessTTL)
	c := claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(t.now()),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	return tok, exp, err
}

func (t *Tokens) VerifyAccess(token string) (string, error) {
	c := &claims{}
	_, err := jwt.ParseWithClaims(token, c, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return t.secret, nil
	}, jwt.WithTimeFunc(t.now))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpired
		}
		return "", ErrInvalid
	}
	return c.Subject, nil
}

func (t *Tokens) NewRefresh() (string, string, time.Time, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", time.Time{}, err
	}
	raw := hex.EncodeToString(b[:])
	return raw, HashRefresh(raw), t.now().Add(refreshTTL), nil
}

func HashRefresh(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
