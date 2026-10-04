// Package security implementa hashing de contraseñas y tokens JWT.
package security

import "golang.org/x/crypto/bcrypt"

// BcryptHasher implementa ports.PasswordHasher.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher crea un hasher con el costo indicado.
func NewBcryptHasher(cost int) BcryptHasher {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	return BcryptHasher{cost: cost}
}

// Hash devuelve el hash bcrypt de la contraseña.
func (h BcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	return string(b), err
}

// Compare indica si la contraseña corresponde al hash.
func (h BcryptHasher) Compare(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
