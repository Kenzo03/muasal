// Command app is Muasal's single server binary (FSD §4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // timezone names also work in the distroless image

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/migrate"
)

const usage = `usage:
  app serve                                  run the API (migrates first when MIGRATE_DATABASE_URL is set)
  app migrate up                             apply migrations and prepare the app database role
  app admin create-admin --email E --name N  create an admin and print a one-time setup link
  app admin reindex --all                    queue an index job for every ticket, e.g. after a restore
  app healthcheck                            exit 0 when the API on LISTEN_ADDR is ready`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), os.Args[1:], log); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	switch {
	case args[0] == "serve":
		return serve(ctx, cfg, log)
	case args[0] == "healthcheck":
		return healthcheck(cfg)
	case len(args) >= 2 && args[0] == "migrate" && args[1] == "up":
		if cfg.MigrateDatabaseURL == "" {
			return errors.New("MIGRATE_DATABASE_URL is required for migrate")
		}
		return migrate.Up(ctx, cfg.MigrateDatabaseURL, cfg.DatabaseURL)
	case len(args) >= 2 && args[0] == "admin" && args[1] == "create-admin":
		return createAdmin(ctx, cfg, log, args[2:])
	case len(args) == 3 && args[0] == "admin" && args[1] == "reindex" && args[2] == "--all":
		return reindexAll(ctx, cfg, log)
	}
	return errors.New(usage)
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.MigrateDatabaseURL != "" {
		if err := migrate.Up(ctx, cfg.MigrateDatabaseURL, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	api := httpapi.New(cfg, pool, log)
	// The index workers share the API's AI runtime, so embedding pauses while
	// an answer is generated (FSD §11.7).
	workers, err := indexer.NewClient(pool, api.AI(), log, indexer.Options{OwnerURL: cfg.MigrateDatabaseURL})
	if err != nil {
		return err
	}
	if err := workers.Start(ctx); err != nil {
		return fmt.Errorf("start index workers: %w", err)
	}
	defer func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = workers.Stop(stop)
	}()
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func createAdmin(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	email := fs.String("email", "", "admin email address (required)")
	name := fs.String("name", "", "admin display name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	link, err := httpapi.New(cfg, pool, log).CreateAdmin(ctx, *email, *name)
	if err != nil {
		return err
	}
	fmt.Printf("Admin %s created. Open this link within 72 hours to set the password:\n%s\n", *email, link)
	return nil
}

// reindexAll queues every ticket for the running workers (FSD §13.4, §19).
func reindexAll(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: log})
	if err != nil {
		return err
	}
	n, err := indexer.QueueAll(ctx, pool, client)
	if err != nil {
		return err
	}
	fmt.Printf("Queued %d tickets; Admin → AI → Index status shows the progress.\n", n)
	return nil
}

// healthcheck lets the distroless image report readiness without curl.
func healthcheck(cfg config.Config) error {
	addr := cfg.ListenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	res, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("readyz answered %s", res.Status)
	}
	return nil
}
