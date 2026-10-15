package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// orderColumns es la lista canónica en el orden que scanOrder espera. Las fechas
// se devuelven como texto con el formato del contrato (YYYY-MM-DD y HH:mm).
const orderColumns = `id::text, COALESCE(customer_id::text, ''), COALESCE(zone_id::text, ''), address, latitude, longitude,
	to_char(delivery_date, 'YYYY-MM-DD'), to_char(window_start, 'HH24:MI'), to_char(window_end, 'HH24:MI'),
	status, review_reasons, version, created_at, COALESCE(route_id, '')`

// OrderRepository es la implementación PostgreSQL de ports.OrderRepository.
type OrderRepository struct {
	pool *pgxpool.Pool
}

var _ ports.OrderRepository = (*OrderRepository)(nil)

// NewOrderRepository inyecta el pool pgx.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// Create inserta el pedido y, si hay notificación, la guarda en la misma transacción.
// Las restricciones de la tabla garantizan la coherencia de estado y motivos.
func (r *OrderRepository) Create(ctx context.Context, order domain.Order, notice *domain.Notification) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("create order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := insertOrder(ctx, tx, order)
	if err != nil {
		return domain.Order{}, err
	}
	if notice != nil {
		if err := insertNotification(ctx, tx, out.CustomerID, out.ID, *notice); err != nil {
			return domain.Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("create order: commit: %w", err)
	}
	return out, nil
}

// insertOrder inserta la fila del pedido dentro de una transacción ya abierta y devuelve la fila
// tal como quedó guardada (con el id que asigna la base). Lo comparten Create y el descuento de stock.
func insertOrder(ctx context.Context, tx pgx.Tx, order domain.Order) (domain.Order, error) {
	customerID, err := uuid.Parse(order.CustomerID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("create order: customer id: %w", err)
	}
	zoneID, err := optionalUUID(order.ZoneID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("create order: zone id: %w", err)
	}
	out, err := scanOrder(tx.QueryRow(ctx,
		`INSERT INTO orders (customer_id, zone_id, address, latitude, longitude, delivery_date,
		                     window_start, window_end, status, review_reasons)
		 VALUES ($1, $2, $3, $4, $5, $6::date, $7::time, $8::time, $9, $10)
		 RETURNING `+orderColumns,
		customerID, zoneID, order.Address, order.Latitude, order.Longitude,
		order.DeliveryDate.String(), order.Window.Start(), order.Window.End(),
		string(order.Status), reasonStrings(order.ReviewReasons),
	))
	if err != nil {
		return domain.Order{}, fmt.Errorf("create order: %w", err)
	}
	return out, nil
}

// ListByCustomer devuelve los pedidos de un cliente, del más reciente al más antiguo.
func (r *OrderRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]domain.Order, error) {
	return r.queryOrders(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE customer_id = $1 ORDER BY created_at DESC, id ASC`,
		customerID)
}

// ListExceptions devuelve la cola de excepciones, la más antigua primero.
func (r *OrderRepository) ListExceptions(ctx context.Context) ([]domain.Order, error) {
	return r.queryOrders(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE status = 'PENDING_REVIEW' ORDER BY created_at ASC, id ASC`)
}

