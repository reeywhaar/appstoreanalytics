package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/netip"
	"strings"
	"testing"
)

func p8(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), key
}

func encoded(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }

func environment(t *testing.T, key string) {
	t.Helper()
	t.Setenv(PublicURLEnv, "https://Stats.Example.com/")
	t.Setenv(UsernameEnv, "misha")
	t.Setenv(PasswordEnv, "a good password")
	t.Setenv(IssuerIDEnv, "00000000-0000-0000-0000-000000000000")
	t.Setenv(KeyIDEnv, "TESTKEY000")
	t.Setenv(PrivateKeyEnv, key)
	t.Setenv(VendorNumberEnv, "")
	t.Setenv(DataDirEnv, "")
	t.Setenv(LogLevelEnv, "")
	t.Setenv(IAPKeyIDEnv, "")
	t.Setenv(IAPPrivateKeyEnv, "")
	t.Setenv(BackupURLEnv, "")
	t.Setenv(TrustedProxiesEnv, "")
}

func TestAWholeEnvironmentLoads(t *testing.T) {
	text, key := p8(t)
	environment(t, encoded(text))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL.String() != "https://stats.example.com" || !cfg.Secure {
		t.Errorf("public url = %s, secure = %v", cfg.PublicURL, cfg.Secure)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("data dir = %q", cfg.DataDir)
	}
	if !cfg.Apple.Key.Equal(key) {
		t.Error("the key read is not the key given")
	}
	if cfg.Apple.BaseURL == "" {
		t.Error("no App Store Connect address")
	}
	if !cfg.TrustedProxies.Trusts(netip.MustParseAddr("172.18.0.5")) || cfg.TrustedProxies.Trusts(netip.MustParseAddr("203.0.113.7")) {
		t.Errorf("trusted proxies = %v, want the private ranges", cfg.TrustedProxies)
	}
}

func TestTrustedProxies(t *testing.T) {
	for _, c := range []struct {
		raw       string
		trusted   []string
		strangers []string
	}{
		{"private", []string{"10.1.2.3", "172.18.0.5", "192.168.1.1", "127.0.0.1", "::1", "fd00::1", "::ffff:10.1.2.3"}, []string{"203.0.113.7", "100.64.0.1", "2001:db8::1"}},
		{"none", nil, []string{"127.0.0.1", "172.18.0.5"}},
		{"198.51.100.3", []string{"198.51.100.3"}, []string{"198.51.100.4", "172.18.0.5"}},
		{"private, 198.51.100.0/24 2001:db8::/32", []string{"172.18.0.5", "198.51.100.9", "2001:db8::9"}, []string{"203.0.113.7"}},
		{"100.64.0.0/10", []string{"100.100.1.1"}, []string{"10.0.0.1"}},
	} {
		p, err := ParseProxies(c.raw)
		if err != nil {
			t.Errorf("%q: %v", c.raw, err)
			continue
		}
		for _, a := range c.trusted {
			if !p.Trusts(netip.MustParseAddr(a)) {
				t.Errorf("%q does not trust %s", c.raw, a)
			}
		}
		for _, a := range c.strangers {
			if p.Trusts(netip.MustParseAddr(a)) {
				t.Errorf("%q trusts %s", c.raw, a)
			}
		}
	}
}

// base64 of the file is what the variable holds, and base64 of the key without its BEGIN and
// END lines is the same key.
func TestAKeyIsReadFromBase64(t *testing.T) {
	text, key := p8(t)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for name, given := range map[string]string{
		"the file":            base64.StdEncoding.EncodeToString([]byte(text)),
		"the file, wrapped":   wrap(base64.StdEncoding.EncodeToString([]byte(text))),
		"the key's own lines": strings.Join(lines[1:len(lines)-1], "\n"),
	} {
		got, err := ParseKey(given)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !got.Equal(key) {
			t.Errorf("%s: read a different key", name)
		}
	}
}

// wrap breaks a line every 76 characters, as base64 does on some systems.
func wrap(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\n")
		s = s[76:]
	}
	b.WriteString(s)
	return b.String()
}

func TestStartingIsRefusedWithAReason(t *testing.T) {
	text, _ := p8(t)
	text = encoded(text)
	for name, c := range map[string]struct {
		env  string
		val  string
		says string
	}{
		"no address":               {PublicURLEnv, "", PublicURLEnv},
		"an address with a path":   {PublicURLEnv, "https://example.com/stats", "path"},
		"no username":              {UsernameEnv, "", UsernameEnv},
		"no password":              {PasswordEnv, "", PasswordEnv},
		"a short password":         {PasswordEnv, "short", "at least 8"},
		"no issuer":                {IssuerIDEnv, "", IssuerIDEnv},
		"a vendor name":            {VendorNumberEnv, "Acme Inc", VendorNumberEnv},
		"no key id":                {KeyIDEnv, "", KeyIDEnv},
		"no key":                   {PrivateKeyEnv, "", PrivateKeyEnv},
		"a key not in base64":      {PrivateKeyEnv, "-----BEGIN PRIVATE KEY-----", "not base64"},
		"base64 of something else": {PrivateKeyEnv, encoded("hello"), "not a .p8"},
		"an unknown level":         {LogLevelEnv, "loud", "not a level"},
		"an IAP key id alone":      {IAPKeyIDEnv, "IAPKEY0000", IAPPrivateKeyEnv},
		"an IAP key alone":         {IAPPrivateKeyEnv, text, IAPKeyIDEnv},
		"a proxy by its name":      {TrustedProxiesEnv, "proxy.example.com", "not an address"},
		"a range too wide":         {TrustedProxiesEnv, "10.0.0.0/33", "not a range"},
		"none with another":        {TrustedProxiesEnv, "none, 10.0.0.1", "goes alone"},
	} {
		environment(t, text)
		t.Setenv(c.env, c.val)
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v, want one saying %q", name, err, c.says)
		}
	}
}

func TestAPasswordKeepsItsSpaces(t *testing.T) {
	text, _ := p8(t)
	environment(t, encoded(text))
	t.Setenv(PasswordEnv, " spaced out ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != " spaced out " {
		t.Errorf("password = %q", cfg.Password)
	}
}

// An In-App Purchase key is optional, and read like the team key when it is given.
func TestAnInAppPurchaseKeyIsOptional(t *testing.T) {
	text, _ := p8(t)
	environment(t, encoded(text))
	cfg, err := Load()
	if err != nil || cfg.Apple.IAPKey != nil || cfg.Apple.StoreKitURL == "" || cfg.Apple.StoreKitSandboxURL == "" {
		t.Fatalf("without one: %v, %+v", err, cfg)
	}
	iap, key := p8(t)
	t.Setenv(IAPKeyIDEnv, "IAPKEY0000")
	t.Setenv(IAPPrivateKeyEnv, encoded(iap))
	cfg, err = Load()
	if err != nil || !cfg.Apple.IAPKey.Equal(key) || cfg.Apple.IAPKeyID != "IAPKEY0000" {
		t.Fatalf("with one: %v", err)
	}
}
