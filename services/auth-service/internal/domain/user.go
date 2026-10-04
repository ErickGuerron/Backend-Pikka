package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minPasswordBytes = 8
	// bcrypt ignora todo lo que exceda 72 bytes, así que se rechaza explícitamente.
	maxPasswordBytes = 72
	maxFullNameRunes = 120
)

// User es un usuario del sistema. PasswordHash nunca sale del Auth Service.
type User struct {
	ID           string
	Email        string
	FullName     string
	PasswordHash string
	Role         Role
	Active       bool
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewUserInput contiene los datos necesarios para registrar un usuario.
type NewUserInput struct {
	Email    string
	Password string
	FullName string
	Role     Role
}

// NormalizedNewUser es la entrada validada y normalizada.
type NormalizedNewUser struct {
	Email    string
	Password string
	FullName string
	Role     Role
}

// Validate valida y normaliza los datos de un usuario nuevo.
func (in NewUserInput) Validate() (NormalizedNewUser, error) {
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return NormalizedNewUser{}, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return NormalizedNewUser{}, err
	}
	name := strings.TrimSpace(in.FullName)
	if name == "" || utf8.RuneCountInString(name) > maxFullNameRunes {
		return NormalizedNewUser{}, ErrInvalidFullName
	}
	if !in.Role.Valid() {
		return NormalizedNewUser{}, ErrInvalidRole
	}
	return NormalizedNewUser{Email: email, Password: in.Password, FullName: name, Role: in.Role}, nil
}

// NormalizeEmail valida un correo y lo devuelve en minúsculas y sin espacios.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// ValidatePassword aplica la política mínima de contraseñas.
func ValidatePassword(p string) error {
	if len(p) < minPasswordBytes || len(p) > maxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}
