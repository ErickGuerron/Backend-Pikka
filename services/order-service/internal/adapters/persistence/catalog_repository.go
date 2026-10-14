package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Store implementa CatalogRepository y OrderRepository sobre el esquema orders.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore crea el adaptador sobre un pool ya abierto.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var (
	_ ports.CatalogRepository = (*Store)(nil)
	_ ports.OrderRepository   = (*Store)(nil)
)

func (s *Store) CreateProduct(ctx context.Context, p domain.Product, initial *domain.StockMovement) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO products (id, name, description, unit_price, active, stock_quantity, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			p.ID, p.Name, p.Description, p.UnitPrice, p.Active, p.StockQuantity, p.CreatedAt, p.UpdatedAt,
		)
		if err != nil {
			return err
		}
		if initial != nil {
			return insertMovement(ctx, tx, *initial)
		}
		return nil
	})
}

func (s *Store) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	p, err := scanProduct(s.pool.QueryRow(ctx, productColumns+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return p, err
}

func (s *Store) ListActiveProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := s.pool.Query(ctx, productColumns+` WHERE active ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SetActive(ctx context.Context, id uuid.UUID, active bool, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE products SET active = $2, updated_at = $3 WHERE id = $1`, id, active, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProductNotFound
	}
	return nil
}

// AdjustStock bloquea el producto, aplica la regla del dominio (motivo, signo, no negativo) y guarda el movimiento.
func (s *Store) AdjustStock(ctx context.Context, productID uuid.UUID, kind domain.MovementType, delta int64, reason string, actor *uuid.UUID, now time.Time) (domain.StockMovement, error) {
	var movement domain.StockMovement
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		p, err := scanProduct(tx.QueryRow(ctx, productColumns+` WHERE id = $1 FOR UPDATE`, productID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrProductNotFound
		}
		if err != nil {
			return err
		}
		movement, err = p.ApplyMovement(kind, delta, reason, nil, actor, now)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE products SET stock_quantity = $2, updated_at = $3 WHERE id = $1`,
			productID, *p.StockQuantity, now); err != nil {
			return err
		}
		return insertMovement(ctx, tx, movement)
	})
	return movement, err
}

// PlaceOrder inserta el pedido y sus líneas, y descuenta el stock de cada producto controlado.
// El descuento es condicional: la base no deja bajar de cero aunque dos pedidos lleguen a la vez.
// Los productos se bloquean en orden de id para evitar esperas circulares entre pedidos.
func (s *Store) PlaceOrder(ctx context.Context, order domain.Order, lines []domain.OrderLine, actor *uuid.UUID) error {
	if len(lines) == 0 {
		return domain.ErrNoItems
	}
	ordered := append([]domain.OrderLine(nil), lines...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ProductID.String() < ordered[j].ProductID.String() })

	return s.inTx(ctx, func(tx pgx.Tx) error {
		orderID, err := uuid.Parse(order.ID)
		if err != nil {
			return fmt.Errorf("order id: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO orders (id, address, latitude, longitude, delivery_date, window_start, window_end,
			                    status, review_reasons, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::date, $6::time, $7::time, $8, '{}', 1, $9, $9)`,
			orderID, order.Address, order.Latitude, order.Longitude, order.DeliveryDate.String(),
			order.Window.Start(), order.Window.End(), string(order.Status), order.CreatedAt,
		); err != nil {
			return err
		}

		for _, line := range ordered {
			if err := s.takeStock(ctx, tx, line, orderID, actor, order.CreatedAt); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO order_items (order_id, product_id, name, unit_price, quantity)
				VALUES ($1, $2, $3, $4, $5)`,
				orderID, line.ProductID, line.Name, line.UnitPrice, line.Quantity,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// takeStock descuenta una línea. Un producto sin control de stock solo se valida y no genera movimiento.
func (s *Store) takeStock(ctx context.Context, tx pgx.Tx, line domain.OrderLine, orderID uuid.UUID, actor *uuid.UUID, now time.Time) error {
	var resulting *int64
	err := tx.QueryRow(ctx, `
		UPDATE products SET stock_quantity = stock_quantity - $2, updated_at = $3
		WHERE id = $1 AND active AND stock_quantity IS NOT NULL AND stock_quantity >= $2
		RETURNING stock_quantity`, line.ProductID, line.Quantity, now).Scan(&resulting)
	if err == nil {
		oid := orderID
		movement := domain.StockMovement{
			ID:             uuid.New(),
			ProductID:      line.ProductID,
			Type:           domain.MovementSale,
			Delta:          -line.Quantity,
			ResultingStock: *resulting,
			OrderID:        &oid,
			ActorUserID:    actor,
			CreatedAt:      now,
		}
		return insertMovement(ctx, tx, movement)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// La actualización no afectó filas: distinguir por qué.
	p, err := scanProduct(tx.QueryRow(ctx, productColumns+` WHERE id = $1`, line.ProductID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProductNotFound
	}
	if err != nil {
		return err
	}
	if !p.Active {
		return domain.ErrProductInactive
	}
	if p.StockQuantity == nil {
		return nil
	}
	return domain.ErrInsufficientStock
}

const productColumns = `SELECT id, name, description, unit_price, active, stock_quantity, created_at, updated_at FROM products`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProduct(row rowScanner) (domain.Product, error) {
	var p domain.Product
	var price decimal.Decimal
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &price, &p.Active, &p.StockQuantity, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return domain.Product{}, err
	}
	p.UnitPrice = price
	return p, nil
}

func insertMovement(ctx context.Context, tx pgx.Tx, m domain.StockMovement) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO stock_movements (id, product_id, movement_type, delta, resulting_stock, reason, order_id, actor_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		m.ID, m.ProductID, string(m.Type), m.Delta, m.ResultingStock, m.Reason, m.OrderID, m.ActorUserID, m.CreatedAt,
	)
	return err
}

// inTx ejecuta fn en una transacción; cualquier error la revierte por completo.
func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
