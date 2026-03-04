package utils

import (
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// LoadRSAPrivateKey загружает только приватный ключ
func LoadRSAPrivateKey(keyStr string) (*rsa.PrivateKey, error) {
	if keyStr == "" {
		return nil, fmt.Errorf("RSA_PRIVATE_KEY is empty or not set in environment")
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(keyStr))
	if err != nil {
		return nil, fmt.Errorf("parse private key from env: %w", err)
	}

	return key, nil
}

// LoadRSAPrivateKeyFromPath загружает только приватный ключ
func LoadRSAPrivateKeyFromPath(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return key, nil
}

// LoadRSAPublicKey загружает только публичный ключ
func LoadRSAPublicKey(keyStr string) (*rsa.PublicKey, error) {
	if keyStr == "" {
		return nil, fmt.Errorf("RSA_PUBLIC_KEY is empty or not set in environment")
	}

	// 2. Parse the public key from the byte slice
	// This specifically looks for the -----BEGIN PUBLIC KEY----- block
	key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(keyStr))
	if err != nil {
		return nil, fmt.Errorf("parse public key from env: %w", err)
	}

	return key, nil
}

// LoadRSAPublicKeyFromPath загружает только публичный ключ
func LoadRSAPublicKeyFromPath(path string) (*rsa.PublicKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	return key, nil
}

// LoadEd25519PrivateKey decodes a base64 string into a usable private key.
func LoadEd25519PrivateKey(base64Key string) (ed25519.PrivateKey, error) {
	if base64Key == "" {
		return nil, fmt.Errorf("ed25519 private key is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %w", err)
	}

	// Ed25519 private keys must be exactly 64 bytes long
	if len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid ed25519 private key length: expected %d, got %d", ed25519.PrivateKeySize, len(decoded))
	}

	return ed25519.PrivateKey(decoded), nil
}

// LoadEd25519PublicKey is the equivalent for the public key (needs to be exactly 32 bytes).
func LoadEd25519PublicKey(base64Key string) (ed25519.PublicKey, error) {
	if base64Key == "" {
		return nil, fmt.Errorf("ed25519 public key is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %w", err)
	}

	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length: expected %d, got %d", ed25519.PublicKeySize, len(decoded))
	}

	return ed25519.PublicKey(decoded), nil
}