// Review aplica la decisión dentro de una transacción con bloqueo de fila
// (SELECT ... FOR UPDATE): dos operadores no pueden decidir el mismo pedido a la vez.
func (r *OrderRepository) Review(ctx context.Context, orderID string, decide func(domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("review order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("review order: load: %w", err)
	}

	next, notice, err := decide(current)
	if err != nil {
		return domain.Order{}, err
	}
	zoneID, err := optionalUUID(next.ZoneID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("review order: zone id: %w", err)
	}
	out, err := scanOrder(tx.QueryRow(ctx,
		`UPDATE orders
		    SET address = $2, latitude = $3, longitude = $4, delivery_date = $5::date,
		        window_start = $6::time, window_end = $7::time, zone_id = $8,
		        status = $9, review_reasons = $10, version = version + 1, updated_at = now()
		  WHERE id = $1
		RETURNING `+orderColumns,
		id, next.Address, next.Latitude, next.Longitude, next.DeliveryDate.String(),
		next.Window.Start(), next.Window.End(), zoneID, string(next.Status), reasonStrings(next.ReviewReasons),
	))
	if err != nil {
		return domain.Order{}, fmt.Errorf("review order: update: %w", err)
	}
	// Un rechazo cancela un pedido que ya reservó stock al crearse: el stock vuelve en la misma transacción.
	if current.Status != domain.StatusCancelled && out.Status == domain.StatusCancelled {
		if err := returnStock(ctx, tx, id, time.Now().UTC()); err != nil {
			return domain.Order{}, err
		}
	}
	if notice != nil {
		if err := insertNotification(ctx, tx, out.CustomerID, out.ID, *notice); err != nil {
			return domain.Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("review order: commit: %w", err)
	}
	return out, nil
}

// CancelForCustomer cancela el pedido del cliente y devuelve su stock en la misma transacción. La consulta
// filtra por cliente: un pedido ajeno no se bloquea ni se modifica y responde como inexistente (BOLA).
func (r *OrderRepository) CancelForCustomer(ctx context.Context, orderID string, customerID uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("cancel order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanOrder(tx.QueryRow(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE id = $1 AND customer_id = $2 FOR UPDATE`, id, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("cancel order: load: %w", err)
	}

	next, err := decide(current)
	if err != nil {
		return domain.Order{}, err
	}
	out, err := scanOrder(tx.QueryRow(ctx,
		`UPDATE orders
		    SET status = $2, review_reasons = $3, version = version + 1, updated_at = now()
		  WHERE id = $1
		RETURNING `+orderColumns,
		id, string(next.Status), reasonStrings(next.ReviewReasons),
	))
	if err != nil {
		return domain.Order{}, fmt.Errorf("cancel order: update: %w", err)
	}
	if err := returnStock(ctx, tx, id, time.Now().UTC()); err != nil {
		return domain.Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("cancel order: commit: %w", err)
	}
	return out, nil
}

func (r *OrderRepository) queryOrders(ctx context.Context, sql string, args ...any) ([]domain.Order, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	out := []domain.Order{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("query orders: %w", err)
		}
		out = append(out, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	return out, nil
}

// insertNotification guarda un aviso dentro de la transacción que cambió el pedido.
func insertNotification(ctx context.Context, tx pgx.Tx, customerID, orderID string, n domain.Notification) error {
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return fmt.Errorf("notification customer id: %w", err)
	}
	oid, err := uuid.Parse(orderID)
	if err != nil {
		return fmt.Errorf("notification order id: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO customer_notifications (customer_id, order_id, kind, detail) VALUES ($1, $2, $3, $4)`,
		cid, oid, string(n.Kind), n.Detail,
	); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func optionalUUID(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func reasonStrings(reasons []domain.ReviewReason) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, string(reason))
	}
	return out
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(row rowScanner) (domain.Order, error) {
	var (
		id, customerID, zoneID, address, deliveryDate, windowStart, windowEnd, status string
		latitude, longitude                                                           float64
		reasons                                                                       []string
		routeID                                                                       string
		version                                                                       int64
		createdAt                                                                     time.Time
	)
	if err := row.Scan(&id, &customerID, &zoneID, &address, &latitude, &longitude,
		&deliveryDate, &windowStart, &windowEnd, &status, &reasons, &version, &createdAt, &routeID); err != nil {
		return domain.Order{}, err
	}
	date, err := domain.ParseDeliveryDate(deliveryDate)
	if err != nil {
		return domain.Order{}, err
	}
	window, err := domain.NewDeliveryWindow(windowStart, windowEnd)
	if err != nil {
		return domain.Order{}, err
	}
	reviewReasons := make([]domain.ReviewReason, 0, len(reasons))
	for _, value := range reasons {
		reason, err := domain.ParseReviewReason(value)
		if err != nil {
			return domain.Order{}, err
		}
		reviewReasons = append(reviewReasons, reason)
	}
	parsedStatus, err := domain.ParseStatus(status)
	if err != nil {
		return domain.Order{}, err
	}
	return domain.Order{
		ID:            id,
		CustomerID:    customerID,
		ZoneID:        zoneID,
		RouteID:       routeID,
		Address:       address,
		Latitude:      latitude,
		Longitude:     longitude,
		DeliveryDate:  date,
		Window:        window,
		Status:        parsedStatus,
		ReviewReasons: reviewReasons,
		CreatedAt:     createdAt.UTC(),
		Version:       version,
	}, nil
}
