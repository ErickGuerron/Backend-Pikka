package config

import (
	"testing"
	"time"
)

func TestBalanceSweepIntervalDefaultsToFifteenMinutes(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	t.Setenv("BALANCE_SWEEP_MINUTES", "")

	cfg, err := Load()

	if err != nil {
		t.Fatal(err)
	}
	if cfg.BalanceSweepInterval != 15*time.Minute {
		t.Fatalf("interval = %v, want 15m", cfg.BalanceSweepInterval)
	}
}

func TestBalanceSweepIntervalComesFromTheEnvironment(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	t.Setenv("BALANCE_SWEEP_MINUTES", "5")

	cfg, err := Load()

	if err != nil {
		t.Fatal(err)
	}
	if cfg.BalanceSweepInterval != 5*time.Minute {
		t.Fatalf("interval = %v, want 5m", cfg.BalanceSweepInterval)
	}
}

func TestBalanceSweepIntervalMustBeAPositiveNumber(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	for _, bad := range []string{"0", "-3", "abc"} {
		t.Setenv("BALANCE_SWEEP_MINUTES", bad)
		if _, err := Load(); err == nil {
			t.Fatalf("BALANCE_SWEEP_MINUTES=%q must be rejected", bad)
		}
	}
}

func TestLoadRequiresThePassword(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("a missing POSTGRES_PASSWORD must fail")
	}
}
