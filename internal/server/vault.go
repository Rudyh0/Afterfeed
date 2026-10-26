package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/rudyh0/afterfeed/internal/auth"
)

func (s *Server) vaultRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/vault", s.authed(s.vaultStatus, false))
	m.HandleFunc("POST /api/vault/pin", s.authed(s.vaultSetPIN, false))
	m.HandleFunc("POST /api/vault/pin/change", s.authed(s.vaultChangePIN, false))
	m.HandleFunc("POST /api/vault/unlock", s.authed(s.vaultUnlock, false))
	m.HandleFunc("POST /api/vault/lock", s.authed(s.vaultLock, false))
	m.HandleFunc("POST /api/vault/reset", s.authed(s.vaultReset, false))
	m.HandleFunc("GET /api/vault/memories", s.authed(s.vaultMemories, false))
}

func (s *Server) vaultStatus(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	open := s.vaultOpen(r)
	if open {
		// Looking at the vault counts as activity.
		s.deps.Auth.TouchVault(r.Context(), sessionFrom(r.Context()))
	}
	resp := map[string]any{
		"pin_set":          u.VaultPINHash != "",
		"open":             open,
		"autolock_minutes": int(s.vaultAutolock(r.Context()).Minutes()),
	}
	if open {
		var memories, conversations int
		s.store.R.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM memories WHERE vault = 1`).Scan(&memories)
		s.store.R.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM conversations WHERE vault = 1`).Scan(&conversations)
		resp["memories"] = memories
		resp["conversations"] = conversations
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func vaultError(err error) error {
	if rl := rateLimited(err); rl != nil {
		return rl
	}
	switch {
	case errors.Is(err, auth.ErrInvalidPIN):
		return errBadRequest("vault.wrong_pin", "wrong PIN")
	case errors.Is(err, auth.ErrPINNotSet):
		return errBadRequest("vault.no_pin", "no PIN set")
	}
	return err
}

// vaultSetPIN sets the first PIN and opens the vault.
func (s *Server) vaultSetPIN(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	u := userFrom(ctx)
	if !auth.ValidPIN(req.PIN) {
		return errBadRequest("vault.invalid_pin", "use six digits")
	}
	if err := s.deps.Auth.SetVaultPIN(ctx, u, req.PIN); err != nil {
		return vaultError(err)
	}
	if err := s.deps.Auth.TouchVault(ctx, sessionFrom(ctx)); err != nil {
		return err
	}
	u.VaultPINHash = "set" // for the response; the hash itself is stored
	return s.vaultStatus(w, r)
}

// vaultChangePIN replaces the PIN after checking the current one.
func (s *Server) vaultChangePIN(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Current string `json:"current"`
		PIN     string `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	u := userFrom(ctx)
	if !auth.ValidPIN(req.PIN) {
		return errBadRequest("vault.invalid_pin", "use six digits")
	}
	key := "vault:" + u.Username
	if err := s.deps.Auth.CheckAttempts(ctx, key); err != nil {
		return vaultError(err)
	}
	if err := s.deps.Auth.ChangeVaultPIN(ctx, u, req.Current, req.PIN); err != nil {
		if errors.Is(err, auth.ErrInvalidPIN) {
			s.deps.Auth.FailAttempt(ctx, key)
		}
		return vaultError(err)
	}
	s.deps.Auth.ResetAttempts(ctx, key)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) vaultUnlock(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	if err := s.deps.Auth.UnlockVault(ctx, userFrom(ctx), sessionFrom(ctx), req.PIN); err != nil {
		return vaultError(err)
	}
	return s.vaultStatus(w, r)
}

func (s *Server) vaultLock(w http.ResponseWriter, r *http.Request) error {
	if err := s.deps.Auth.LockVault(r.Context(), sessionFrom(r.Context())); err != nil {
		return err
	}
	return s.vaultStatus(w, r)
}

// vaultReset removes a forgotten PIN after checking the account password.
func (s *Server) vaultReset(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	u := userFrom(ctx)
	key := "vault-reset:" + u.Username
	if err := s.deps.Auth.CheckAttempts(ctx, key); err != nil {
		return vaultError(err)
	}
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		s.deps.Auth.FailAttempt(ctx, key)
		return errBadRequest("auth.invalid_credentials", "password is wrong")
	}
	s.deps.Auth.ResetAttempts(ctx, key)
	if err := s.deps.Auth.ResetVaultPIN(ctx, u); err != nil {
		return err
	}
	if err := s.deps.Auth.LockVault(ctx, sessionFrom(ctx)); err != nil {
		return err
	}
	u.VaultPINHash = ""
	return s.vaultStatus(w, r)
}

// vaultMemories lists the memories in the vault by year, newest first:
// the first few of every year, or all of one year with ?year=.
func (s *Server) vaultMemories(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.vaultOpen(r) {
		return errForbidden("vault.locked")
	}
	s.deps.Auth.TouchVault(ctx, sessionFrom(ctx))
	const inVault = `m.vault = 1 AND m.duplicate_of IS NULL`
	if y := r.URL.Query().Get("year"); y != "" {
		year, err := strconv.Atoi(y)
		if err != nil {
			return errBadRequest("invalid_year", "invalid year")
		}
		list, err := s.cards(ctx, 1, `SELECT `+cardColumns+` FROM memories m WHERE `+inVault+` AND m.year = ?
			ORDER BY m.taken_at DESC LIMIT 2000`, year)
		if err != nil {
			return err
		}
		if list == nil {
			list = []*card{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"memories": list})
		return nil
	}
	type yearJSON struct {
		Year     int     `json:"year"`
		Count    int     `json:"count"`
		Memories []*card `json:"memories"`
	}
	years := []*yearJSON{}
	byYear := map[int]*yearJSON{}
	rows, err := s.store.R.QueryContext(ctx, `SELECT m.year, COUNT(*) FROM memories m WHERE `+inVault+`
		GROUP BY m.year ORDER BY m.year DESC`)
	if err != nil {
		return err
	}
	for rows.Next() {
		y := &yearJSON{Memories: []*card{}}
		if err := rows.Scan(&y.Year, &y.Count); err != nil {
			rows.Close()
			return err
		}
		years = append(years, y)
		byYear[y.Year] = y
	}
	rows.Close()
	list, err := s.cards(ctx, 1, `SELECT `+cardColumns+` FROM (
			SELECT m.*, ROW_NUMBER() OVER (PARTITION BY m.year ORDER BY m.taken_at DESC) AS n
			FROM memories m WHERE `+inVault+`) m
		WHERE m.n <= ? ORDER BY m.taken_at DESC`, vaultPreview)
	if err != nil {
		return err
	}
	for _, c := range list {
		if y := byYear[c.Year]; y != nil {
			y.Memories = append(y.Memories, c)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"years": years})
	return nil
}

// vaultPreview is the number of memories shown per year before "Show all".
const vaultPreview = 12
