// Command afterfeed runs the Afterfeed server and its maintenance commands.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/server"
	"github.com/rudyh0/afterfeed/internal/store"
	"github.com/rudyh0/afterfeed/internal/version"
)

const usage = `Afterfeed – your memories, without the platform.

Usage:
  afterfeed [command]

Commands:
  serve                 Run the server (default)
  version               Print the version
  healthcheck           Exit with status 0 when the server is healthy
  auth reset-2fa        Turn off two-factor authentication for the account
  auth reset-password   Set a new temporary password for the account
  auth reset-vault-pin  Remove the vault PIN so a new one can be set
  index rebuild         Rebuild the database index from the data volume
  backup <file>         Write a consistent copy of the database to a file

Run commands inside the container, for example:
  docker exec -it afterfeed afterfeed auth reset-2fa
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "version", "--version", "-v":
		fmt.Printf("afterfeed %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
	case "healthcheck":
		err = healthcheck()
	case "auth":
		err = authCommand(args)
	case "index":
		err = indexCommand(args)
	case "backup":
		err = backupCommand(args)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}

func openStore() (*config.Config, *store.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureDirs(); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(cfg.DatabasePath())
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	return cfg, st, nil
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogging(cfg.LogLevel)

	cfg, st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := newApp(ctx, cfg, st)
	if err != nil {
		return err
	}
	defer app.Close()

	srv := server.New(cfg, st, app.deps())
	return srv.Run(ctx)
}

func healthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", cfg.Port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
