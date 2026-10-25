package server

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/version"
)

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
)

func userFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userKey).(*auth.User)
	return u
}

func sessionFrom(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(sessionKey).(*auth.Session)
	return s
}

func (s *Server) authRoutes() {
	m := s.mux
	m.HandleFunc("POST /api/setup", s.handle(s.setup))
	m.HandleFunc("POST /api/auth/login", s.handle(s.login))
	m.HandleFunc("POST /api/auth/mfa", s.handle(s.mfa))
	m.HandleFunc("POST /api/auth/logout", s.handle(s.logout))

	// Allowed even when the password must be changed first.
	m.HandleFunc("POST /api/account/password", s.authed(s.changePassword, true))

	m.HandleFunc("PATCH /api/account", s.authed(s.updateAccount, false))
	m.HandleFunc("POST /api/account/2fa/begin", s.authed(s.beginTOTP, false))
	m.HandleFunc("POST /api/account/2fa/enable", s.authed(s.enableTOTP, false))
	m.HandleFunc("POST /api/account/2fa/disable", s.authed(s.disableTOTP, false))
	m.HandleFunc("POST /api/account/2fa/recovery-codes", s.authed(s.newRecoveryCodes, false))
	m.HandleFunc("GET /api/account/security", s.authed(s.securityStatus, false))
	m.HandleFunc("POST /api/take-in/done", s.authed(s.takeInDone, false))
}

// authed wraps a handler that needs a signed-in user. When allowPasswordChange
// is false, accounts that must change their password are refused.
func (s *Server) authed(h handlerFunc, allowPasswordChange bool) http.HandlerFunc {
	return s.handle(func(w http.ResponseWriter, r *http.Request) error {
		sess, u, err := s.currentSession(r)
		if err != nil || sess.MFAPending {
			return errUnauthorized()
		}
		if u.MustChangePassword && !allowPasswordChange {
			return errForbidden("auth.must_change_password")
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		ctx = context.WithValue(ctx, sessionKey, sess)
		return h(w, r.WithContext(ctx))
	})
}

func (s *Server) currentSession(r *http.Request) (*auth.Session, *auth.User, error) {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return nil, nil, auth.ErrNoSession
	}
	return s.deps.Auth.Session(r.Context(), c.Value)
}

