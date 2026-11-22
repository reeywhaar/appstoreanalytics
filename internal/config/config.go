// Package config is the environment, parsed once. See README.md.
package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strings"

	"appstoreanalytics/internal/app"
)

// Environment variables read by this package.
const (
	// PublicURLEnv is the address a browser opens. It decides whether the session cookie is
	// Secure, and is never inferred from a request, whose Host a client writes.
	PublicURLEnv = "APPSTOREANALYTICS_PUBLIC_URL"

	// DataDirEnv is where appstoreanalytics.db lives.
	DataDirEnv = "APPSTOREANALYTICS_DATA_DIR"

	// LogLevelEnv sets the slog level: debug, info, warn or error.
	LogLevelEnv = "APPSTOREANALYTICS_LOG_LEVEL"

	// UsernameEnv and PasswordEnv are the one sign-in there is.
	UsernameEnv = "APPSTOREANALYTICS_USERNAME"
	PasswordEnv = "APPSTOREANALYTICS_PASSWORD"

	// IssuerIDEnv is the team's issuer, shown above the team keys in App Store Connect.
	IssuerIDEnv = "APPSTOREANALYTICS_ISSUER_ID"

	// KeyIDEnv is the API key's id, the XXXXXXXXXX of AuthKey_XXXXXXXXXX.p8.
	KeyIDEnv = "APPSTOREANALYTICS_KEY_ID"

	// VendorNumberEnv names the account whose sales reports fill in what analytics reports have
	// not said yet. Optional: without it there is no filling in. Payments and Financial Reports
	// shows it.
	VendorNumberEnv = "APPSTOREANALYTICS_VENDOR_NUMBER"

	// PrivateKeyEnv is the .p8 in base64, which fits on one line of an .env file:
	// `base64 -i AuthKey_XXXXXXXXXX.p8`.
	PrivateKeyEnv = "APPSTOREANALYTICS_PRIVATE_KEY_BASE64"

	// BackupURLEnv is a backio agent's POST endpoint, the whole address rather than a host: the
	// path is the agent's to choose. Unset, nothing is backed up.
	BackupURLEnv = "APPSTOREANALYTICS_BACKUP_URL"

	// IAPKeyIDEnv and IAPPrivateKeyEnv are an In-App Purchase key, which the App Store Server
	// API wants where App Store Connect's takes the team key: only for asking Apple to send a
	// test notification, and so optional, both or neither.
	IAPKeyIDEnv      = "APPSTOREANALYTICS_IAP_KEY_ID"
	IAPPrivateKeyEnv = "APPSTOREANALYTICS_IAP_PRIVATE_KEY_BASE64"

	// TrustedProxiesEnv is whose X-Forwarded-For is believed: addresses and ranges, "private"
	// for the private and loopback ranges, or "none". Unset, "private".
	TrustedProxiesEnv = "APPSTOREANALYTICS_TRUSTED_PROXIES"
)

// Defaults for everything that has one.
const DefaultDataDir = "/data"

// Config is everything the process was told at startup.
type Config struct {
	// PublicURL is normalized: lowercased, no trailing slash, no path, query or fragment.
	PublicURL *url.URL

	DataDir  string
	LogLevel slog.Level

	// Secure is PublicURL being https, derived once rather than at each Set-Cookie.
	Secure bool

	Username string
	Password string

	// BackupURL is where an archive of the database is posted, or empty for no backups at all.
	BackupURL string

	// TrustedProxies are the peers whose X-Forwarded-For says who the caller is. Empty, nobody's.
	TrustedProxies Proxies

	Apple Apple
}

// Proxies are the addresses a request may come through and still name its caller.
type Proxies []netip.Prefix

