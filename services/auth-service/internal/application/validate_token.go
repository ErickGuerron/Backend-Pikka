package application

import (
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// ValidateToken verifica un token de acceso.
type ValidateToken struct {
	tokens ports.TokenIssuer
}

// NewValidateToken construye el caso de uso.
func NewValidateToken(tokens ports.TokenIssuer) *ValidateToken {
	return &ValidateToken{tokens: tokens}
}

// Execute devuelve la identidad contenida en el token.
func (uc *ValidateToken) Execute(token string) (ports.Claims, error) {
	if token == "" {
		return ports.Claims{}, domain.ErrInvalidToken
	}
	return uc.tokens.Verify(token)
}
