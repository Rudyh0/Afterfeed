// Package auth handles the account, sessions, two-factor authentication and
// the vault PIN.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rudyh0/afterfeed/internal/store"
)

// Errors returned to the API, which maps them to error codes.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidCode        = errors.New("invalid code")
	ErrSetupDone          = errors.New("setup already done")
	ErrWeakPassword       = errors.New("password too short")
	ErrInvalidUsername    = errors.New("invalid username")
	ErrNoSession          = errors.New("no session")
	ErrInvalidPIN         = errors.New("invalid pin")
	ErrPINNotSet          = errors.New("pin not set")
)

// RateLimitedError means too many recent failures for a key.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return fmt.Sprintf("rate limited for %s", e.RetryAfter) }

const (
	// SessionLifetime is how long a session lasts without being used.
	SessionLifetime = 30 * 24 * time.Hour
	// mfaPendingLifetime is how long a session waits for the second factor.
	mfaPendingLifetime = 10 * time.Minute
	// CookieName is the session cookie.
	CookieName = "af_session"
)

// User is the account.
type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	MustChangePassword bool
	TOTPSecret         string
	TOTPEnabled        bool
	TOTPLastStep       int64
	VaultPINHash       string
	Locale             string
}

// Session is a signed-in browser.
type Session struct {
	TokenHash     string
	UserID        int64
	MFAPending    bool
	VaultUnlocked time.Time
	LastSeen      time.Time
	ExpiresAt     time.Time
}

// Service implements the account operations.
type Service struct {
	st  *store.Store
	now func() time.Time
}

// New creates the service.
func New(st *store.Store) *Service {
	return &Service{st: st, now: time.Now}
}

// UserCount returns the number of accounts.
func (s *Service) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.st.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// FirstUser returns the account. Afterfeed has a single account.
func (s *Service) FirstUser(ctx context.Context) (*User, error) {
	return s.scanUser(s.st.R.QueryRowContext(ctx, userSelect+` ORDER BY id LIMIT 1`))
}

// UserByID returns an account.
func (s *Service) UserByID(ctx context.Context, id int64) (*User, error) {
	return s.scanUser(s.st.R.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
}

const userSelect = `SELECT id, username, password_hash, must_change_password,
	COALESCE(totp_secret, ''), totp_enabled, totp_last_step, COALESCE(vault_pin_hash, ''), locale FROM users`

func (s *Service) scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.MustChangePassword,
		&u.TOTPSecret, &u.TOTPEnabled, &u.TOTPLastStep, &u.VaultPINHash, &u.Locale)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ValidateUsername checks a username.
func ValidateUsername(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return ErrInvalidUsername
	}
	return nil
}

// ValidatePassword checks the strength rules for a new password.
func ValidatePassword(p string) error {
	if utf8.RuneCountInString(p) < MinPasswordLength {
		return ErrWeakPassword
	}
	return nil
}

// Setup creates the account on a new installation.
func (s *Service) Setup(ctx context.Context, username, password, locale string) (*User, error) {
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	tx, err := s.st.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrSetupDone
	}
	now := store.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO users(username, password_hash, locale, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, strings.TrimSpace(username), hash, locale, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.UserByID(ctx, id)
}

// Authenticate checks a username and password.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error) {
	u, err := s.scanUser(s.st.R.QueryRowContext(ctx, userSelect+` WHERE username = ?`, strings.TrimSpace(username)))
	if errors.Is(err, sql.ErrNoRows) {
		// Spend the same time as a real check so usernames cannot be probed.
		VerifyPassword(password, dummyHash())
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !VerifyPassword(password, u.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// dummyHash is verified against when the username does not exist.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("afterfeed-dummy-password")
	return h
})

// ChangePassword sets a new password after checking the current one, and ends
// every other session.
func (s *Service) ChangePassword(ctx context.Context, u *User, current, next, keepTokenHash string) error {
	if !u.MustChangePassword && !VerifyPassword(current, u.PasswordHash) {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`,
		hash, store.Now(), u.ID); err != nil {
		return err
	}
	_, err = s.st.W.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, u.ID, keepTokenHash)
	return err
}

// ResetPassword sets a random temporary password, which must be changed at the
// next sign-in, and ends every session. Used by the reset-password command.
func (s *Service) ResetPassword(ctx context.Context, u *User) (string, error) {
	pw := RandomPassword()
	hash, err := HashPassword(pw)
	if err != nil {
		return "", err
	}
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 1, updated_at = ? WHERE id = ?`,
		hash, store.Now(), u.ID); err != nil {
		return "", err
	}
	if _, err := s.st.W.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, u.ID); err != nil {
		return "", err
	}
	s.ResetAttempts(ctx, "login:")
	return pw, nil
}

// SetLocale stores the account's language.
func (s *Service) SetLocale(ctx context.Context, u *User, locale string) error {
	_, err := s.st.W.ExecContext(ctx, `UPDATE users SET locale = ?, updated_at = ? WHERE id = ?`, locale, store.Now(), u.ID)
	return err
}

// --- sessions ---

