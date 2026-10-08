package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// Encryptor provides AES-256-GCM encryption for secrets at rest
// (SMTP passwords, DKIM private keys).
type Encryptor struct {
	aead cipher.AEAD
}

// NewEncryptor builds an Encryptor from a 64-char hex key (32 bytes).
// An empty key disables encryption support (callers must handle nil).
func NewEncryptor(hexKey string) (*Encryptor, error) {
	if hexKey == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, errors.New("encryption key must be hex")
	}
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes (64 hex chars)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{aead: aead}, nil
}

// Seal encrypts plaintext and returns nonce||ciphertext.
func (e *Encryptor) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return e.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts a Seal output produced by this key.
func (e *Encryptor) Open(sealed []byte) (string, error) {
	ns := e.aead.NonceSize()
	if len(sealed) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := sealed[:ns], sealed[ns:]
	plain, err := e.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
