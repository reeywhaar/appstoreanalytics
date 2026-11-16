package hooks

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// Apple's own example: secret "This is my secret", body "Hello, World!".
func TestAnAppStoreConnectSignatureIsCheckedAgainstTheSecret(t *testing.T) {
	body := []byte("Hello, World!")
	good := "hmacsha256=7f062172b01cb00b53ca068614674a3d982a34062a0f5d37687d5e3377e54657"
	for name, c := range map[string]struct {
		signature, secret string
		want              Verdict
	}{
		"signed with the secret":       {good, "This is my secret", Verdict{By: BySecret}},
		"in upper case":                {strings.ToUpper(good[:11]) + strings.ToUpper(good[11:]), "This is my secret", Verdict{Failed: "Its signature does not match its address's secret."}},
		"signed with another":          {good, "Another secret", Verdict{Failed: "Its signature does not match its address's secret."}},
		"signed, and no secret here":   {good, "", Verdict{Failed: "It is signed, and its address has no secret to check it with."}},
		"unsigned, and a secret here":  {"", "This is my secret", Verdict{Failed: "It carries no signature, and its address has a secret."}},
		"unsigned, and no secret here": {"", "", Verdict{}},
	} {
		if got := Check(body, c.signature, c.secret); got != c.want {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
	if !signedWith(body, "hmacsha256="+strings.ToUpper(good[11:]), "This is my secret") {
		t.Error("hex in upper case was refused")
	}
}

// chain stands in for Apple's: a root, an intermediate and a leaf, marked as Apple marks them.
type chain struct {
	root, intermediate, leaf *x509.Certificate
	leafKey                  *ecdsa.PrivateKey
}

func newChain(t *testing.T, leafMarked bool, notAfter time.Time) chain {
	t.Helper()
	mark := func(oid asn1.ObjectIdentifier) []pkix.Extension {
		return []pkix.Extension{{Id: oid, Value: []byte{0x05, 0x00}}}
	}
	make := func(name string, ca bool, ext []pkix.Extension, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(time.Now().UnixNano()),
			Subject:               pkix.Name{CommonName: name},
			NotBefore:             time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			NotAfter:              notAfter,
			BasicConstraintsValid: true,
			IsCA:                  ca,
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			ExtraExtensions:       ext,
		}
		if parent == nil {
			parent, parentKey = tmpl, key
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		return cert, key
	}
	root, rootKey := make("Apple Root CA - G3 (a stand-in)", true, nil, nil, nil)
	intermediate, intermediateKey := make("WWDR (a stand-in)", true, mark(oidIntermediate), root, rootKey)
	var leafExt []pkix.Extension
	if leafMarked {
		leafExt = mark(oidLeaf)
	}
	leaf, leafKey := make("Receipt Signing (a stand-in)", false, leafExt, intermediate, intermediateKey)
	return chain{root, intermediate, leaf, leafKey}
}

// trust makes the chain's root the one pinned, for the length of the test.
func trust(t *testing.T, c chain) {
	t.Helper()
	sum := sha256.Sum256(c.root.Raw)
	previous := appleRootG3
	appleRootG3 = hex.EncodeToString(sum[:])
	t.Cleanup(func() { appleRootG3 = previous })
}

func (c chain) sign(t *testing.T, claims any) string {
	t.Helper()
	x5c := []string{}
	for _, cert := range []*x509.Certificate{c.leaf, c.intermediate, c.root} {
		x5c = append(x5c, base64.StdEncoding.EncodeToString(cert.Raw))
	}
	header, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": x5c})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, c.leafKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func notification(t *testing.T, c chain, signedAt time.Time, transaction string) []byte {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"signedPayload": c.sign(t, map[string]any{
		"notificationType": "SUBSCRIBED",
		"signedDate":       signedAt.UnixMilli(),
		"data":             map[string]any{"bundleId": "com.acme.dots", "signedTransactionInfo": transaction},
	})})
	return body
}

func TestAServerNotificationIsProvenByAppleChain(t *testing.T) {
	c := newChain(t, true, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	trust(t, c)
	at := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	tx := c.sign(t, map[string]any{"productId": "dots.pro", "signedDate": at.UnixMilli()})

	if got := Check(notification(t, c, at, tx), "", ""); got != (Verdict{By: ByApple}) {
		t.Fatalf("a good one = %+v", got)
	}
	// Apple sends a notification with no secret to sign it with, whatever the address it comes to
	// has: its own signature is what proves it.
	if got := Check(notification(t, c, at, tx), "", "This is my secret"); got != (Verdict{By: ByApple}) {
		t.Errorf("a good one to an address with a secret = %+v", got)
	}

	failed := func(name string, body []byte, says string) {
		t.Helper()
		got := Check(body, "", "")
		if got.Proven() || !strings.Contains(got.Failed, says) {
			t.Errorf("%s = %+v, want it to fail saying %q", name, got, says)
		}
	}

	// A payload changed after signing.
	body := notification(t, c, at, tx)
	var doc map[string]string
	json.Unmarshal(body, &doc)
	parts := strings.Split(doc["signedPayload"], ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"notificationType":"REFUND"}`))
	forged, _ := json.Marshal(map[string]string{"signedPayload": strings.Join(parts, ".")})
	failed("a changed payload", forged, "not its certificate's")

	// A transaction inside changed after signing.
	txParts := strings.Split(tx, ".")
	txParts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"productId":"free"}`))
	failed("a changed transaction", notification(t, c, at, strings.Join(txParts, ".")), "signedTransactionInfo")

	// A chain to a root that is not Apple's.
	other := newChain(t, true, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	failed("another root", notification(t, other, at, other.sign(t, map[string]any{})), "Apple's root")

	// A leaf without Apple's mark.
	unmarked := newChain(t, false, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	trust(t, unmarked)
	failed("an unmarked leaf", notification(t, unmarked, at, ""), "marks")

	// Signed after its certificates lapsed.
	lapsed := newChain(t, true, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	trust(t, lapsed)
	failed("signed after the chain lapsed", notification(t, lapsed, at, ""), "does not verify")
}

// A notification is not proven by the secret: it proves itself or fails.
func TestASecretDoesNotProveAServerNotification(t *testing.T) {
	body := []byte(`{"signedPayload":"eyJhbGciOiJub25lIn0.e30."}`)
	if got := Check(body, "hmacsha256=00", "This is my secret"); got.Proven() {
		t.Errorf("= %+v", got)
	}
}