func newToken() (token, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken returns the stored form of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession starts a session. A session waiting for the second factor
// is short-lived and gives access to nothing but the code check.
func (s *Service) CreateSession(ctx context.Context, userID int64, mfaPending bool, userAgent string) (string, *Session, error) {
	token, hash := newToken()
	now := s.now().UTC()
	lifetime := SessionLifetime
	if mfaPending {
		lifetime = mfaPendingLifetime
	}
	sess := &Session{TokenHash: hash, UserID: userID, MFAPending: mfaPending, LastSeen: now, ExpiresAt: now.Add(lifetime)}
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	_, err := s.st.W.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, mfa_pending, created_at, last_seen_at, expires_at, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, hash, userID, mfaPending, store.FormatTime(now), store.FormatTime(now), store.FormatTime(sess.ExpiresAt), userAgent)
	if err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

// Session looks up a session by its token and extends it when it is in use.
func (s *Service) Session(ctx context.Context, token string) (*Session, *User, error) {
	if token == "" {
		return nil, nil, ErrNoSession
	}
	hash := HashToken(token)
	var sess Session
	var vault sql.NullString
	var lastSeen, expires string
	err := s.st.R.QueryRowContext(ctx, `SELECT token_hash, user_id, mfa_pending, vault_unlocked, last_seen_at, expires_at
		FROM sessions WHERE token_hash = ?`, hash).Scan(&sess.TokenHash, &sess.UserID, &sess.MFAPending, &vault, &lastSeen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNoSession
	}
	if err != nil {
		return nil, nil, err
	}
	sess.LastSeen, _ = store.ParseTime(lastSeen)
	sess.ExpiresAt, _ = store.ParseTime(expires)
	if vault.Valid {
		sess.VaultUnlocked, _ = store.ParseTime(vault.String)
	}
	now := s.now().UTC()
	if now.After(sess.ExpiresAt) {
		s.st.W.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash)
		return nil, nil, ErrNoSession
	}
	// Extend at most once an hour to avoid a write on every request.
	if !sess.MFAPending && now.Sub(sess.LastSeen) > time.Hour {
		sess.LastSeen = now
		sess.ExpiresAt = now.Add(SessionLifetime)
		s.st.W.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
			store.FormatTime(now), store.FormatTime(sess.ExpiresAt), hash)
	}
	u, err := s.UserByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, ErrNoSession
	}
	return &sess, u, nil
}

// CompleteMFA turns a session waiting for the second factor into a full session.
func (s *Service) CompleteMFA(ctx context.Context, sess *Session) error {
	now := s.now().UTC()
	_, err := s.st.W.ExecContext(ctx, `UPDATE sessions SET mfa_pending = 0, last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
		store.FormatTime(now), store.FormatTime(now.Add(SessionLifetime)), sess.TokenHash)
	sess.MFAPending = false
	return err
}

// DeleteSession ends a session.
func (s *Service) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.st.W.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// CleanupSessions removes expired sessions and old failed-attempt records.
func (s *Service) CleanupSessions(ctx context.Context) error {
	now := store.FormatTime(s.now())
	if _, err := s.st.W.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now); err != nil {
		return err
	}
	_, err := s.st.W.ExecContext(ctx, `DELETE FROM attempts WHERE last_failure_at < ?`,
		store.FormatTime(s.now().Add(-24*time.Hour)))
	return err
}

// --- two-factor authentication ---

// VerifySecondFactor checks a TOTP code or a recovery code.
func (s *Service) VerifySecondFactor(ctx context.Context, u *User, code string) error {
	code = strings.TrimSpace(code)
	if step, ok := VerifyTOTP(u.TOTPSecret, code, s.now()); ok {
		// A code can only be used once.
		res, err := s.st.W.ExecContext(ctx, `UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, u.ID, step)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrInvalidCode
		}
		return nil
	}
	res, err := s.st.W.ExecContext(ctx, `UPDATE recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		store.Now(), u.ID, hashRecoveryCode(code))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	return ErrInvalidCode
}

// BeginTOTP generates a new secret for the account. Two-factor authentication
// is only switched on after EnableTOTP confirms a code from the app.
func (s *Service) BeginTOTP(ctx context.Context, u *User) (secret, uri string, err error) {
	if u.TOTPEnabled {
		return "", "", errors.New("already enabled")
	}
	secret = NewTOTPSecret()
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET totp_secret = ?, updated_at = ? WHERE id = ?`, secret, store.Now(), u.ID); err != nil {
		return "", "", err
	}
	return secret, TOTPURI(secret, u.Username), nil
}

// EnableTOTP switches on two-factor authentication after checking a code from
// the app, and returns new recovery codes.
func (s *Service) EnableTOTP(ctx context.Context, u *User, code string) ([]string, error) {
	if u.TOTPSecret == "" {
		return nil, errors.New("not started")
	}
	step, ok := VerifyTOTP(u.TOTPSecret, code, s.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET totp_enabled = 1, totp_last_step = ?, updated_at = ? WHERE id = ?`,
		step, store.Now(), u.ID); err != nil {
		return nil, err
	}
	return s.RegenerateRecoveryCodes(ctx, u)
}

// RegenerateRecoveryCodes replaces the recovery codes.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, u *User) ([]string, error) {
	codes := NewRecoveryCodes(10)
	tx, err := s.st.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, u.ID); err != nil {
		return nil, err
	}
	for _, c := range codes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO recovery_codes(user_id, code_hash) VALUES (?, ?)`, u.ID, hashRecoveryCode(c)); err != nil {
			return nil, err
		}
	}
	return codes, tx.Commit()
}

// DisableTOTP switches off two-factor authentication. It is also used by the
// reset-2fa command.
func (s *Service) DisableTOTP(ctx context.Context, u *User) error {
	if _, err := s.st.W.ExecContext(ctx, `UPDATE users SET totp_enabled = 0, totp_secret = NULL, totp_last_step = 0, updated_at = ? WHERE id = ?`,
		store.Now(), u.ID); err != nil {
		return err
	}
	_, err := s.st.W.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, u.ID)
	return err
}

// RecoveryCodesLeft returns the number of unused recovery codes.
func (s *Service) RecoveryCodesLeft(ctx context.Context, u *User) (int, error) {
	var n int
	err := s.st.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used_at IS NULL`, u.ID).Scan(&n)
	return n, err
}
