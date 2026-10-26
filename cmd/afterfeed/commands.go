package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/library"
)

func authCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: afterfeed auth reset-2fa | reset-password | reset-vault-pin")
	}
	_, st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	svc := auth.New(st)
	u, err := svc.FirstUser(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("there is no account yet; open Afterfeed in a browser to create one")
	}
	if err != nil {
		return err
	}

	switch args[0] {
	case "reset-2fa":
		if !u.TOTPEnabled {
			fmt.Println("Two-factor authentication is already off.")
			return nil
		}
		if err := svc.DisableTOTP(ctx, u); err != nil {
			return err
		}
		svc.ResetAttempts(ctx, "mfa:")
		slog.Warn("two-factor authentication reset from the command line", "user", u.Username)
		fmt.Printf("Two-factor authentication is off for %s.\n", u.Username)
		fmt.Println("Sign in with your password and turn it on again under Settings › Security.")
	case "reset-password":
		pw, err := svc.ResetPassword(ctx, u)
		if err != nil {
			return err
		}
		slog.Warn("password reset from the command line", "user", u.Username)
		fmt.Printf("Temporary password for %s:\n\n    %s\n\n", u.Username, pw)
		fmt.Println("Sign in with it; Afterfeed asks you to choose a new password. All sessions were signed out.")
	case "reset-vault-pin":
		if err := svc.ResetVaultPIN(ctx, u); err != nil {
			return err
		}
		slog.Warn("vault PIN reset from the command line", "user", u.Username)
		fmt.Println("The vault PIN is removed. The vault keeps its contents; set a new PIN the next time you open it.")
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
	return nil
}

func backupCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: afterfeed backup <file>")
	}
	target, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists", target)
	}
	_, st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.W.Exec(`VACUUM INTO ?`, target); err != nil {
		return err
	}
	fmt.Printf("Database copied to %s\n", target)
	return nil
}

func indexCommand(args []string) error {
	if len(args) == 0 || args[0] != "rebuild" {
		return errors.New("usage: afterfeed index rebuild")
	}
	cfg, st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	lib := library.New(cfg.MemoriesDir(), cfg.MessagesDir(), st)
	lib.SetGeocoder(geocoder)
	fmt.Println("Rebuilding the index from", cfg.DataDir, "…")
	start := time.Now()
	stats, err := lib.Rebuild(context.Background(), func(s library.RebuildStats) {
		fmt.Printf("\r%d memories, %d conversations", s.Memories, s.Conversations)
	})
	fmt.Println()
	if err != nil {
		return err
	}
	slog.Warn("index rebuilt from the command line", "memories", stats.Memories, "conversations", stats.Conversations)
	fmt.Printf("Done in %s: %d memories and %d conversations.\n", time.Since(start).Round(time.Second), stats.Memories, stats.Conversations)
	if n := len(stats.Skipped); n > 0 {
		fmt.Printf("%d files could not be read:\n", n)
		for _, p := range stats.Skipped {
			fmt.Println("  " + p)
		}
	}
	return nil
}
