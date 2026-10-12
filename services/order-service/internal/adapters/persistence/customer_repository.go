// Package persistence contiene los adaptadores PostgreSQL de los puertos
// definidos en internal/application/ports.
package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// customerColumns es la lista canónica en orden en que se escanea la fila.
// Mantenerla en una constante evita errores tontos entre SELECT, INSERT/UPDATE
// RETURNING y la firma de scanCustomer.
const customerColumns = `id, user_id, email, full_name, status, pending_surcharge, created_at, updated_at`

// CustomerRepository es la implementación PostgreSQL de
// ports.CustomerRepository.
type CustomerRepository struct {
	pool *pgxpool.Pool
}

// NewCustomerRepository inyecta el pool pgx.
func NewCustomerRepository(pool *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: pool}
}

// Create persiste un Customer nuevo. Devuelve domain.ErrCustomerAlreadyExists
// cuando el UNIQUE INDEX sobre user_id (o email) rechaza la inserción; el
// código 23505 es unique_violation en PostgreSQL.
func (r *CustomerRepository) Create(ctx context.Context, c domain.Customer) (domain.Customer, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO customers (user_id, email, full_name, status, pending_surcharge)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+customerColumns,
		c.UserID, c.Email, c.FullName, string(c.Status), c.PendingSurcharge,
	)
	out, err := scanCustomer(row)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Customer{}, domain.ErrCustomerAlreadyExists
		}
		return domain.Customer{}, fmt.Errorf("create customer: %w", err)
	}
	return out, nil
}

// GetByUserID busca al cliente asociado a un usuario de auth.
func (r *CustomerRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (domain.Customer, error) {
	out, err := scanCustomer(r.pool.QueryRow(ctx,
		`SELECT `+customerColumns+` FROM customers WHERE user_id = $1`, userID))
	if err != nil {
		if errors.Is(err, domain.ErrCustomerNotFound) {
			return domain.Customer{}, err
		}
		return domain.Customer{}, fmt.Errorf("get customer by user_id: %w", err)
	}
	return out, nil
}

// GetByID busca por PK.
func (r *CustomerRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Customer, error) {
	out, err := scanCustomer(r.pool.QueryRow(ctx,
		`SELECT `+customerColumns+` FROM customers WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, domain.ErrCustomerNotFound) {
			return domain.Customer{}, err
		}
		return domain.Customer{}, fmt.Errorf("get customer by id: %w", err)
	}
	return out, nil
}

// UpdateStatus cambia el status y refresca updated_at a now(). El CHECK
// constraint sobre la columna status garantiza que solo valores válidos
// lleguen a la base; igual validamos en Go para no malgastar la transacción.
func (r *CustomerRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.CustomerStatus) (domain.Customer, error) {
	if !status.Valid() {
		return domain.Customer{}, domain.ErrInvalidStatus
	}
	out, err := scanCustomer(r.pool.QueryRow(ctx,
		`UPDATE customers SET status = $1, updated_at = now() WHERE id = $2 RETURNING `+customerColumns,
		string(status), id))
	if err != nil {
		if errors.Is(err, domain.ErrCustomerNotFound) {
			return domain.Customer{}, err
		}
		return domain.Customer{}, fmt.Errorf("update customer status: %w", err)
	}
	return out, nil
}

// scanCustomer mapea una fila a domain.Customer y traduce pgx.ErrNoRows
// a domain.ErrCustomerNotFound. Centraliza el manejo del ErrNoRows para
// no repetirlo en cada método.
func scanCustomer(row pgx.Row) (domain.Customer, error) {
	var (
		c         domain.Customer
		status    string
		surcharge decimal.Decimal
	)
	err := row.Scan(&c.ID, &c.UserID, &c.Email, &c.FullName, &status, &surcharge, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Customer{}, domain.ErrCustomerNotFound
		}
		return domain.Customer{}, err
	}
	c.Status = domain.CustomerStatus(status)
	c.PendingSurcharge = surcharge
	return c, nil
}

// isUniqueViolation reporta si el error de pgx corresponde a un
// UNIQUE/CONSTRAINT violation (código SQLSTATE 23505). Lo usamos para
// distinguir ErrCustomerAlreadyExists de otros errores de transporte.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// List devuelve una pagina de clientes y el total que cumple el filtro.
// Orden determinista (created_at, id); el SQL esta parametrizado y el limit
// ya viene acotado por el caso de uso (se re-acota aqui como defensa).
func (r *CustomerRepository) List(ctx context.Context, f ports.CustomerListFilter) ([]domain.Customer, int, error) {
	limit, offset := f.Limit, f.Offset
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var status *string
	if f.Status != nil {
		s := string(*f.Status)
		status = &s
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM customers WHERE ($1::text IS NULL OR status = $1)`, status,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count customers: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+customerColumns+` FROM customers
		 WHERE ($1::text IS NULL OR status = $1)
		 ORDER BY created_at ASC, id ASC
		 LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Customer, 0, limit)
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan customer: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list customers: %w", err)
	}
	return out, total, nil
}
