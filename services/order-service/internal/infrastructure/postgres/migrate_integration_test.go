//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/infrastructure/postgres"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/migrations"
)

const insertSQL = `INSERT INTO orders
	(address, latitude, longitude, delivery_date, window_start, window_end, status, review_reasons, version)
	VALUES ($1, $2, $3, $4::date, $5::time, $6::time, $7, $8::text[], $9)`

func TestOrderMigrations(t *testing.T) {
	dsn := os.Getenv("ORDER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_TEST_DATABASE_URL is not set")
	}
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(dsn); err != nil {
		t.Fatalf("repeated migration must be a no-op: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	t.Run("schema isolation", func(t *testing.T) {
		var schema string
		var canAccessAuth, canAccessZones bool
		err := pool.QueryRow(ctx, `SELECT current_schema(),
			has_schema_privilege(current_user, 'auth', 'USAGE'),
			has_schema_privilege(current_user, 'zones', 'USAGE')`).Scan(&schema, &canAccessAuth, &canAccessZones)
		if err != nil || schema != "orders" || canAccessAuth || canAccessZones {
			t.Fatalf("schema isolation failed: %q auth=%v zones=%v err=%v", schema, canAccessAuth, canAccessZones, err)
		}
	})

	t.Run("defaults and independent dates", func(t *testing.T) {
		tx := testTx(t, ctx, pool)
		var id, status, date string
		var createdAt time.Time
		var version int64
		var reasons []string
		err := tx.QueryRow(ctx, `INSERT INTO orders
			(address, latitude, longitude, delivery_date, window_start, window_end)
			VALUES ('Avenida Amazonas 123', -0.18, -78.48, '2030-04-12', '09:00', '12:00')
			RETURNING id::text, status, delivery_date::text, created_at, version, review_reasons`).
			Scan(&id, &status, &date, &createdAt, &version, &reasons)
		if err != nil {
			t.Fatal(err)
		}
		if id == "" || status != "PENDING" || date != "2030-04-12" || version != 1 || len(reasons) != 0 || createdAt.IsZero() {
			t.Fatalf("invalid defaults: id=%s status=%s date=%s version=%d reasons=%v createdAt=%s", id, status, date, version, reasons, createdAt)
		}
		if createdAt.Format(time.DateOnly) == date {
			t.Fatal("delivery date was replaced by registration date")
		}
	})

	t.Run("database rejects invalid records", func(t *testing.T) {
		tests := []struct {
			name  string
			index int
			value any
		}{
			{"blank address", 0, "     "},
			{"short address", 0, "1234"},
			{"latitude", 1, 91.0},
			{"longitude", 2, -181.0},
			{"NaN", 1, math.NaN()},
			{"infinite coordinate", 2, math.Inf(1)},
			{"infinite date", 3, "infinity"},
			{"BC date outside contract", 3, "0001-01-01 BC"},
			{"five digit year outside contract", 3, "10000-01-01"},
			{"equal window", 5, "09:00"},
			{"reversed window", 5, "08:00"},
			{"24 hour end", 5, "24:00"},
			{"seconds in window", 4, "09:00:01"},
			{"unknown status", 6, "UNKNOWN"},
			{"review without reason", 6, "PENDING_REVIEW"},
			{"reasons outside review", 7, []string{"ZONE_NOT_COVERED"}},
			{"zero version", 8, int64(0)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tx := testTx(t, ctx, pool)
				args := []any{"Avenida Amazonas 123", -0.18, -78.48, "2030-04-12", "09:00", "12:00", "PENDING", []string{}, int64(1)}
				args[tt.index] = tt.value
				_, err := tx.Exec(ctx, insertSQL, args...)
				assertCheckViolation(t, err)
			})
		}
	})

	t.Run("calendar boundaries preserve contract format", func(t *testing.T) {
		tx := testTx(t, ctx, pool)
		for _, date := range []string{"0001-01-01", "9999-12-31"} {
			var stored string
			err := tx.QueryRow(ctx, insertSQL+` RETURNING delivery_date::text`,
				"Avenida Amazonas 123", 0, 0, date, "00:00", "23:59", "PENDING", []string{}, 1).Scan(&stored)
			if err != nil || stored != date {
				t.Fatalf("date %s: stored=%s err=%v", date, stored, err)
			}
		}
	})

	t.Run("all published states are persistible", func(t *testing.T) {
		tx := testTx(t, ctx, pool)
		for _, state := range []string{"PENDING", "PENDING_REVIEW", "ROUTE_ASSIGNED", "PICKED_UP", "IN_TRANSIT", "DELIVERED", "NOT_DELIVERED", "CANCELLED"} {
			reasons := []string{}
			if state == "PENDING_REVIEW" {
				reasons = []string{"ZONE_NOT_COVERED"}
			}
			_, err := tx.Exec(ctx, insertSQL, "Avenida Amazonas 123", -0.18, -78.48, "2030-04-12", "09:00", "12:00", state, reasons, 1)
			if err != nil {
				t.Fatalf("state %s: %v", state, err)
			}
		}
	})

	t.Run("unknown and null review reasons fail", func(t *testing.T) {
		for _, sql := range []string{
			`UPDATE orders SET review_reasons = ARRAY['UNKNOWN'] WHERE id = $1`,
			`UPDATE orders SET review_reasons = ARRAY[NULL]::text[] WHERE id = $1`,
		} {
			tx := testTx(t, ctx, pool)
			var id string
			err := tx.QueryRow(ctx, insertSQL+` RETURNING id::text`, "Avenida Amazonas 123", 0, 0, "2030-04-12", "09:00", "12:00", "PENDING_REVIEW", []string{"ZONE_NOT_COVERED"}, 1).Scan(&id)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, sql, id)
			assertCheckViolation(t, err)
		}
	})

	t.Run("indexes support planning and listing", func(t *testing.T) {
		for _, name := range []string{"orders_delivery_date_status_idx", "orders_created_at_idx"} {
			var count int
			err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'orders' AND indexname = $1`, name).Scan(&count)
			if err != nil || count != 1 {
				t.Fatalf("index %s: count=%d err=%v", name, count, err)
			}
		}
	})

	t.Run("down and up scripts are reversible", func(t *testing.T) {
		// DDL is transactional: rollback restores all preexisting rows and tables.
		tx := testTx(t, ctx, pool)
		var customerID string
		if err := tx.QueryRow(ctx, `INSERT INTO customers (user_id, full_name, email)
			VALUES (gen_random_uuid(), 'Migration test', gen_random_uuid()::text || '@example.com')
			RETURNING id::text`).Scan(&customerID); err != nil {
			t.Fatal(err)
		}
		// Las migraciones posteriores dependen de orders: se revierten en orden inverso y se reaplican.
		for _, file := range []string{
			"000005_review_address_and_notifications.down.sql", "000004_order_customer_and_zone.down.sql",
			"000003_limit_delivery_date.down.sql", "000002_create_orders.down.sql",
			"000002_create_orders.up.sql", "000003_limit_delivery_date.up.sql",
			"000004_order_customer_and_zone.up.sql", "000005_review_address_and_notifications.up.sql",
		} {
			sql, err := migrations.FS.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				t.Fatalf("%s: %v", file, err)
			}
		}
		var customerCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM customers WHERE id = $1`, customerID).Scan(&customerCount); err != nil || customerCount != 1 {
			t.Fatalf("orders migrations must preserve customers: count=%d err=%v", customerCount, err)
		}
		if _, err := tx.Exec(ctx, insertSQL, "Avenida Amazonas 123", 0, 0, "2030-04-12", "09:00", "12:00", "PENDING", []string{}, 1); err != nil {
			t.Fatalf("insert after down/up: %v", err)
		}
	})
}

func testTx(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback test data: %v", err)
		}
	})
	return tx
}

func assertCheckViolation(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("expected PostgreSQL CHECK violation, got %v", err)
	}
}
