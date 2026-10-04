package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestJWTRoundTrip(t *testing.T) {
	j := NewJWTIssuer(secret, "lastmile-auth", time.Hour)
	tok, ttl, err := j.Issue("u-1", domain.RoleDriver)
	require.NoError(t, err)
	assert.Equal(t, time.Hour, ttl)

	c, err := j.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, "u-1", c.UserID)
	assert.Equal(t, domain.RoleDriver, c.Role)
}

func TestJWTRejects(t *testing.T) {
	j := NewJWTIssuer(secret, "lastmile-auth", time.Hour)
	tok, _, err := j.Issue("u-1", domain.RoleAdmin)
	require.NoError(t, err)

	t.Run("expirado", func(t *testing.T) {
		later := NewJWTIssuer(secret, "lastmile-auth", time.Hour)
		later.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
		_, err := later.Verify(tok)
		assert.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("otra clave", func(t *testing.T) {
		other := NewJWTIssuer("ffffffffffffffffffffffffffffffff", "lastmile-auth", time.Hour)
		_, err := other.Verify(tok)
		assert.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("otro emisor", func(t *testing.T) {
		other := NewJWTIssuer(secret, "someone-else", time.Hour)
		_, err := other.Verify(tok)
		assert.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("algoritmo none", func(t *testing.T) {
		unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims{Role: "ADMIN", RegisteredClaims: jwt.RegisteredClaims{
			Subject: "u-1", Issuer: "lastmile-auth", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)
		_, err = j.Verify(unsigned)
		assert.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("rol desconocido", func(t *testing.T) {
		bad, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{Role: "ROOT", RegisteredClaims: jwt.RegisteredClaims{
			Subject: "u-1", Issuer: "lastmile-auth", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}}).SignedString([]byte(secret))
		require.NoError(t, err)
		_, err = j.Verify(bad)
		assert.ErrorIs(t, err, domain.ErrInvalidToken)
	})
}

func TestBcrypt(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)
	hash, err := h.Hash("password123")
	require.NoError(t, err)
	assert.True(t, h.Compare(hash, "password123"))
	assert.False(t, h.Compare(hash, "password124"))
}
