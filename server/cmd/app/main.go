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

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/draft"
	"github.com/kenzo03/muasal/server/internal/eval"
	"github.com/kenzo03/muasal/server/internal/gitlink"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/migrate"
	"github.com/kenzo03/muasal/server/internal/ticketimport"
)

const usage = `usage:
  app serve                                  run the API (migrates first when MIGRATE_DATABASE_URL is set)
  app migrate up                             apply migrations and prepare the app database role
  app admin create-admin --email E --name N  create an admin and print a one-time setup link
  app admin reindex --all                    queue an index job for every ticket, e.g. after a restore
  app admin ai-local --url URL --tier T      set AI to Local on one model server with a tier's presets (dev, minimum, recommended)
  app eval [--seed] [--use-local URL] [--set FILE] [--data FILE] [--strict-latency]
                                             run the Ask golden set (FSD §11.8); --seed loads the demo project first
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
	case len(args) >= 2 && args[0] == "admin" && args[1] == "ai-local":
		return aiLocal(ctx, cfg, args[2:])
	case len(args) == 3 && args[0] == "admin" && args[1] == "reindex" && args[2] == "--all":
		return reindexAll(ctx, cfg, log)
	case args[0] == "eval":
		return runEval(ctx, cfg, args[1:])
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
	defer api.Close()
	// The index workers share the API's AI runtime, so embedding pauses while
	// an answer is generated (FSD §11.7).
	workers, err := indexer.NewClient(pool, api.AI(), log, indexer.Options{
		OwnerURL: cfg.MigrateDatabaseURL, AskLogDays: cfg.AskLogRetentionDays,
		Register: func(ws *river.Workers) {
			river.AddWorker(ws, &ticketimport.Worker{Pool: pool})
			river.AddWorker(ws, &gitlink.Worker{Pool: pool})
			river.AddWorker(ws, &draft.TreeWorker{Pool: pool, AI: api.AI()})
		},
	})
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

// aiLocal switches AI to Local with a tier's presets, as the installer does
// (FSD §19.3). The change is recorded as the first system admin's.
func aiLocal(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("ai-local", flag.ContinueOnError)
	url := fs.String("url", "http://model:11434/v1", "the model server's OpenAI-compatible base URL")
	tier := fs.String("tier", "minimum", "hardware tier: dev, minimum or recommended (§18.1)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	var adminID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM users WHERE is_admin AND disabled_at IS NULL ORDER BY id LIMIT 1").Scan(&adminID); err != nil {
		return fmt.Errorf("no system admin yet; run `app admin create-admin` first: %w", err)
	}
	q := db.New(pool)
	store := ai.NewStore(q)
	cur, err := store.Get(ctx)
	if err != nil {
		return err
	}
	s, err := cur.LocalForTier(*url, *tier)
	if err != nil {
		return err
	}
	if p := s.Validate(); len(p) > 0 {
		return fmt.Errorf("%s: %s", p[0].Field, p[0].Message)
	}
	if err := store.Put(ctx, q, s, adminID); err != nil {
		return err
	}
	fmt.Printf("AI set to Local at %s: chat %s, embeddings %s (%s tier).\n", *url, s.Chat.Model, s.Embed.Model, *tier)
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

// runEval runs the golden set as the first system admin (FSD §11.8) and fails
// when citation precision, evidence recall or abstention misses its target.
func runEval(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	set := fs.String("set", "", "golden set, JSON lines (default: the built-in golden set v0)")
	data := fs.String("data", "", "demo dataset for --seed (default: the built-in HRIS demo)")
	seed := fs.Bool("seed", false, "load the demo dataset into project DEMO if missing, then index it")
	local := fs.String("use-local", "", "first set AI to Local with this base URL, e.g. http://host.docker.internal:11434/v1")
	chatModel := fs.String("chat-model", "qwen3.5:4b", "chat model for --use-local")
	embedModel := fs.String("embed-model", "bge-m3", "embedding model for --use-local")
	strict := fs.Bool("strict-latency", false, "fail when the median latency misses 15 s")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := db.New(pool)
	var admin db.User
	if err := pool.QueryRow(ctx, "SELECT id, locale FROM users WHERE is_admin AND disabled_at IS NULL ORDER BY id LIMIT 1").Scan(&admin.ID, &admin.Locale); err != nil {
		return fmt.Errorf("no system admin to ask as; run `app admin create-admin` first: %w", err)
	}
	rt := &ai.Runtime{Store: ai.NewStore(q), Gate: ai.NewGate(), SecretKey: cfg.SecretKey, HTTP: &http.Client{}}
	if *local != "" {
		s, err := rt.Store.Get(ctx)
		if err != nil {
			return err
		}
		s.Mode = ai.ModeLocal
		s.Chat = ai.Endpoint{URL: *local, Model: *chatModel}
		s.Embed = ai.Endpoint{URL: *local, Model: *embedModel}
		if p := s.Validate(); len(p) > 0 {
			return fmt.Errorf("--use-local: %s %s", p[0].Field, p[0].Message)
		}
		if err := rt.Store.Put(ctx, q, s, admin.ID); err != nil {
			return err
		}
		fmt.Printf("AI set to Local: %s, chat %s, embeddings %s\n", *local, *chatModel, *embedModel)
	}
	if *seed {
		ds, err := eval.LoadDataset(*data)
		if err != nil {
			return err
		}
		fmt.Println("Loading and indexing the demo dataset…")
		if err := eval.Seed(ctx, pool, rt, ds, os.Stdout); err != nil {
			return err
		}
	}
	questions, err := eval.LoadQuestions(*set)
	if err != nil {
		return err
	}
	asker := ask.Asker{UserID: admin.ID, IsAdmin: true, Locale: admin.Locale, TZ: time.UTC}
	rep, err := eval.Run(ctx, pool, rt, asker, questions, 7, os.Stdout)
	if err != nil {
		return err
	}
	targets := eval.DefaultTargets
	targets.Strict = *strict
	rep.Print(os.Stdout, targets)
	if !rep.Pass(targets) {
		return errors.New("the golden set missed a target")
	}
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
