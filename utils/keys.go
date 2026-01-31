package utils

import (
	"crypto/rsa"
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
