package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// JWTIssuer firma tokens HS256 con información mínima: sub, role, iss, iat, exp.
type JWTIssuer struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// NewJWTIssuer crea el emisor de tokens.
func NewJWTIssuer(secret, issuer string, ttl time.Duration) *JWTIssuer {
	return &JWTIssuer{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}
}

// Issue firma un token para el usuario.
func (j *JWTIssuer) Issue(userID string, role domain.Role) (string, time.Duration, error) {
	now := j.now()
	c := claims{
		Role: string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(j.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, j.ttl, nil
}

// Verify valida firma, algoritmo, emisor y expiración.
func (j *JWTIssuer) Verify(token string) (ports.Claims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(j.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return ports.Claims{}, errors.Join(domain.ErrInvalidToken, err)
	}
	role, err := domain.ParseRole(c.Role)
	if err != nil || c.Subject == "" {
		return ports.Claims{}, domain.ErrInvalidToken
	}
	return ports.Claims{UserID: c.Subject, Role: role}, nil
}
