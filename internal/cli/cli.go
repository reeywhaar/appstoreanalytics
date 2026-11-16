// Package cli is the command line: the server, and the two questions a container asks of it.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"appstoreanalytics/internal/api"
	"appstoreanalytics/internal/app"
	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/pull"
	"appstoreanalytics/internal/store"
)

const usage = `usage: appstoreanalytics <command>

  serve        run the server
  healthcheck  ask the running server whether it is well
  version      print the version this binary was built from
`

// Execute runs the command line and returns a process exit code.
func Execute(args []string) int {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "serve":
		err = serve()
	case "healthcheck":
		err = healthcheck()
	case "version":
		fmt.Println(app.Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, app.Name+":", err)
		return 1
	}
	return 0
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// JSON, so a line is a record a collector reads field by field.
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel, ReplaceAttr: readable}))

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signedOut, err := st.KeepCredential(ctx, cfg.Username, cfg.Password)
	if err != nil {
		return err
	}
	if signedOut {
		log.Info("the username or password changed, so every browser was signed out")
	}
	if cfg.PublicURL.Scheme != "https" {
		log.Warn("the public url is http, so the session cookie goes without Secure", "url", cfg.PublicURL.String())
	}

	spa, err := api.NewSPA(bundle(log))
	if err != nil {
		return err
	}
	apple := asc.New(cfg.Apple)
	puller := pull.New(st, apple, cfg.Apple.VendorNumber, log)
	srv := &http.Server{
		Addr:              app.ListenAddr,
		Handler:           api.New(cfg, log, st, spa, puller, apple),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Waited on below, so nothing is still writing when the database closes.
	var background sync.WaitGroup
	background.Go(func() { puller.Run(ctx) })
	background.Go(func() { sweep(ctx, st, log) })

	errc := make(chan error, 1)
	go func() {
		log.Info("appstoreanalytics started", "addr", app.ListenAddr, "version", app.Version,
			"url", cfg.PublicURL.String(), "data", cfg.DataDir)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		stop()
		background.Wait()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		// Bounded, or one slow request costs the full kill timeout every deploy.
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Info("appstoreanalytics stopping")
		err := srv.Shutdown(shutdown)
		background.Wait()
		return err
	}
}

// bundle is the built frontend: the image's, or a checkout's own web/dist. Neither is the
// placeholder page, said once at startup.
func bundle(log *slog.Logger) fs.FS {
	for _, dir := range []string{app.WebDir, "web/dist"} {
		if _, err := os.Stat(dir); err == nil {
			return os.DirFS(dir)
		}
	}
	log.Warn("no web directory: browsers get the placeholder page instead of the app", "dir", app.WebDir)
	return nil
}

// sweep deletes lapsed sessions, which are already unusable, every hour.
func sweep(ctx context.Context, st *store.Store, log *slog.Logger) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if n, err := st.SweepSessions(ctx); err != nil && ctx.Err() == nil {
			log.Error("expired sessions could not be swept", "error", err.Error())
		} else if n > 0 {
			log.Info("swept expired sessions", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// readable writes a duration as one reads it, "1.2s", rather than as a count of nanoseconds.
func readable(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindDuration {
		return slog.String(a.Key, a.Value.Duration().String())
	}
	return a
}

// healthcheck is what HEALTHCHECK runs: a second process asking the first, so the image needs
// no HTTP client and a wedged server fails it.
func healthcheck() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + app.ListenAddr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz answered %s", resp.Status)
	}
	return nil
}
