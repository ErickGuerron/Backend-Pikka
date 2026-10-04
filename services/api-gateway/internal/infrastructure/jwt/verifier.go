// Package jwt verifica localmente los tokens emitidos por el Auth Service.
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken indica un token ausente, mal formado, expirado o con firma inválida.
var ErrInvalidToken = errors.New("invalid token")

var knownRoles = map[string]bool{"ADMIN": true, "OPERATOR": true, "DRIVER": true}

// Identity es la identidad autenticada que el Gateway propaga a los servicios.
type Identity struct {
	UserID string
	Role   string
}

// Verifier valida tokens HS256 sin llamar al Auth Service en cada petición.
type Verifier struct {
	secret []byte
	issuer string
	now    func() time.Time
}

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// NewVerifier crea el verificador.
func NewVerifier(secret, issuer string) *Verifier {
	return &Verifier{secret: []byte(secret), issuer: issuer, now: time.Now}
}

// Verify valida el token y devuelve la identidad.
func (v *Verifier) Verify(token string) (Identity, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil || c.Subject == "" || !knownRoles[c.Role] {
		return Identity{}, ErrInvalidToken
	}
	return Identity{UserID: c.Subject, Role: c.Role}, nil
}
