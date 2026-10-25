package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultMapStyle is used when the map is set to the external server without
// a custom style. OpenFreeMap needs no account or key.
const DefaultMapStyle = "https://tiles.openfreemap.org/styles/positron"

// MapSettings choose where map tiles come from.
type MapSettings struct {
	// Mode is "external", "local" or "off".
	Mode     string `json:"mode"`
	StyleURL string `json:"style_url"`
}

// VaultSettings control the vault.
type VaultSettings struct {
	AutolockMinutes int `json:"autolock_minutes"`
}

// Settings are the user-editable settings.
type Settings struct {
	Map   MapSettings   `json:"map"`
	Vault VaultSettings `json:"vault"`
}

func defaultSettings() Settings {
	return Settings{
		Map:   MapSettings{Mode: "external"},
		Vault: VaultSettings{AutolockMinutes: 5},
	}
}

// LoadSettings reads the settings, filling in defaults.
func (s *Server) LoadSettings(ctx context.Context) Settings {
	out := defaultSettings()
	var m MapSettings
	if ok, _ := s.store.GetSetting(ctx, "map", &m); ok {
		out.Map = m
	}
	var v VaultSettings
	if ok, _ := s.store.GetSetting(ctx, "vault", &v); ok && v.AutolockMinutes > 0 {
		out.Vault = v
	}
	return out
}

// EffectiveStyleURL is the map style the app should load, or "" when off.
func (m MapSettings) EffectiveStyleURL() string {
	switch m.Mode {
	case "off":
		return ""
	case "local":
		return m.StyleURL
	default:
		if m.StyleURL != "" {
			return m.StyleURL
		}
		return DefaultMapStyle
	}
}

func (s *Server) vaultAutolock(ctx context.Context) time.Duration {
	return time.Duration(s.LoadSettings(ctx).Vault.AutolockMinutes) * time.Minute
}

// applyMapHosts allows the map style's host in the content security policy.
func applyMapHosts(m MapSettings) {
	style := m.EffectiveStyleURL()
	if style == "" {
		SetMapHosts(nil)
		return
	}
	u, err := url.Parse(style)
	if err != nil || u.Host == "" {
		SetMapHosts(nil)
		return
	}
	SetMapHosts([]string{u.Scheme + "://" + u.Host})
}

func (s *Server) settingsRoutes() {
	s.mux.HandleFunc("GET /api/settings", s.authed(s.getSettings, false))
	s.mux.HandleFunc("PATCH /api/settings", s.authed(s.patchSettings, false))
	applyMapHosts(s.LoadSettings(context.Background()).Map)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	set := s.LoadSettings(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"map":           set.Map,
		"vault":         set.Vault,
		"map_style":     set.Map.EffectiveStyleURL(),
		"default_style": DefaultMapStyle,
	})
	return nil
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Map   *MapSettings   `json:"map"`
		Vault *VaultSettings `json:"vault"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	if req.Map != nil {
		m := *req.Map
		m.StyleURL = strings.TrimSpace(m.StyleURL)
		switch m.Mode {
		case "external", "local", "off":
		default:
			return errBadRequest("settings.invalid_map_mode", "unknown map mode")
		}
		if m.StyleURL != "" {
			u, err := url.Parse(m.StyleURL)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return errBadRequest("settings.invalid_style_url", "invalid style URL")
			}
		}
		if m.Mode == "local" && m.StyleURL == "" {
			return errBadRequest("settings.invalid_style_url", "a style URL is required for local map files")
		}
		if err := s.store.SetSetting(ctx, "map", m); err != nil {
			return err
		}
		applyMapHosts(m)
	}
	if req.Vault != nil {
		v := *req.Vault
		if v.AutolockMinutes < 1 || v.AutolockMinutes > 120 {
			return errBadRequest("settings.invalid_autolock", "autolock must be between 1 and 120 minutes")
		}
		if err := s.store.SetSetting(ctx, "vault", v); err != nil {
			return err
		}
	}
	return s.getSettings(w, r)
}
