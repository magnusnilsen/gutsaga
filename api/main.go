package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magnusnilsen/gutsaga/internal/discord"
	"github.com/magnusnilsen/gutsaga/internal/smia"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "serve":
		os.Exit(serve(args))
	case "mint":
		os.Exit(mint(args))
	case "seed":
		os.Exit(seedCmd())
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `GutSaga API

  gutsaga-api                 start the HTTP server
  gutsaga-api seed            insert the local Elwynn test quests
  gutsaga-api mint            create or rotate a player API key
  gutsaga-api help

Environment:
  DATABASE_URL          Postgres connection string (required)
  DISCORD_WEBHOOK_URL   optional; saga notices post here as GutSaga
  ADMIN_TOKEN           bearer token for /v1/admin
  PORT                  listen port (default 8080)
  SMIA_AUTH_MODE        smia (default) or dev
  SMIA_DEV_TOKEN        static token when SMIA_AUTH_MODE=dev
  SMIA_MCP_URL          default https://smia.tools/wow/mcp
`)
}

func openPool(ctx context.Context) (*pgxpool.Pool, error) {
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func serve(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address (default :$PORT or :8080)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx := context.Background()
	pool, err := openPool(ctx)
	if err != nil {
		slog.Error("database", "err", err)
		return 1
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		slog.Error("migrate", "err", err)
		return 1
	}
	store := NewStore(pool)
	var notifier Notifier
	if hook := strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK_URL")); hook != "" {
		q := discord.NewQueue(&discord.Client{WebhookURL: hook})
		q.After = func(res discord.Result) {
			if res.Err != nil || res.Kind != discord.KindCompletion || res.Ref == "" {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := store.MarkDiscord(ctx, res.Ref, res.Message.AttachmentURL); err != nil {
				slog.Error("mark discord", "err", err)
			}
		}
		notifier = q
	} else {
		slog.Warn("DISCORD_WEBHOOK_URL is empty; Discord notices are skipped")
	}
	if strings.TrimSpace(os.Getenv("ADMIN_TOKEN")) == "" {
		slog.Warn("ADMIN_TOKEN is empty; admin routes are disabled")
	}
	srv := &Server{store: store, adminToken: os.Getenv("ADMIN_TOKEN"), discord: notifier, smia: newVerifier(store)}
	if strings.EqualFold(os.Getenv("SMIA_AUTH_MODE"), "dev") && strings.TrimSpace(os.Getenv("SMIA_DEV_TOKEN")) == "" {
		slog.Warn("SMIA_AUTH_MODE=dev but SMIA_DEV_TOKEN is empty; login will fail")
	}
	listen := *addr
	if listen == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		listen = ":" + port
	}
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	slog.Info("listening", "addr", listen)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		slog.Error("server", "err", err)
		return 1
	case <-sig:
	}
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shut)
	return 0
}

func newVerifier(store *Store) smia.Verifier {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SMIA_AUTH_MODE")))
	if mode == "dev" {
		return smia.DevVerifier{Token: strings.TrimSpace(os.Getenv("SMIA_DEV_TOKEN"))}
	}
	resource := strings.TrimSpace(os.Getenv("SMIA_MCP_URL"))
	if resource == "" {
		resource = "https://smia.tools/wow/mcp"
	}
	hub := strings.TrimSpace(os.Getenv("SMIA_HUB"))
	if hub == "" {
		hub = smia.Hub
	}
	server := strings.TrimSpace(os.Getenv("SMIA_SERVER"))
	if server == "" {
		server = smia.Server
	}
	return smia.MCPVerifier{ResourceURL: resource, Hub: hub, Server: server, Cache: storeSmiaCache{store: store}}
}

func seedCmd() int {
	ctx := context.Background()
	pool, err := openPool(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	created, updated, err := seed(ctx, NewStore(pool))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("seed: %d nye oppdrag, %d oppdatert\n", created, updated)
	return 0
}

func mint(args []string) int {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	tag := fs.String("battletag", "", "player BattleTag, for example Magnus#2231")
	name := fs.String("name", "", "display name")
	rotate := fs.Bool("rotate", false, "rotate the key if the BattleTag already exists")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *tag == "" || *name == "" && !*rotate {
		fmt.Fprintln(os.Stderr, "mint requires --battletag and --name")
		return 2
	}
	ctx := context.Background()
	pool, err := openPool(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store := NewStore(pool)
	var id string
	err = pool.QueryRow(ctx, `SELECT id::text FROM players WHERE battletag=$1`, strings.TrimSpace(*tag)).Scan(&id)
	if err == nil {
		if !*rotate {
			fmt.Fprintln(os.Stderr, "player already exists; pass --rotate to issue a new key")
			return 1
		}
		out, err := store.RotateKey(ctx, id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("battletag: %s\napi_key: %s\n", out.Player.BattleTag, out.APIKey)
		return 0
	}
	if *name == "" {
		fmt.Fprintln(os.Stderr, "--name is required for a new player")
		return 2
	}
	out, err := store.CreatePlayer(ctx, *tag, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("battletag: %s\napi_key: %s\n", out.Player.BattleTag, out.APIKey)
	return 0
}
