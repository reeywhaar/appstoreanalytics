package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
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
	t.Setenv(VendorNumberEnv, "00000000")
	t.Setenv(PrivateKeyEnv, key)
	t.Setenv(DataDirEnv, "")
	t.Setenv(LogLevelEnv, "")
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
		"no key id":                {KeyIDEnv, "", KeyIDEnv},
		"no vendor number":         {VendorNumberEnv, "", VendorNumberEnv},
		"a vendor name":            {VendorNumberEnv, "Acme Inc", VendorNumberEnv},
		"no key":                   {PrivateKeyEnv, "", PrivateKeyEnv},
		"a key not in base64":      {PrivateKeyEnv, "-----BEGIN PRIVATE KEY-----", "not base64"},
		"base64 of something else": {PrivateKeyEnv, encoded("hello"), "not a .p8"},
		"an unknown level":         {LogLevelEnv, "loud", "not a level"},
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
