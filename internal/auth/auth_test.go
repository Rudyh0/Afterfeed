package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery", h) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong horse battery", h) {
		t.Fatal("wrong password accepted")
	}
}

func TestTOTPKnownVector(t *testing.T) {
	// RFC 6238 test secret "12345678901234567890" at 59 seconds gives 287082
	// for 6 digits with SHA-1.
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	code, err := TOTPCode(secret, time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("got %s, want 287082", code)
	}
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(59+30, 0)); !ok {
		t.Fatal("code from the previous step should be accepted")
	}
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(59+120, 0)); ok {
		t.Fatal("old code should be rejected")
	}
}

func TestSetupLoginAndSecondFactor(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)

	if _, err := s.Setup(ctx, "rudy", "short", "en"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("want weak password error, got %v", err)
	}
	u, err := s.Setup(ctx, "rudy", "a long enough password", "nl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Setup(ctx, "other", "a long enough password", "en"); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("second setup should fail, got %v", err)
	}
	if _, err := s.Authenticate(ctx, "RUDY", "a long enough password"); err != nil {
		t.Fatalf("username should be case-insensitive: %v", err)
	}
	if _, err := s.Authenticate(ctx, "rudy", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want invalid credentials, got %v", err)
	}

	secret, _, err := s.BeginTOTP(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = s.UserByID(ctx, u.ID)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	code, _ := TOTPCode(secret, now)
	codes, err := s.EnableTOTP(ctx, u, code)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 {
		t.Fatalf("want 10 recovery codes, got %d", len(codes))
	}

	u, _ = s.UserByID(ctx, u.ID)
	// The code used to enable two-factor authentication cannot be used again.
	if err := s.VerifySecondFactor(ctx, u, code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("reused code should fail, got %v", err)
	}
	now = now.Add(30 * time.Second)
	next, _ := TOTPCode(secret, now)
	if err := s.VerifySecondFactor(ctx, u, next); err != nil {
		t.Fatalf("next code should work: %v", err)
	}
	if err := s.VerifySecondFactor(ctx, u, codes[0]); err != nil {
		t.Fatalf("recovery code should work: %v", err)
	}
	if err := s.VerifySecondFactor(ctx, u, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Fatal("recovery code should only work once")
	}

	if err := s.DisableTOTP(ctx, u); err != nil {
		t.Fatal(err)
	}
	u, _ = s.UserByID(ctx, u.ID)
	if u.TOTPEnabled || u.TOTPSecret != "" {
		t.Fatal("two-factor authentication should be off")
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	u, err := s.Setup(ctx, "rudy", "a long enough password", "en")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.CreateSession(ctx, u.ID, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	sess, got, err := s.Session(ctx, token)
	if err != nil || got.ID != u.ID || sess.MFAPending {
		t.Fatalf("session lookup failed: %v", err)
	}
	if _, _, err := s.Session(ctx, "nope"); !errors.Is(err, ErrNoSession) {
		t.Fatal("unknown token should not resolve")
	}

	other, _, _ := s.CreateSession(ctx, u.ID, false, "other")
	if err := s.ChangePassword(ctx, u, "a long enough password", "another long password", sess.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Session(ctx, other); !errors.Is(err, ErrNoSession) {
		t.Fatal("other sessions should end after a password change")
	}
	if _, _, err := s.Session(ctx, token); err != nil {
		t.Fatal("current session should stay")
	}

	s.now = func() time.Time { return time.Now().Add(SessionLifetime + time.Hour) }
	if _, _, err := s.Session(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatal("expired session should not resolve")
	}
}

func TestRateLimit(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	key := "login:1.2.3.4"
	for i := 0; i < freeAttempts-1; i++ {
		if err := s.FailAttempt(ctx, key); err != nil {
			t.Fatalf("attempt %d should not lock: %v", i+1, err)
		}
	}
	var rl *RateLimitedError
	if err := s.FailAttempt(ctx, key); !errors.As(err, &rl) || rl.RetryAfter != firstLock {
		t.Fatalf("fifth failure should lock for %s, got %v", firstLock, err)
	}
	if err := s.CheckAttempts(ctx, key); !errors.As(err, &rl) {
		t.Fatal("key should be locked")
	}
	s.ResetAttempts(ctx, "login:")
	if err := s.CheckAttempts(ctx, key); err != nil {
		t.Fatalf("key should be unlocked after reset: %v", err)
	}
}

func TestVault(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	u, _ := s.Setup(ctx, "rudy", "a long enough password", "en")
	_, sess, _ := s.CreateSession(ctx, u.ID, false, "")

	if err := s.SetVaultPIN(ctx, u, "12345"); !errors.Is(err, ErrInvalidPIN) {
		t.Fatal("a five-digit PIN should be rejected")
	}
	if err := s.SetVaultPIN(ctx, u, "123456"); err != nil {
		t.Fatal(err)
	}
	u, _ = s.UserByID(ctx, u.ID)
	if err := s.UnlockVault(ctx, u, sess, "000000"); !errors.Is(err, ErrInvalidPIN) {
		t.Fatalf("wrong PIN should fail, got %v", err)
	}
	if err := s.UnlockVault(ctx, u, sess, "123456"); err != nil {
		t.Fatal(err)
	}
	if !s.VaultOpen(sess, DefaultVaultAutolock) {
		t.Fatal("vault should be open")
	}
	s.now = func() time.Time { return time.Now().Add(DefaultVaultAutolock + time.Second) }
	if s.VaultOpen(sess, DefaultVaultAutolock) {
		t.Fatal("vault should lock automatically")
	}
}
