// Package dkim provides DKIM keypair generation and DNS record builders
// for SPF, DKIM and DMARC — surfaced to customers in the domains console.
package dkim

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// KeyPair holds a PEM-encoded private key and the DNS-ready public value.
type KeyPair struct {
	PrivateKeyPEM string // store encrypted
	DNSRecord     string // value for {selector}._domainkey.{domain} TXT
}

// GenerateRSAKey creates a DKIM keypair. bits is typically 1024 or 2048.
func GenerateRSAKey(selector, domain string, bits int) (*KeyPair, error) {
	if bits < 1024 || bits > 4096 {
		bits = 2048
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generate rsa key: %w", err)
	}
	derBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(derBytes)

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	record := fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubB64)
	return &KeyPair{
		PrivateKeyPEM: string(pemBlock),
		DNSRecord:     record,
	}, nil
}

// PublicRecordFromPEM derives the DKIM DNS TXT value from a PEM private key.
func PublicRecordFromPEM(pemStr string) (string, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return "", fmt.Errorf("invalid PEM")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	derBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", err
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(derBytes), nil
}

// Record describes a single DNS TXT/CNAME entry a customer must publish.
type Record struct {
	Type     string `json:"type"` // TXT | CNAME
	Name     string `json:"name"`
	Value    string `json:"value"`
	Purpose  string `json:"purpose"`
	Verified bool   `json:"verified"`
}

// SPFRecord returns the recommended SPF TXT record for a sending domain.
// include mechanisms can be appended per customer ESP.
func SPFRecord(domain string) Record {
	return Record{
		Type:    "TXT",
		Name:    domain,
		Value:   "v=spf1 include:_spf." + domain + " ip4:0.0.0.0/0 ~all",
		Purpose: "SPF — authorize which servers may send for this domain",
	}
}

// DKIMRecord returns the DKIM TXT record for the given selector.
func DKIMRecord(selector, domain, dnsValue string) Record {
	return Record{
		Type:    "TXT",
		Name:    fmt.Sprintf("%s._domainkey.%s", selector, domain),
		Value:   dnsValue,
		Purpose: "DKIM — cryptographically sign messages so receivers can verify integrity",
	}
}

// DMARCRecord returns the DMARC TXT record with a reporting mailbox.
func DMARCRecord(domain, reportEmail string) Record {
	if reportEmail == "" {
		reportEmail = "dmarc@" + domain
	}
	return Record{
		Type:    "TXT",
		Name:    "_dmarc." + domain,
		Value:   fmt.Sprintf("v=DMARC1; p=quarantine; rua=mailto:%s; adkim=s; aspf=s", reportEmail),
		Purpose: "DMARC — policy + reporting for SPF/DKIM alignment",
	}
}
