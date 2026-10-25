//go:build integration

package persistence_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/postgres"
)

// Plan: concurrent sign-ups with the same email end with one user and the domain duplicate error for the rest.
func TestPlan_ConcurrentCreateSameEmail_OnlyOneWins(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))
	pool, err := postgres.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	repo := persistence.NewUserRepository(pool)
	email := "race-" + uuid.NewString()[:8] + "@example.com"
	const workers = 16

	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.Create(ctx, domain.User{Email: email, FullName: "Race", PasswordHash: "h", Role: domain.RoleCustomer, Active: true})
		}()
	}
	close(start)
	wg.Wait()

	var ok, dup int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case assert.ErrorIs(t, err, domain.ErrEmailAlreadyExists):
			dup++
		}
	}
	assert.Equal(t, 1, ok, "exactly one creation must succeed")
	assert.Equal(t, workers-1, dup)

	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&rows))
	assert.Equal(t, 1, rows)
}
