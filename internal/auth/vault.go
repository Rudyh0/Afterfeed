package auth

import (
	"context"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

// DefaultVaultAutolock is how long the vault stays open without activity.
const DefaultVaultAutolock = 5 * time.Minute

// SetVaultPIN sets the vault PIN. It can only be set when none is set; use
// ChangeVaultPIN or ResetVaultPIN otherwise.
func (s *Service) SetVaultPIN(ctx context.Context, u *User, pin string) error {
	if !ValidPIN(pin) {
		return ErrInvalidPIN
	}
	if u.VaultPINHash != "" {
		return ErrInvalidPIN
	}
	return s.storePIN(ctx, u, pin)
}

// ChangeVaultPIN replaces the PIN after checking the current one.
func (s *Service) ChangeVaultPIN(ctx context.Context, u *User, current, next string) error {
	if u.VaultPINHash == "" || !VerifyPIN(current, u.VaultPINHash) {
		return ErrInvalidPIN
	}
	if !ValidPIN(next) {
		return ErrInvalidPIN
	}
	return s.storePIN(ctx, u, next)
}

func (s *Service) storePIN(ctx context.Context, u *User, pin string) error {
	hash, err := HashPIN(pin)
	if err != nil {
		return err
	}
	_, err = s.st.W.ExecContext(ctx, `UPDATE users SET vault_pin_hash = ?, updated_at = ? WHERE id = ?`, hash, store.Now(), u.ID)
	return err
}

// ResetVaultPIN removes the PIN. The vault keeps its contents.
func (s *Service) ResetVaultPIN(ctx context.Context, u *User) error {
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET vault_pin_hash = NULL, updated_at = ? WHERE id = ?`, store.Now(), u.ID); err != nil {
		return err
	}
	if _, err := s.st.W.ExecContext(ctx, `UPDATE sessions SET vault_unlocked = NULL WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	s.ResetAttempts(ctx, vaultKey(u))
	return nil
}

func vaultKey(u *User) string { return "vault:" + u.Username }

// UnlockVault checks the PIN and opens the vault for this session.
func (s *Service) UnlockVault(ctx context.Context, u *User, sess *Session, pin string) error {
	if u.VaultPINHash == "" {
		return ErrPINNotSet
	}
	key := vaultKey(u)
	if err := s.CheckAttempts(ctx, key); err != nil {
		return err
	}
	if !VerifyPIN(pin, u.VaultPINHash) {
		if err := s.FailAttempt(ctx, key); err != nil {
			return err
		}
		return ErrInvalidPIN
	}
	s.ResetAttempts(ctx, key)
	return s.TouchVault(ctx, sess)
}

// TouchVault records vault activity, which keeps it open.
func (s *Service) TouchVault(ctx context.Context, sess *Session) error {
	now := s.now().UTC()
	sess.VaultUnlocked = now
	_, err := s.st.W.ExecContext(ctx, `UPDATE sessions SET vault_unlocked = ? WHERE token_hash = ?`, store.FormatTime(now), sess.TokenHash)
	return err
}

// LockVault closes the vault for this session.
func (s *Service) LockVault(ctx context.Context, sess *Session) error {
	sess.VaultUnlocked = time.Time{}
	_, err := s.st.W.ExecContext(ctx, `UPDATE sessions SET vault_unlocked = NULL WHERE token_hash = ?`, sess.TokenHash)
	return err
}

// VaultOpen reports whether the vault is open for this session.
func (s *Service) VaultOpen(sess *Session, autolock time.Duration) bool {
	if sess.VaultUnlocked.IsZero() {
		return false
	}
	return s.now().Sub(sess.VaultUnlocked) < autolock
}
