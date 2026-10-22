package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/magnusnilsen/gutsaga/internal/app"
	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/tray"
	"github.com/magnusnilsen/gutsaga/internal/version"
)

type options struct {
	APIURL   string
	Client   string
	WowPath  string
	Headless bool
	NoUpdate bool
}

func main() {
	opts, cmd, err := parseArgs(os.Args[1:])
	if err != nil {
		usage()
		os.Exit(2)
	}
	var code int
	switch cmd {
	case "run":
		code = run(opts)
	case "setup":
		code = setupCmd()
	case "sync":
		code = syncOnce(opts)
	case "version", "-version", "--version":
		fmt.Println("GutSaga", version.Version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		code = 2
	}
	os.Exit(code)
}

func parseArgs(args []string) (options, string, error) {
	var opts options
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--headless":
			opts.Headless = true
		case a == "--no-update":
			opts.NoUpdate = true
		case a == "--api-url" || strings.HasPrefix(a, "--api-url="):
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, "", err
			}
			opts.APIURL = v
		case a == "--client" || strings.HasPrefix(a, "--client="):
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, "", err
			}
			opts.Client = v
		case a == "--wow-path" || strings.HasPrefix(a, "--wow-path="):
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, "", err
			}
			opts.WowPath = v
		case strings.HasPrefix(a, "-"):
			return opts, "", fmt.Errorf("ukjent flagg %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if opts.APIURL == "" {
		opts.APIURL = os.Getenv("GUTSAGA_API_URL")
	}
	if opts.Client == "" {
		opts.Client = os.Getenv("GUTSAGA_CLIENT")
	}
	if opts.WowPath == "" {
		opts.WowPath = os.Getenv("GUTSAGA_WOW_PATH")
	}
	cmd := "run"
	if len(positionals) > 0 {
		cmd = positionals[0]
	}
	return opts, cmd, nil
}

func flagValue(a string, args []string, i *int) (string, error) {
	if eq := strings.IndexByte(a, '='); eq >= 0 {
		return a[eq+1:], nil
	}
	if *i+1 >= len(args) {
		return "", fmt.Errorf("%s trenger en verdi", a)
	}
	*i++
	return args[*i], nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `GutSaga – følgeprogram for oppdragene til gutane

  gutsaga                         start, installer tillegget, logg inn med Smia
  gutsaga --client retail         bruk _retail_ (Midnight)
  gutsaga --client classic_era    bruk _classic_era_ (Classic Era)
  gutsaga --client classic        bruk _classic_ (Mists)
  gutsaga --api-url http://localhost:8080
  gutsaga sync                    hent oppdrag én gang
  gutsaga setup                   valgfri mappe og oppdateringsrepo
  gutsaga --headless              kjør uten skuffikon

Miljøvariabler: GUTSAGA_API_URL, GUTSAGA_CLIENT (forever|retail|classic_era|classic), GUTSAGA_WOW_PATH, GUTSAGA_DEV_TOKEN.
Innstillingene lagres i config.json. Programmet leser chatloggen. Det sender aldri tastetrykk til spillet.
`)
}

func logger() *slog.Logger {
	var w io.Writer = os.Stdout
	if dir, err := config.Dir(); err == nil {
		_ = os.MkdirAll(dir, 0o755)
		if f, err := os.OpenFile(filepath.Join(dir, "gutsaga.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			w = io.MultiWriter(os.Stdout, f)
		}
	}
	return slog.New(slog.NewTextHandler(w, nil))
}

func loadOrSetup(log *slog.Logger, opts options) (config.Config, int) {
	cfg, err := config.Load()
	if os.IsNotExist(err) {
		cfg = config.Config{Client: "forever"}
		err = nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cfg, 1
	}
	cfg = applyOptions(cfg, opts)
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cfg, 1
	}
	if err := config.Save(cfg); err != nil {
		log.Warn("kunne ikke lagre config", "err", err)
	}
	return cfg, 0
}

func applyOptions(cfg config.Config, opts options) config.Config {
	if opts.APIURL != "" {
		cfg.APIBaseURL = strings.TrimRight(strings.TrimSpace(opts.APIURL), "/")
	} else if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = strings.TrimRight(version.DefaultAPIBaseURL, "/")
	}
	if opts.Client != "" {
		cfg.Client = strings.ToLower(strings.TrimSpace(opts.Client))
	}
	if opts.WowPath != "" {
		cfg.WowPath = opts.WowPath
	}
	cfg.Normalize()
	return cfg
}

func setupCmd() int {
	log := logger()
	current, _ := config.Load()
	if current.Client == "" {
		current.Client = "forever"
	}
	cfg, err := config.Setup(os.Stdin, os.Stdout, current)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := config.Save(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path, _ := config.Path()
	log.Info("oppsett lagret", "path", path)
	fmt.Println("Lagret", path)
	return 0
}

func run(opts options) int {
	log := logger()
	cfg, code := loadOrSetup(log, opts)
	if code != 0 {
		return code
	}
	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	application, err := app.New(cfg, filepath.Join(dir, "queue.json"), log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	application.NoUpdate = opts.NoUpdate
	if isTerminal(os.Stdin) {
		application.Prompt = func() (string, error) {
			fmt.Fprint(os.Stdout, "Lim inn koden fra nettleseren hvis vinduet ikke kom tilbake: ")
			var line string
			_, err := fmt.Fscanln(os.Stdin, &line)
			return strings.TrimSpace(line), err
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go application.Run(ctx)
	if opts.Headless || !tray.Supported() {
		fmt.Println("GutSaga kjører. Logg inn med Smia i nettleseren om du blir bedt om det. Avslutt med Ctrl+C.")
		<-ctx.Done()
		return 0
	}
	tray.Run(ctx, "GutSaga", []tray.Item{
		{Title: "Logg inn med Smia", Tooltip: "Åpne nettleseren og logg inn", OnClick: func() {
			go func() {
				if err := application.Login(context.Background()); err != nil {
					log.Warn("innlogging feilet", "err", err)
					return
				}
				if err := application.SyncNow(context.Background()); err != nil {
					log.Warn("synkronisering feilet", "err", err)
				}
			}()
		}},
		{Title: "Synkroniser nå", Tooltip: "Hent oppdrag fra API-et", OnClick: func() {
			go func() {
				if err := application.SyncNow(context.Background()); err != nil {
					log.Warn("synkronisering feilet", "err", err)
				}
			}()
		}},
		{Title: "Avslutt", Tooltip: "Avslutt GutSaga", OnClick: cancel},
	})
	return 0
}

func syncOnce(opts options) int {
	log := logger()
	cfg, code := loadOrSetup(log, opts)
	if code != 0 {
		return code
	}
	dir, err := config.Dir()
	if err != nil {
		return 1
	}
	application, err := app.New(cfg, filepath.Join(dir, "queue.json"), log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	application.NoUpdate = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := application.SyncNow(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Synkronisert. Skriv /reload i spillet hvis du er innlogget.")
	return 0
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
