// SPDX-FileCopyrightText: Copyright 2025 Carabiner Systems, Inc
// SPDX-License-Identifier: Apache-2.0

package burnafter

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/pbkdf2"

	pb "github.com/carabiner-dev/burnafter/internal/common"
)

const (
	fallbackFileVersion = 1
	pbkdf2Iterations    = 100000
	aesKeySize          = 32 // AES-256
	gcmNonceSize        = 12
)

// fallbackSecretFile represents the structure of an encrypted secret file
// Format: [version:1][nonce:12][expiry:8][ciphertext+tag:variable]
type fallbackSecretFile struct {
	version    byte
	nonce      []byte // GCM nonce
	expiry     int64  // Unix timestamp when secret expires
	ciphertext []byte // Encrypted secret + GCM tag
}

// deriveKey generates an encryption key from client nonce, binary hash, and secret name
func (c *Client) deriveKey(secretName string) ([]byte, error) {
	// Get binary hash
	binaryHash, err := pb.GetCurrentBinaryHash()
	if err != nil {
		return nil, fmt.Errorf("failed to get binary hash: %w", err)
	}

	// Create input for key derivation: nonce + binary hash + secret name
	input := []byte(c.options.Nonce + binaryHash + secretName)

	// Salt is hash of secret name for deterministic but unique per-secret salt
	saltInput := []byte(secretName)
	salt := sha256.Sum256(saltInput)

	// Derive key using PBKDF2
	key := pbkdf2.Key(input, salt[:], pbkdf2Iterations, aesKeySize, sha256.New)

	return key, nil
}

// getFallbackFilePath generates a deterministic file path for a secret
func (c *Client) getFallbackFilePath(secretName string) (string, error) {
	// Get binary hash
	binaryHash, err := pb.GetCurrentBinaryHash()
	if err != nil {
		return "", fmt.Errorf("failed to get binary hash: %w", err)
	}

	// Hash the secret name for the filename
	secretHash := sha256.Sum256([]byte(secretName))

	// Create filename: burnafter-{binary_hash[:16]}-{secret_hash[:16]}
	filename := fmt.Sprintf("burnafter-%s-%x", binaryHash[:16], secretHash[:16])

	// Use system temp directory
	tmpDir := os.TempDir()
	filePath := filepath.Join(tmpDir, filename)

	return filePath, nil
}

// encryptSecret encrypts a secret and writes it to a file
func (c *Client) encryptSecret(secretName string, secret []byte, expiryTime time.Time) error {
	// Derive encryption key
	key, err := c.deriveKey(secretName)
	if err != nil {
		return err
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the secret
	ciphertext := gcm.Seal(nil, nonce, secret, nil)

	// Get file path
	filePath, err := c.getFallbackFilePath(secretName)
	if err != nil {
		return err
	}

	// Create file structure
	file := fallbackSecretFile{
		version:    fallbackFileVersion,
		nonce:      nonce,
		expiry:     expiryTime.Unix(),
		ciphertext: ciphertext,
	}

	// Serialize to bytes
	buf := make([]byte, 1+gcmNonceSize+8+len(ciphertext))
	buf[0] = file.version
	copy(buf[1:], file.nonce)
	// Ensure expiry is non-negative before conversion
	if file.expiry < 0 {
		return fmt.Errorf("invalid expiry time: %d", file.expiry)
	}
	binary.BigEndian.PutUint64(buf[1+gcmNonceSize:], uint64(file.expiry))
	copy(buf[1+gcmNonceSize+8:], file.ciphertext)

	// Write to temp file then rename (atomic)
	tmpFile, err := os.CreateTemp(filepath.Dir(filePath), ".burnafter-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(buf); err != nil {
		tmpFile.Close()    //nolint:errcheck,gosec
		os.Remove(tmpPath) //nolint:errcheck,gosec
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Make file read/write for owner only
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// decryptSecret reads and decrypts a secret from a file
func (c *Client) decryptSecret(secretName string) ([]byte, error) {
	// Get file path
	filePath, err := c.getFallbackFilePath(secretName)
	if err != nil {
		return nil, err
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("secret not found")
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse file structure
	if len(data) < 1+gcmNonceSize+8 {
		return nil, fmt.Errorf("invalid file format: too small")
	}

	version := data[0]
	if version != fallbackFileVersion {
		return nil, fmt.Errorf("unsupported file version: %d", version)
	}

	nonce := data[1 : 1+gcmNonceSize]
	expiryUint := binary.BigEndian.Uint64(data[1+gcmNonceSize : 1+gcmNonceSize+8])
	if expiryUint > math.MaxInt64 {
		return nil, fmt.Errorf("invalid expiry time in file")
	}
	expiry := int64(expiryUint)
	ciphertext := data[1+gcmNonceSize+8:]

	// Check if expired
	if time.Now().Unix() > expiry {
		// Delete expired file
		os.Remove(filePath) //nolint:errcheck,gosec
		return nil, fmt.Errorf("secret expired")
	}

	// Derive encryption key
	key, err := c.deriveKey(secretName)
	if err != nil {
		return nil, err
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// deleteFallbackSecret removes a secret file
func (c *Client) deleteFallbackSecret(secretName string) error {
	filePath, err := c.getFallbackFilePath(secretName)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("secret not found")
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// cleanupExpiredFallbackFiles removes expired secret files
func (c *Client) cleanupExpiredFallbackFiles() error {
	// Get binary hash for filtering our files
	binaryHash, err := pb.GetCurrentBinaryHash()
	if err != nil {
		return err
	}

	prefix := fmt.Sprintf("burnafter-%s-", binaryHash[:16])
	tmpDir := os.TempDir()

	// List files in temp directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read temp directory: %w", err)
	}

	now := time.Now().Unix()

	for _, entry := range entries {
		// Skip if not our file
		if entry.IsDir() || len(entry.Name()) < len(prefix) || entry.Name()[:len(prefix)] != prefix {
			continue
		}

		filePath := filepath.Join(tmpDir, entry.Name())

		// Read and check expiry
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		// Parse expiry time
		if len(data) < 1+gcmNonceSize+8 {
			continue
		}

		expiryUint := binary.BigEndian.Uint64(data[1+gcmNonceSize : 1+gcmNonceSize+8])
		if expiryUint > math.MaxInt64 {
			continue // Skip invalid files
		}
		expiry := int64(expiryUint)

		// Delete if expired
		if now > expiry {
			os.Remove(filePath) //nolint:errcheck,gosec
		}
	}

	return nil
}

// useFallback returns true if we should use fallback storage instead of server
func (c *Client) useFallback() bool {
	return c.options.NoServer || c.serverStartFailed
}
