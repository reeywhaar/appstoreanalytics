// Package asc speaks to App Store Connect: signing in with the API key, listing the apps, and
// asking for and reading analytics reports.
//
// It knows nothing of the database. What it reads it returns, parsed, for internal/pull to keep.
package asc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"appstoreanalytics/internal/config"
)

// tokenLifetime is how long a signed token is good for. Apple refuses one that lives longer
// than twenty minutes.
const tokenLifetime = 15 * time.Minute

// Client signs requests to App Store Connect.
type Client struct {
	base   string
	cred   config.Apple
	http   *http.Client
	now    func() time.Time
	mu     sync.Mutex
	token  string
	expiry time.Time
}

// New makes a client for the account the key belongs to.
func New(cred config.Apple) *Client {
	return &Client{
		base: strings.TrimSuffix(cred.BaseURL, "/"),
		cred: cred,
		http: &http.Client{Timeout: 2 * time.Minute},
		now:  time.Now,
	}
}

// Error is a refusal from App Store Connect, with what it said.
type Error struct {
	Status int
	Code   string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("App Store Connect answered %d: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("App Store Connect answered %d", e.Status)
}

// StatusOf is the HTTP status of an App Store Connect refusal, and 0 for any other error.
func StatusOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// Why says in words what a refusal means for whoever reads the status page.
func Why(err error) string {
	switch StatusOf(err) {
	case http.StatusUnauthorized:
		return "App Store Connect refused the key: check the key id, the issuer id and the .p8."
	case http.StatusForbidden:
		return "The key is not allowed to do this. " + detail(err)
	case http.StatusTooManyRequests:
		return "App Store Connect asked to slow down; the next sync picks up where this one stopped."
	case 0:
		if errors.Is(err, context.Canceled) {
			return "Stopped."
		}
		return "App Store Connect could not be reached: " + err.Error()
	}
	return err.Error()
}

func detail(err error) string {
	var e *Error
	if errors.As(err, &e) && e.Detail != "" {
		return e.Detail
	}
	return ""
}

// get reads a JSON:API document. path is either a path under the API or a whole URL, which is
// what a page's next link is.
func (c *Client) get(ctx context.Context, path string, out any) error {
	body, err := c.do(ctx, http.MethodGet, path, nil, "application/json")
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(out)
}

// post sends a JSON:API document and reads the one that comes back.
func (c *Client) post(ctx context.Context, path string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	body, err := c.do(ctx, http.MethodPost, path, payload, "application/json")
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(out)
}

func (c *Client) do(ctx context.Context, method, path string, payload []byte, accept string) (io.ReadCloser, error) {
	url := path
	if !strings.HasPrefix(path, "https://") && !strings.HasPrefix(path, "http://") {
		url = c.base + path
	}
	token, err := c.bearer()
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, refusal(resp)
	}
	return resp.Body, nil
}

// refusal reads Apple's error document, keeping its first error's words.
func refusal(resp *http.Response) error {
	e := &Error{Status: resp.StatusCode}
	var doc struct {
		Errors []struct {
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if json.Unmarshal(raw, &doc) == nil && len(doc.Errors) > 0 {
		e.Code = doc.Errors[0].Code
		e.Detail = doc.Errors[0].Detail
		if e.Detail == "" {
			e.Detail = doc.Errors[0].Title
		}
	}
	return e
}

// bearer is a signed token, made again once it nears its end.
func (c *Client) bearer() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.token != "" && now.Before(c.expiry.Add(-time.Minute)) {
		return c.token, nil
	}
	token, err := sign(c.cred, now)
	if err != nil {
		return "", err
	}
	c.token, c.expiry = token, now.Add(tokenLifetime)
	return token, nil
}

// sign makes the JWT App Store Connect takes: ES256, with the key's id in the header and the
// team's issuer in the claims.
func sign(cred config.Apple, now time.Time) (string, error) {
	header := map[string]string{"alg": "ES256", "kid": cred.KeyID, "typ": "JWT"}
	claims := map[string]any{
		"iss": cred.IssuerID,
		"iat": now.Unix(),
		"exp": now.Add(tokenLifetime).Unix(),
		"aud": "appstoreconnect-v1",
	}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(claims)
	unsigned := b64(h) + "." + b64(p)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, cred.Key, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign the App Store Connect token: %w", err)
	}
	// JWS wants r and s as two 32-byte numbers side by side, not the ASN.1 Go writes by default.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return unsigned + "." + b64(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
