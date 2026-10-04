package application_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

type fakeRepo struct {
	mu    sync.Mutex
	users map[string]domain.User
	seq   int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[string]domain.User{}} }

func (r *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if existing.Email == u.Email {
			return domain.User{}, domain.ErrEmailAlreadyExists
		}
	}
	r.seq++
	u.ID = fmt.Sprintf("user-%d", r.seq)
	u.Version = 1
	u.CreatedAt = time.Now().UTC()
	u.UpdatedAt = u.CreatedAt
	r.users[u.ID] = u
	return u, nil
}

func (r *fakeRepo) FindByEmail(_ context.Context, email string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (r *fakeRepo) FindByID(_ context.Context, id string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *fakeRepo) Count(context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.users), nil
}

// fakeHasher evita el costo de bcrypt en pruebas unitarias.
type fakeHasher struct{ compares int }

func (h *fakeHasher) Hash(p string) (string, error) { return "hashed:" + p, nil }
func (h *fakeHasher) Compare(hash, p string) bool {
	h.compares++
	return strings.TrimPrefix(hash, "hashed:") == p
}

type fakeTokens struct{}

func (fakeTokens) Issue(userID string, role domain.Role) (string, time.Duration, error) {
	return "token:" + userID + ":" + string(role), time.Hour, nil
}

func (fakeTokens) Verify(token string) (ports.Claims, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "token" {
		return ports.Claims{}, domain.ErrInvalidToken
	}
	return ports.Claims{UserID: parts[1], Role: domain.Role(parts[2])}, nil
}
