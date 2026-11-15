package pull

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/store"
)

// analyticsApple stands in for App Store Connect with no sales and two apps: Dots, whose
// ongoing request Apple stopped and whose reports have one daily instance, and Lines, which
// nobody has asked for reports of yet.
func analyticsApple(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	created := map[string]bool{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(map[string]any{"data": v}) }
		path := r.URL.Path
		switch {
		case path == "/v1/apps":
			reply([]map[string]any{
				{"id": "1", "attributes": map[string]any{"name": "Dots", "sku": "dots"}},
				{"id": "2", "attributes": map[string]any{"name": "Lines", "sku": "lines"}},
			})
		case path == "/v1/salesReports":
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"detail": "There were no sales for the date specified."}}})
		case path == "/v1/apps/1/analyticsReportRequests":
			list := []map[string]any{
				{"id": "dots-old", "attributes": map[string]any{"accessType": "ONGOING", "stoppedDueToInactivity": true}},
				{"id": "dots-snap", "attributes": map[string]any{"accessType": "ONE_TIME_SNAPSHOT"}},
			}
			if created["1-ONGOING"] {
				list = append(list, map[string]any{"id": "1-ONGOING", "attributes": map[string]any{"accessType": "ONGOING"}})
			}
			reply(list)
		case path == "/v1/apps/2/analyticsReportRequests":
			var list []map[string]any
			for _, kind := range []string{"ONGOING", "ONE_TIME_SNAPSHOT"} {
				if created["2-"+kind] {
					list = append(list, map[string]any{"id": "2-" + kind, "attributes": map[string]any{"accessType": kind}})
				}
			}
			reply(list)
		case path == "/v1/analyticsReportRequests" && r.Method == http.MethodPost:
			var body struct {
				Data struct {
					Attributes struct {
						AccessType string `json:"accessType"`
					} `json:"attributes"`
					Relationships struct {
						App struct {
							Data struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"app"`
					} `json:"relationships"`
				} `json:"data"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			id := body.Data.Relationships.App.Data.ID + "-" + body.Data.Attributes.AccessType
			created[id] = true
			w.WriteHeader(http.StatusCreated)
			reply(map[string]any{"id": id})
		case path == "/v1/analyticsReportRequests/dots-snap/reports":
			reply([]map[string]any{{"id": "snap-downloads", "attributes": map[string]any{"name": "App Downloads Standard"}}})
		case strings.HasPrefix(path, "/v1/analyticsReportRequests/"):
			reply([]any{})
		case path == "/v1/analyticsReports/snap-downloads/instances":
			reply([]map[string]any{{"id": "snap-1", "attributes": map[string]any{"processingDate": "2026-11-14"}}})
		case path == "/v1/analyticsReportInstances/snap-1/segments":
			reply([]map[string]any{{"attributes": map[string]any{"url": srv.URL + "/files/snap-1"}}})
		case path == "/files/snap-1":
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			z.Write([]byte("Date\tDownload Type\tSource Type\tTerritory\tCounts\n2026-11-13\tFirst-time download\tApp Store search\tUS\t4\n"))
			z.Close()
			w.Write(b.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPassKeepsWhereEachAppsReportsStand(t *testing.T) {
	srv := analyticsApple(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := New(st, asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "i", KeyID: "k", Key: key}), "1", slog.New(slog.DiscardHandler))
	p.now = func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }
	p.rates = func(context.Context) (map[string]float64, error) { return map[string]float64{"USD": 1}, nil }
	ctx := context.Background()
	p.Pass(ctx)

	got, err := st.RequestsByApp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("requests = %+v, want both apps", got)
	}
	dots, lines := got[0], got[1]
	// Apple stopped Dots' ongoing request; a new one was asked for, which is running.
	if dots.Name != "Dots" || dots.Ongoing != "running" || !dots.Snapshot || dots.Delivered != 1 || dots.Latest != "2026-11-14" || dots.Read != 1 {
		t.Errorf("Dots = %+v", dots)
	}
	if lines.Name != "Lines" || lines.Ongoing != "running" || !lines.Snapshot || lines.Delivered != 0 || lines.Since == 0 {
		t.Errorf("Lines = %+v", lines)
	}
}
