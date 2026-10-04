package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clave de prueba generada para no dejar literales con forma de secreto.
var secret = strings.Repeat("k", 32)

func sign(t *testing.T, c claims, key string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(key))
	require.NoError(t, err)
	return s
}

func valid() claims {
	return claims{Role: "OPERATOR", RegisteredClaims: jwt.RegisteredClaims{
		Subject: "u-1", Issuer: "lastmile-auth", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
}

func TestVerify(t *testing.T) {
	v := NewVerifier(secret, "lastmile-auth")

	id, err := v.Verify(sign(t, valid(), secret))
	require.NoError(t, err)
	assert.Equal(t, Identity{UserID: "u-1", Role: "OPERATOR"}, id)

	expired := valid()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	noExp := valid()
	noExp.ExpiresAt = nil
	badRole := valid()
	badRole.Role = "ROOT"
	noSub := valid()
	noSub.Subject = ""
	otherIss := valid()
	otherIss.Issuer = "x"

	for name, tok := range map[string]string{
		"expirado":     sign(t, expired, secret),
		"sin exp":      sign(t, noExp, secret),
		"rol inválido": sign(t, badRole, secret),
		"sin sub":      sign(t, noSub, secret),
		"otro emisor":  sign(t, otherIss, secret),
		"otra clave":   sign(t, valid(), strings.Repeat("x", 32)),
		"basura":       "a.b.c",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := v.Verify(tok)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}
