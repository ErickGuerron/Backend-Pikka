// Package persistence implementa los repositorios sobre PostgreSQL.
package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

const uniqueViolation = "23505"

// UserRepository implementa ports.UserRepository. Solo toca el esquema auth,
// fijado por el search_path de la conexión.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository crea el repositorio.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

const userColumns = `id, email, full_name, password_hash, role, active, version, created_at, updated_at`

// Create inserta un usuario y devuelve la fila con id y timestamps.
func (r *UserRepository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, full_name, password_hash, role, active)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		u.Email, u.FullName, u.PasswordHash, string(u.Role), u.Active)
	created, err := scanUser(row)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.User{}, domain.ErrEmailAlreadyExists
	}
	return created, err
}

// FindByEmail busca por correo normalizado.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

// FindByID busca por id; un id que no es UUID se trata como inexistente.
func (r *UserRepository) FindByID(ctx context.Context, id string) (domain.User, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	return scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// Count devuelve el número de usuarios.
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// List devuelve todos los usuarios ordenados por fecha de creación descendente.
// La autorización por rol se valida en la capa de aplicación, no acá: este
// método solo lee filas cuando ya pasó la comprobación de Caller.
func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		var role string
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.PasswordHash, &role, &u.Active, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Role = domain.Role(role)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.PasswordHash, &role, &u.Active, &u.Version, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	u.Role = domain.Role(role)
	return u, nil
}
