package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"appstoreanalytics/internal/store"
)

func openStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

var quiet = slog.New(slog.DiscardHandler)

func TestAnArchiveIsOneDatabaseSentWithItsSize(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)

	var received atomic.Int64
	var body atomic.Value
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		// Sized up front: an agent may refuse a chunked body.
		if r.ContentLength != int64(len(b)) || len(r.TransferEncoding) > 0 {
			t.Errorf("sent %d bytes as %d, %v", len(b), r.ContentLength, r.TransferEncoding)
		}
		body.Store(b)
		received.Add(1)
	}))
	defer agent.Close()

	if err := send(context.Background(), st, agent.URL, dir); err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 {
		t.Fatalf("the agent got %d archives", received.Load())
	}
	raw, _ := body.Load().([]byte)
	if names := membersOf(t, raw); len(names) != 1 || names[0] != store.FileName {
		t.Errorf("archive holds %v", names)
	}
	// The pass cleans up after itself.
	if left, _ := filepath.Glob(filepath.Join(dir, scratch+"*")); len(left) > 0 {
		t.Errorf("left behind %v", left)
	}
}

// The counter is the signal: what Apple will not give again, what came to a webhook and what
// somebody set up move it, and the process noticing itself, or what the next sync reads anyway,
// does not.
func TestOnlyContentCountsAsAChange(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir())

	moved := func(what string, f func() error) {
		t.Helper()
		before := st.Changes()
		if err := f(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if st.Changes() == before {
			t.Errorf("%s did not count as a change", what)
		}
	}
	still := func(what string, f func() error) {
		t.Helper()
		before := st.Changes()
		if err := f(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if st.Changes() != before {
			t.Errorf("%s counted as a change, which would schedule a backup", what)
		}
	}

	still("a sign-in", func() error { _, err := st.CreateSession(ctx, "token", "curl/8"); return err })
	still("a sync's bookkeeping", func() error { return st.SetSyncState(ctx, "analytics", "") })
	still("rates", func() error { return st.SaveRates(ctx, map[string]float64{"EUR": 0.9}) })
	still("the apps", func() error {
		return st.SaveApps(ctx, []store.App{{ID: "42", Name: "Dots", BundleID: "com.acme.dots"}})
	})
	still("report requests", func() error {
		return st.SaveRequests(ctx, []store.Request{{ID: "r", AppID: "42", AccessType: "ONGOING"}})
	})

	moved("an analytics instance", func() error {
		return st.SaveInstance(ctx, store.Instance{ID: "i", AppID: "42", Report: "App Downloads Standard", ProcessingDate: "2026-11-15"},
			store.SameReport(store.MetricFirstDownloads),
			[]store.Measured{{AppID: "42", Day: "2026-11-14", Metric: store.MetricFirstDownloads, Territory: "US", Value: 3}}, nil)
	})
	moved("a sales day read first", func() error { return st.SaveSalesDay(ctx, "2026-11-15", nil, nil) })
	still("the same day read again", func() error { return st.SaveSalesDay(ctx, "2026-11-15", nil, nil) })

	relay := store.Relay{Host: "smtp.example.com", Port: 587, Security: store.SecurityStartTLS, FromAddress: "stats@example.com"}
	moved("a relay", func() error { return st.SetRelay(ctx, relay) })
	still("the same relay again", func() error { return st.SetRelay(ctx, relay) })
	moved("its port", func() error { relay.Port = 465; return st.SetRelay(ctx, relay) })
	moved("forgetting it", func() error { return st.DeleteRelay(ctx) })
	still("forgetting it again", func() error { return st.DeleteRelay(ctx) })

	var endpoint *store.HookEndpoint
	moved("an address", func() (err error) { endpoint, err = st.AddHookEndpoint(ctx, "Dots", ""); return })
	var hook int64
	moved("a delivery", func() (err error) {
		hook, err = st.SaveHook(ctx, store.Hook{EndpointID: endpoint.ID, Event: "TEST", Body: []byte("{}")})
		return
	})
	moved("its mailing", func() error { return st.MarkHookMailed(ctx, hook, []string{"me@example.com"}, "") })
	var m *store.HookMail
	moved("a recipient", func() (err error) {
		m, err = st.AddHookMail(ctx, store.HookMail{To: "me@example.com", Events: []string{"TEST"}})
		return
	})
	still("the same recipient again", func() error { _, err := st.SetHookMail(ctx, *m); return err })
	moved("another kind for them", func() error {
		m.Events = append(m.Events, "REFUND")
		_, err := st.SetHookMail(ctx, *m)
		return err
	})
	moved("a recipient removed", func() error { return st.RemoveHookMail(ctx, m.ID) })
	moved("an address removed", func() error { return st.RemoveHookEndpoint(ctx, endpoint.ID) })
}

// A snapshot has to be a database rather than a copy of a moment that never existed.
func TestTheArchiveIsADatabaseThatOpens(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir())
	if _, err := st.AddHookEndpoint(ctx, "Dots", "a secret"); err != nil {
		t.Fatal(err)
	}

	restored := t.TempDir()
	if err := st.SnapshotTo(ctx, filepath.Join(restored, store.FileName)); err != nil {
		t.Fatal(err)
	}
	reopened := openStore(t, restored)
	endpoints, err := reopened.HookEndpoints(ctx)
	if err != nil || len(endpoints) != 1 || endpoints[0].Label != "Dots" || endpoints[0].Secret != "a secret" {
		t.Errorf("the restored database does not know the address: %+v, %v", endpoints, err)
	}
}

func TestAFailingAgentIsReported(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer agent.Close()

	if err := send(context.Background(), st, agent.URL, dir); err == nil {
		t.Fatal("a refusing agent was reported as a success")
	}
}

func TestArchiveMembersAreNotDirectories(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)
	snapshot := filepath.Join(dir, "snap.db")
	if err := st.SnapshotTo(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "snap.tar.gz")
	if err := pack(snapshot, archive); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	tr := tarOf(t, raw)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeDir {
			t.Errorf("%s is a directory entry", h.Name)
		}
		if h.Mode != 0o600 {
			t.Errorf("%s is mode %o", h.Name, h.Mode)
		}
	}
}

// A pass the process died in leaves its snapshot behind; the next start removes it, backups on
// or off, and leaves the database alone.
func TestLeftoversAreRemovedOnStart(t *testing.T) {
	dir := t.TempDir()
	st := openStore(t, dir)

	left := filepath.Join(dir, scratch+"123")
	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, store.FileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	Run(context.Background(), st, "", dir, quiet)

	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Errorf("the leftover is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, store.FileName)); err != nil {
		t.Errorf("the database went with it: %v", err)
	}
}

func tarOf(t *testing.T, raw []byte) *tar.Reader {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return tar.NewReader(gz)
}

func membersOf(t *testing.T, raw []byte) []string {
	t.Helper()
	tr := tarOf(t, raw)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	return names
}
