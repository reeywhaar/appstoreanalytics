package hooks

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Who a delivery is proven to come from.
const (
	// ByApple is an App Store Server Notification whose signatures lead to Apple's root.
	ByApple = "apple"
	// BySecret is an App Store Connect webhook signed with the secret of the address it came to.
	BySecret = "secret"
)

// Verdict is what checking a delivery found: who it is proven to come from, or why it is not.
// Neither is set for a delivery that carries nothing to check.
type Verdict struct {
	By     string
	Failed string
}

// Proven reports whether a delivery's origin checked out.
func (v Verdict) Proven() bool { return v.By != "" && v.Failed == "" }

// appleRootG3 is the SHA-256 of Apple Root CA - G3, which App Store signatures lead to, as
// https://www.apple.com/certificateauthority/AppleRootCA-G3.cer is. Pinned rather than
// embedded: a chain carries its root, and this says which root it has to be. A var so a test
// can sign with a root of its own.
var appleRootG3 = "63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179"

// The extensions Apple marks its signing certificates with: the leaf's, and the Worldwide
// Developer Relations intermediate's.
var (
	oidLeaf         = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}
	oidIntermediate = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
)

// Check proves a delivery where it can. signature is its x-apple-signature header, and secret
// the one of the address it came to, "" for none.
//
// An App Store Server Notification proves itself, signed down to its transaction. An App Store
// Connect webhook is proven by the secret it was registered with, so to an address without one
// it can only be kept, unproven; to an address with one, one that does not match it, or
// carries no signature at all, has failed.
func Check(body []byte, signature, secret string) Verdict {
	var top map[string]any
	if json.Unmarshal(body, &top) == nil {
		if payload, ok := top["signedPayload"].(string); ok {
			if err := verifyApple(payload, true); err != nil {
				return Verdict{Failed: "Apple's signature does not check out: " + err.Error()}
			}
			return Verdict{By: ByApple}
		}
	}
	switch {
	case signature == "" && secret == "":
		return Verdict{}
	case signature == "":
		return Verdict{Failed: "It carries no signature, and its address has a secret."}
	case secret == "":
		return Verdict{Failed: "It is signed, and its address has no secret to check it with."}
	}
	if !signedWith(body, signature, secret) {
		return Verdict{Failed: "Its signature does not match its address's secret."}
	}
	return Verdict{By: BySecret}
}

// signedWith checks an App Store Connect signature: hmacsha256=, then the hex of an HMAC-SHA256
// of the body keyed with the secret.
func signedWith(body []byte, signature, secret string) bool {
	hexed, ok := strings.CutPrefix(strings.TrimSpace(signature), "hmacsha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(strings.ToLower(hexed))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// verifyApple checks an App Store JWS as Apple's own libraries do without going online: the
// chain in its header, each certificate carrying Apple's mark, leads to Apple's root at the
// moment it says it was signed, and its signature is the leaf's. With nested, the signed
// transaction and renewal inside it are checked the same way.
func verifyApple(jws string, nested bool) error {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return errors.New("it is not a JWS")
	}
	var header struct {
		Alg string   `json:"alg"`
		X5c []string `json:"x5c"`
	}
	if err := decodePart(parts[0], &header); err != nil {
		return fmt.Errorf("its header cannot be read: %w", err)
	}
	if header.Alg != "ES256" {
		return fmt.Errorf("it is signed with %q, not ES256", header.Alg)
	}
	if len(header.X5c) != 3 {
		return fmt.Errorf("its chain has %d certificates, not 3", len(header.X5c))
	}
	certs := make([]*x509.Certificate, 3)
	for i, raw := range header.X5c {
		der, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("certificate %d is not base64", i)
		}
		if certs[i], err = x509.ParseCertificate(der); err != nil {
			return fmt.Errorf("certificate %d cannot be read: %w", i, err)
		}
	}
	leaf, intermediate, root := certs[0], certs[1], certs[2]
	if sum := sha256.Sum256(root.Raw); hex.EncodeToString(sum[:]) != appleRootG3 {
		return errors.New("its chain does not lead to Apple's root")
	}
	if !marked(leaf, oidLeaf) || !marked(intermediate, oidIntermediate) {
		return errors.New("its certificates do not carry Apple's marks")
	}

	var claims map[string]any
	if err := decodePart(parts[1], &claims); err != nil {
		return fmt.Errorf("its payload cannot be read: %w", err)
	}
	at := time.Now()
	if ms, ok := claims["signedDate"].(float64); ok {
		at = time.UnixMilli(int64(ms))
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(intermediate)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   at,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return fmt.Errorf("its chain does not verify: %w", err)
	}

	key, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("its signing certificate holds no EC key")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("its signature is not an ES256 one")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return errors.New("its signature is not its certificate's")
	}

	if nested {
		data, _ := claims["data"].(map[string]any)
		for _, key := range []string{"signedTransactionInfo", "signedRenewalInfo"} {
			if inner, ok := data[key].(string); ok {
				if err := verifyApple(inner, false); err != nil {
					return fmt.Errorf("its %s: %w", key, err)
				}
			}
		}
	}
	return nil
}

func marked(cert *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oid) {
			return true
		}
	}
	return false
}

func decodePart(part string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
