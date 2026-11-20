package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files the stand-in keeps in its directory, which the demo shares with appstoreanalytics: a
// certificate authority to trust, and the API key to sign with, as App Store Connect hands a
// .p8 out once.
const (
	caFile     = "ca.pem"
	caKeyFile  = "ca-key.pem"
	certFile   = "cert.pem"
	keyFile    = "cert-key.pem"
	apiKeyFile = "AuthKey.p8"
	// apiKeyB64File is the .p8 as the environment takes it.
	apiKeyB64File = "AuthKey.p8.b64"
)

// Keys are what the stand-in serves and checks with.
type Keys struct {
	TLS tls.Certificate
	// API is the key appstoreanalytics signs its tokens with.
	API *ecdsa.PrivateKey
}

// ensureKeys reads the keys kept in dir, making them the first time: a certificate authority,
// a certificate from it for the names the stand-in answers to, and an API key.
func ensureKeys(dir string, names []string) (*Keys, error) {
	if _, err := os.Stat(filepath.Join(dir, apiKeyB64File)); errors.Is(err, os.ErrNotExist) {
		if err := makeKeys(dir, names); err != nil {
			return nil, err
		}
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, certFile), filepath.Join(dir, keyFile))
	if err != nil {
		return nil, fmt.Errorf("read the certificate: %w", err)
	}
	api, err := readAPIKey(dir)
	if err != nil {
		return nil, err
	}
	return &Keys{TLS: cert, API: api}, nil
}

func readAPIKey(dir string) (*ecdsa.PrivateKey, error) {
	raw, err := os.ReadFile(filepath.Join(dir, apiKeyFile))
	if err != nil {
		return nil, fmt.Errorf("read the API key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("the API key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("read the API key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("the API key is not an EC key")
	}
	return key, nil
}

func makeKeys(dir string, names []string) error {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	ca := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "appstoreanalytics demo authority"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("make the certificate authority: %w", err)
	}
	ca, _ = x509.ParseCertificate(caDER)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leaf := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: names[0]},
		DNSNames:     names,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("make the certificate: %w", err)
	}

	api, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	apiDER, err := x509.MarshalPKCS8PrivateKey(api)
	if err != nil {
		return err
	}
	p8 := pemOf("PRIVATE KEY", apiDER)

	caKeyDER, _ := x509.MarshalPKCS8PrivateKey(caKey)
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	// The base64 last: it is what says the rest are there.
	for _, f := range []struct {
		name string
		data []byte
	}{
		{caFile, pemOf("CERTIFICATE", caDER)},
		{caKeyFile, pemOf("PRIVATE KEY", caKeyDER)},
		{certFile, pemOf("CERTIFICATE", leafDER)},
		{keyFile, pemOf("PRIVATE KEY", keyDER)},
		{apiKeyFile, p8},
		{apiKeyB64File, []byte(base64.StdEncoding.EncodeToString(p8))},
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), f.data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func pemOf(kind string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der})
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	return n
}

// signToken makes a token as App Store Connect takes it: ES256, the key's id in the header,
// the team's issuer in the claims.
func signToken(issuer, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error) {
	h, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID, "typ": "JWT"})
	p, _ := json.Marshal(map[string]any{
		"iss": issuer, "iat": now.Unix(), "exp": now.Add(15 * time.Minute).Unix(), "aud": "appstoreconnect-v1",
	})
	unsigned := b64(h) + "." + b64(p)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return unsigned + "." + b64(sig), nil
}

// checkToken says why App Store Connect would refuse a token, or nil when it takes it.
func checkToken(token, issuer, keyID string, key *ecdsa.PublicKey, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("not a JWT")
	}
	var header struct{ Alg, Kid string }
	var claims struct {
		Iss string
		Aud string
		Exp int64
	}
	if decodeJSON(parts[0], &header) != nil || decodeJSON(parts[1], &claims) != nil {
		return errors.New("not a JWT")
	}
	if header.Alg != "ES256" || header.Kid != keyID {
		return errors.New("signed by another key")
	}
	if claims.Iss != issuer || claims.Aud != "appstoreconnect-v1" {
		return errors.New("for another team")
	}
	if now.Unix() > claims.Exp || claims.Exp > now.Add(20*time.Minute).Unix() {
		return errors.New("expired, or living longer than twenty minutes")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("not signed")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return errors.New("the signature does not match the key")
	}
	return nil
}

func decodeJSON(part string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