func (s *Server) secureRequest(r *http.Request) bool {
	switch s.cfg.SecureCookies {
	case "true":
		return true
	case "false":
		return false
	}
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func rateLimited(err error) *apiError {
	var rl *auth.RateLimitedError
	if errors.As(err, &rl) {
		return &apiError{
			Status:     http.StatusTooManyRequests,
			Code:       "auth.too_many_attempts",
			Message:    "too many attempts",
			RetryAfter: int(math.Ceil(rl.RetryAfter.Seconds())),
		}
	}
	return nil
}

type userJSON struct {
	Username           string `json:"username"`
	Locale             string `json:"locale"`
	TOTPEnabled        bool   `json:"totp_enabled"`
	MustChangePassword bool   `json:"must_change_password"`
	VaultPINSet        bool   `json:"vault_pin_set"`
}

func toUserJSON(u *auth.User) userJSON {
	return userJSON{
		Username:           u.Username,
		Locale:             u.Locale,
		TOTPEnabled:        u.TOTPEnabled,
		MustChangePassword: u.MustChangePassword,
		VaultPINSet:        u.VaultPINHash != "",
	}
}

func (s *Server) takeInIsDone(ctx context.Context) bool {
	var done bool
	ok, err := s.store.GetSetting(ctx, "take_in_done", &done)
	return err == nil && ok && done
}

func (s *Server) stateWithAuth(w http.ResponseWriter, r *http.Request) error {
	n, err := s.deps.Auth.UserCount(r.Context())
	if err != nil {
		return err
	}
	out := map[string]any{
		"version":        version.Version,
		"setup_required": n == 0,
		"authenticated":  false,
	}
	if sess, u, err := s.currentSession(r); err == nil {
		if sess.MFAPending {
			out["mfa_required"] = true
		} else {
			out["authenticated"] = true
			out["user"] = toUserJSON(u)
			out["take_in_done"] = s.takeInIsDone(r.Context())
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func validLocale(l string) bool {
	switch l {
	case "en", "nl", "fr", "de", "es":
		return true
	}
	return false
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Locale   string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if !validLocale(req.Locale) {
		req.Locale = "en"
	}
	u, err := s.deps.Auth.Setup(r.Context(), req.Username, req.Password, req.Locale)
	switch {
	case errors.Is(err, auth.ErrSetupDone):
		return errForbidden("setup.done")
	case errors.Is(err, auth.ErrWeakPassword):
		return errBadRequest("auth.password_too_short", "password too short")
	case errors.Is(err, auth.ErrInvalidUsername):
		return errBadRequest("auth.invalid_username", "invalid username")
	case err != nil:
		return err
	}
	token, sess, err := s.deps.Auth.CreateSession(r.Context(), u.ID, false, r.UserAgent())
	if err != nil {
		return err
	}
	s.setSessionCookie(w, r, token, sess.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	keys := []string{"login:ip:" + remoteIP(r), "login:user:" + strings.ToLower(strings.TrimSpace(req.Username))}
	for _, k := range keys {
		if err := s.deps.Auth.CheckAttempts(ctx, k); err != nil {
			if ae := rateLimited(err); ae != nil {
				return ae
			}
			return err
		}
	}
	u, err := s.deps.Auth.Authenticate(ctx, req.Username, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		var limited error
		for _, k := range keys {
			if err := s.deps.Auth.FailAttempt(ctx, k); err != nil && limited == nil {
				limited = err
			}
		}
		if ae := rateLimited(limited); ae != nil {
			return ae
		}
		return &apiError{Status: http.StatusUnauthorized, Code: "auth.invalid_credentials", Message: "invalid credentials"}
	}
	if err != nil {
		return err
	}
	for _, k := range keys {
		s.deps.Auth.ResetAttempts(ctx, k)
	}
	token, sess, err := s.deps.Auth.CreateSession(ctx, u.ID, u.TOTPEnabled, r.UserAgent())
	if err != nil {
		return err
	}
	s.setSessionCookie(w, r, token, sess.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"mfa_required": u.TOTPEnabled})
	return nil
}

func (s *Server) mfa(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	sess, u, err := s.currentSession(r)
	if err != nil || !sess.MFAPending {
		return errUnauthorized()
	}
	ctx := r.Context()
	key := "mfa:" + u.Username
	if err := s.deps.Auth.CheckAttempts(ctx, key); err != nil {
		if ae := rateLimited(err); ae != nil {
			return ae
		}
		return err
	}
	if err := s.deps.Auth.VerifySecondFactor(ctx, u, req.Code); err != nil {
		if errors.Is(err, auth.ErrInvalidCode) {
			if ae := rateLimited(s.deps.Auth.FailAttempt(ctx, key)); ae != nil {
				return ae
			}
			return errBadRequest("auth.invalid_code", "invalid code")
		}
		return err
	}
	s.deps.Auth.ResetAttempts(ctx, key)
	if err := s.deps.Auth.CompleteMFA(ctx, sess); err != nil {
		return err
	}
	c, _ := r.Cookie(auth.CookieName)
	s.setSessionCookie(w, r, c.Value, time.Now().Add(auth.SessionLifetime))
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if sess, _, err := s.currentSession(r); err == nil {
		if err := s.deps.Auth.DeleteSession(r.Context(), sess.TokenHash); err != nil {
			return err
		}
	}
	s.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	u, sess := userFrom(r.Context()), sessionFrom(r.Context())
	err := s.deps.Auth.ChangePassword(r.Context(), u, req.Current, req.New, sess.TokenHash)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return errBadRequest("auth.invalid_credentials", "current password is wrong")
	case errors.Is(err, auth.ErrWeakPassword):
		return errBadRequest("auth.password_too_short", "password too short")
	case err != nil:
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Locale string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if !validLocale(req.Locale) {
		return errBadRequest("invalid_locale", "unknown locale")
	}
	if err := s.deps.Auth.SetLocale(r.Context(), userFrom(r.Context()), req.Locale); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) securityStatus(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	left := 0
	if u.TOTPEnabled {
		var err error
		if left, err = s.deps.Auth.RecoveryCodesLeft(r.Context(), u); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totp_enabled":          u.TOTPEnabled,
		"recovery_codes_left":   left,
		"reset_2fa_command":     "docker exec -it afterfeed afterfeed auth reset-2fa",
		"reset_password_cmd":    "docker exec -it afterfeed afterfeed auth reset-password",
		"session_lifetime_days": int(auth.SessionLifetime.Hours() / 24),
	})
	return nil
}

func (s *Server) beginTOTP(w http.ResponseWriter, r *http.Request) error {
	secret, uri, err := s.deps.Auth.BeginTOTP(r.Context(), userFrom(r.Context()))
	if err != nil {
		return errBadRequest("auth.totp_enabled", err.Error())
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
	return nil
}

func (s *Server) enableTOTP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	codes, err := s.deps.Auth.EnableTOTP(r.Context(), userFrom(r.Context()), req.Code)
	if errors.Is(err, auth.ErrInvalidCode) {
		return errBadRequest("auth.invalid_code", "invalid code")
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
	return nil
}

func (s *Server) disableTOTP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	u := userFrom(r.Context())
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		return errBadRequest("auth.invalid_credentials", "password is wrong")
	}
	if err := s.deps.Auth.DisableTOTP(r.Context(), u); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) newRecoveryCodes(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	u := userFrom(r.Context())
	if !u.TOTPEnabled {
		return errBadRequest("auth.totp_disabled", "two-factor authentication is off")
	}
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		return errBadRequest("auth.invalid_credentials", "password is wrong")
	}
	codes, err := s.deps.Auth.RegenerateRecoveryCodes(r.Context(), u)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
	return nil
}

func (s *Server) takeInDone(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.SetSetting(r.Context(), "take_in_done", true); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
