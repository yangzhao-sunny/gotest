//go:build integration

package auth

import (
	"context"
	"testing"

	"github.com/test/taskmgr/internal/testhelper"
)

func TestRegisterAndLogin_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	svc := NewService(NewRepo(pool), "test-secret", 60, 30)

	u, err := svc.Register(ctx, "alice@example.com", "password123", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email: got %q", u.Email)
	}

	// duplicate email
	_, err = svc.Register(ctx, "alice@example.com", "password123", "Alice2")
	if err == nil {
		t.Fatal("expected duplicate email error")
	}

	pair, err := svc.Login(ctx, "alice@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("empty token(s)")
	}

	// wrong password
	_, err = svc.Login(ctx, "alice@example.com", "wrongpass")
	if err == nil {
		t.Fatal("expected wrong password error")
	}
}

func TestRefreshAndLogout_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	svc := NewService(NewRepo(pool), "test-secret", 60, 30)

	_, err := svc.Register(ctx, "bob@example.com", "password123", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := svc.Login(ctx, "bob@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}

	access, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if access == "" {
		t.Fatal("empty refreshed access token")
	}

	// logout revokes
	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatal(err)
	}

	// refresh after logout should fail
	_, err = svc.Refresh(ctx, pair.RefreshToken)
	if err == nil {
		t.Fatal("expected error refreshing with revoked token")
	}
}
