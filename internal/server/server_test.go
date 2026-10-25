package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/store"
)

type testClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func newTestServer(t *testing.T) (*testClient, *Deps) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		ConfigDir:     filepath.Join(dir, "config"),
		DataDir:       filepath.Join(dir, "data"),
		CacheDir:      filepath.Join(dir, "cache"),
		ImportDir:     filepath.Join(dir, "data", "import"),
		SecureCookies: "auto",
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	deps := testDeps(t, cfg, st)
	srv := httptest.NewServer(New(cfg, st, deps).Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &testClient{t: t, base: srv.URL, http: &http.Client{Jar: jar}}, &deps
}

func (c *testClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Afterfeed", "1")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestAuthFlow(t *testing.T) {
	c, _ := newTestServer(t)

	var state map[string]any
	c.do("GET", "/api/state", nil, &state)
	if state["setup_required"] != true || state["authenticated"] != false {
		t.Fatalf("unexpected initial state %v", state)
	}

	if code := c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "short"}, nil); code != 400 {
		t.Fatalf("short password: got %d", code)
	}
	if code := c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "a long enough password", "locale": "nl"}, nil); code != 200 {
		t.Fatalf("setup: got %d", code)
	}
	c.do("GET", "/api/state", nil, &state)
	if state["authenticated"] != true {
		t.Fatalf("should be signed in after setup: %v", state)
	}
	if code := c.do("POST", "/api/setup", map[string]string{"username": "x", "password": "a long enough password"}, nil); code != 403 {
		t.Fatalf("second setup: got %d", code)
	}

	// Turn on two-factor authentication.
	var begin struct{ Secret, URI string }
	if code := c.do("POST", "/api/account/2fa/begin", nil, &begin); code != 200 || begin.Secret == "" {
		t.Fatalf("begin 2fa: %d", code)
	}
	code, _ := auth.TOTPCode(begin.Secret, time.Now())
	var enabled struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if status := c.do("POST", "/api/account/2fa/enable", map[string]string{"code": code}, &enabled); status != 200 || len(enabled.RecoveryCodes) != 10 {
		t.Fatalf("enable 2fa: %d", status)
	}

	if status := c.do("POST", "/api/auth/logout", nil, nil); status != 204 {
		t.Fatalf("logout: %d", status)
	}
	c.do("GET", "/api/state", nil, &state)
	if state["authenticated"] != false {
		t.Fatal("should be signed out")
	}

	if status := c.do("POST", "/api/auth/login", map[string]string{"username": "rudy", "password": "wrong password!"}, nil); status != 401 {
		t.Fatalf("wrong password: %d", status)
	}
	var login struct {
		MFARequired bool `json:"mfa_required"`
	}
	if status := c.do("POST", "/api/auth/login", map[string]string{"username": "rudy", "password": "a long enough password"}, &login); status != 200 || !login.MFARequired {
		t.Fatalf("login: %d %v", status, login)
	}
	// A session waiting for the second factor cannot use the API.
	if status := c.do("POST", "/api/take-in/done", nil, nil); status != 401 {
		t.Fatalf("pending session should be refused: %d", status)
	}
	if status := c.do("POST", "/api/auth/mfa", map[string]string{"code": enabled.RecoveryCodes[0]}, nil); status != 200 {
		t.Fatalf("mfa with recovery code: %d", status)
	}
	c.do("GET", "/api/state", nil, &state)
	if state["authenticated"] != true {
		t.Fatal("should be signed in after the second factor")
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	c, _ := newTestServer(t)
	req, _ := http.NewRequest("POST", c.base+"/api/setup", bytes.NewBufferString(`{}`))
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 without the header, got %d", res.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	c, _ := newTestServer(t)
	c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "a long enough password"}, nil)
	c.do("POST", "/api/auth/logout", nil, nil)
	var last int
	for i := 0; i < 6; i++ {
		last = c.do("POST", "/api/auth/login", map[string]string{"username": "rudy", "password": "wrong password!"}, nil)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("want 429 after repeated failures, got %d", last)
	}
	// Even the right password is refused while locked.
	if status := c.do("POST", "/api/auth/login", map[string]string{"username": "rudy", "password": "a long enough password"}, nil); status != http.StatusTooManyRequests {
		t.Fatalf("want 429 while locked, got %d", status)
	}
}