// Trusts reports whether addr is one of them.
func (p Proxies) Trusts(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// private is what "private" stands for: where a proxy on the same host or Docker network
// connects from.
var private = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7", "127.0.0.0/8", "::1/128"}

// ParseProxies reads a list of addresses, ranges, "private" and "none", split by commas or spaces.
func ParseProxies(raw string) (Proxies, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	var out Proxies
	for _, f := range fields {
		switch {
		case f == "none":
			if len(fields) > 1 {
				return nil, fmt.Errorf("%s: \"none\" goes alone, not with %q", TrustedProxiesEnv, raw)
			}
			return Proxies{}, nil
		case f == "private":
			for _, r := range private {
				out = append(out, netip.MustParsePrefix(r))
			}
		case strings.Contains(f, "/"):
			prefix, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not a range", TrustedProxiesEnv, f)
			}
			out = append(out, prefix.Masked())
		default:
			addr, err := netip.ParseAddr(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not an address, a range, \"private\" or \"none\"", TrustedProxiesEnv, f)
			}
			addr = addr.Unmap()
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return out, nil
}

// Apple is what signs in to App Store Connect.
type Apple struct {
	// BaseURL is App Store Connect, or a stand-in for it in tests.
	BaseURL  string
	IssuerID string
	KeyID    string
	Key      *ecdsa.PrivateKey
	// VendorNumber is "" when sales reports are not read.
	VendorNumber string

	// StoreKitURL and StoreKitSandboxURL are the App Store Server API, or stand-ins in tests.
	StoreKitURL        string
	StoreKitSandboxURL string
	// IAPKeyID and IAPKey are the In-App Purchase key, nil when none was given.
	IAPKeyID string
	IAPKey   *ecdsa.PrivateKey
}

// Load reads the environment. Errors are written for somebody looking at a container that
// refused to start.
func Load() (*Config, error) {
	raw := strings.TrimSpace(os.Getenv(PublicURLEnv))
	if raw == "" {
		return nil, fmt.Errorf("%s is required: set it to the address you open in a browser, e.g. https://stats.example.com", PublicURLEnv)
	}
	public, err := parsePublicURL(raw)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		PublicURL: public,
		DataDir:   DefaultDataDir,
		LogLevel:  slog.LevelInfo,
		Secure:    public.Scheme == "https",
		// Untrimmed: a password may begin or end with a space, and whoever set it meant it.
		Username: strings.TrimSpace(os.Getenv(UsernameEnv)),
		Password: os.Getenv(PasswordEnv),
		Apple: Apple{
			BaseURL:            app.AppleAPI,
			IssuerID:           strings.TrimSpace(os.Getenv(IssuerIDEnv)),
			KeyID:              strings.TrimSpace(os.Getenv(KeyIDEnv)),
			VendorNumber:       strings.TrimSpace(os.Getenv(VendorNumberEnv)),
			StoreKitURL:        app.StoreKitAPI,
			StoreKitSandboxURL: app.StoreKitSandboxAPI,
			IAPKeyID:           strings.TrimSpace(os.Getenv(IAPKeyIDEnv)),
		},
	}
	if dir := strings.TrimSpace(os.Getenv(DataDirEnv)); dir != "" {
		cfg.DataDir = dir
	}
	cfg.BackupURL = strings.TrimSpace(os.Getenv(BackupURLEnv))
	proxies := strings.TrimSpace(os.Getenv(TrustedProxiesEnv))
	if proxies == "" {
		proxies = "private"
	}
	if cfg.TrustedProxies, err = ParseProxies(proxies); err != nil {
		return nil, err
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("%s is required: it is the name you sign in with", UsernameEnv)
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("%s is required: it is the password you sign in with", PasswordEnv)
	}
	if len(cfg.Password) < 8 {
		return nil, fmt.Errorf("%s is %d characters, and it has to be at least 8", PasswordEnv, len(cfg.Password))
	}
	if cfg.Apple.IssuerID == "" {
		return nil, fmt.Errorf("%s is required: it is shown above the list of team keys, in Users and Access, Integrations, App Store Connect API", IssuerIDEnv)
	}
	if v := cfg.Apple.VendorNumber; v != "" && strings.Trim(v, "0123456789") != "" {
		return nil, fmt.Errorf("%s is %q, and it is a number: the one at the top of Payments and Financial Reports in App Store Connect", VendorNumberEnv, v)
	}
	if cfg.Apple.KeyID == "" {
		return nil, fmt.Errorf("%s is required: it is the id of the App Store Connect API key, the XXXXXXXXXX of AuthKey_XXXXXXXXXX.p8", KeyIDEnv)
	}
	raw = strings.TrimSpace(os.Getenv(PrivateKeyEnv))
	if raw == "" {
		return nil, fmt.Errorf("%s is required: the App Store Connect API key in base64, made with `base64 -i AuthKey_XXXXXXXXXX.p8`", PrivateKeyEnv)
	}
	if cfg.Apple.Key, err = ParseKey(raw); err != nil {
		return nil, fmt.Errorf("%s %w", PrivateKeyEnv, err)
	}
	raw = strings.TrimSpace(os.Getenv(IAPPrivateKeyEnv))
	switch {
	case raw == "" && cfg.Apple.IAPKeyID == "":
	case raw == "" || cfg.Apple.IAPKeyID == "":
		return nil, fmt.Errorf("%s and %s come together: an In-App Purchase key's id, the XXXXXXXXXX of SubscriptionKey_XXXXXXXXXX.p8, and the .p8 in base64", IAPKeyIDEnv, IAPPrivateKeyEnv)
	default:
		if cfg.Apple.IAPKey, err = ParseKey(raw); err != nil {
			return nil, fmt.Errorf("%s %w", IAPPrivateKeyEnv, err)
		}
	}

	// A startup error rather than a fall back to info: a level that quietly works is one
	// nobody finds until the log lacks what they came for.
	if v := strings.TrimSpace(os.Getenv(LogLevelEnv)); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(strings.ToLower(v))); err != nil {
			return nil, fmt.Errorf("%s: %q is not a level (debug, info, warn, error)", LogLevelEnv, v)
		}
	}
	return cfg, nil
}

// ParseKey reads an App Store Connect key from base64: of the .p8 file, as `base64` writes it,
// or of what lies between its BEGIN and END lines, which is the same key without them.
func ParseKey(text string) (*ecdsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(text), ""))
	if err != nil {
		return nil, fmt.Errorf("is not base64: make it with `base64 -i AuthKey_XXXXXXXXXX.p8`")
	}
	if block, _ := pem.Decode(der); block != nil {
		der = block.Bytes
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("is not a .p8 from App Store Connect in base64: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("is not a P-256 key, which a .p8 from App Store Connect is")
	}
	return key, nil
}

// parsePublicURL validates and normalizes.
//
// A trailing slash is trimmed: it is the commonest way to write one down. A path is refused —
// this serves from the root, and a prefix would produce links that half work.
func parsePublicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %q is not a URL: %w", PublicURLEnv, raw, err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%s: %q needs an http:// or https:// scheme", PublicURLEnv, raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%s: %q names no host", PublicURLEnv, raw)
	}
	u.Host = strings.ToLower(u.Host)
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%s: %q should be a scheme and a host, with no query or fragment", PublicURLEnv, raw)
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		return nil, fmt.Errorf("%s: %q has a path in it, and this serves from the root", PublicURLEnv, raw)
	}
	u.Path = ""
	u.User = nil
	return u, nil
}
