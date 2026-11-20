// Command appstoreconnect stands in for App Store Connect in the demo: an invented account of
// six apps, with years of analytics reports, made from the day it is. See dev/README.md.
//
//	appstoreconnect serve    answers as https://api.appstoreconnect.apple.com
//	appstoreconnect ready    whether serve is answering, for a health check
//	appstoreconnect seed     sets appstoreanalytics up once it is running
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	// Apple's report days are Pacific days, and the image has no zone files of its own.
	_ "time/tzdata"
)

// Environment variables read here.
const (
	// stateEnv is where the keys and what was asked for are kept, shared with appstoreanalytics.
	stateEnv = "APPSTORECONNECT_STATE"
	// namesEnv is the host names answered to, comma-separated; the first is the stand-in's own.
	namesEnv = "APPSTORECONNECT_NAMES"
	// issuerEnv and keyIDEnv are the team's issuer id and its key's id, which tokens must name.
	issuerEnv = "APPSTORECONNECT_ISSUER_ID"
	keyIDEnv  = "APPSTORECONNECT_KEY_ID"
	// deliverToEnv is where webhooks are delivered in place of their address's scheme and host.
	deliverToEnv = "APPSTORECONNECT_DELIVER_TO"
	// nowEnv fixes the moment it is, RFC 3339, so that every run makes the same days and events:
	// the screenshots' account. Unset, it is the time.
	nowEnv = "APPSTORECONNECT_NOW"
	// siteEnv is appstoreanalytics as the seed reaches it, and usernameEnv and passwordEnv its
	// sign-in.
	siteEnv     = "APPSTORECONNECT_SITE"
	usernameEnv = "APPSTORECONNECT_USERNAME"
	passwordEnv = "APPSTORECONNECT_PASSWORD"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch command := strings.Join(os.Args[1:], " "); command {
	case "serve":
		err = serve(ctx, log)
	case "ready":
		err = ready()
	case "seed":
		err = seed(ctx, log)
	default:
		err = fmt.Errorf("say serve, ready or seed, not %q", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func names() []string {
	return strings.Split(env(namesEnv, "api.appstoreconnect.apple.com"), ",")
}

func serve(ctx context.Context, log *slog.Logger) error {
	dir := env(stateEnv, "/state")
	keys, err := ensureKeys(dir, names())
	if err != nil {
		return err
	}
	st, err := loadState(filepath.Join(dir, "state.json"))
	if err != nil {
		return fmt.Errorf("read what was asked for: %w", err)
	}
	s := &Server{
		Issuer: env(issuerEnv, ""), KeyID: env(keyIDEnv, ""), Key: &keys.API.PublicKey,
		Origin: "https://" + names()[0], DeliverTo: env(deliverToEnv, ""),
		Log: log, Now: time.Now, Pause: 700 * time.Millisecond,
	}
	if at := env(nowEnv, ""); at != "" {
		fixed, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return fmt.Errorf("%s is %q, and it is a moment as RFC 3339 writes one", nowEnv, at)
		}
		s.Now = func() time.Time { return fixed }
	}
	if s.Issuer == "" || s.KeyID == "" {
		return fmt.Errorf("%s and %s are the issuer and key id tokens must name", issuerEnv, keyIDEnv)
	}
	srv := &http.Server{
		Addr:      ":443",
		Handler:   s.Handler(st),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{keys.TLS}},
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Info("answering as App Store Connect", "names", strings.Join(names(), ","))
	if err := srv.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ready is serve answering over TLS as the names it answers to, its keys there for
// appstoreanalytics to read.
func ready() error {
	dir := env(stateEnv, "/state")
	ca, err := os.ReadFile(filepath.Join(dir, caFile))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: names()[0]},
	}}
	resp, err := client.Get("https://localhost/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if _, err := os.Stat(filepath.Join(dir, apiKeyB64File)); err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("answered %s", resp.Status)
	}
	return nil
}

func seed(ctx context.Context, log *slog.Logger) error {
	dir := env(stateEnv, "/state")
	key, err := readAPIKey(dir)
	if err != nil {
		return err
	}
	s := &Seeder{
		Site:     env(siteEnv, "http://appstoreanalytics"),
		Username: env(usernameEnv, ""), Password: env(passwordEnv, ""),
		Apple:  "https://" + names()[0],
		Issuer: env(issuerEnv, ""), KeyID: env(keyIDEnv, ""), Key: key,
		Log: log,
	}
	if err := s.Run(ctx); err != nil {
		return err
	}
	log.Info("the demo is set up")
	return nil
}
