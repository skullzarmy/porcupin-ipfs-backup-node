// Package api provides the REST API server for remote access to Porcupin.
package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	// TokenPrefix is the prefix for all Porcupin API tokens
	TokenPrefix = "prcpn_"

	// TokenLength is the total length of the token including prefix
	// prcpn_ (6) + 42 random = 48 total
	TokenLength = 48

	// tokenRandomBytes is the number of random bytes to generate
	// 32 bytes = 256 bits of entropy
	tokenRandomBytes = 32

	// TokenFileName is the name of the token hash file
	TokenFileName = ".api-token-hash"

	// TokenFileMode is the file permission for the token file (owner read/write only)
	TokenFileMode = 0600

	// bcryptCost is the cost factor for bcrypt hashing
	bcryptCost = 10
)

// base62Alphabet is the character set for token encoding (alphanumeric only)
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateToken generates a new secure API token.
// Returns a token in the format: prcpn_<42 base62 chars>
func GenerateToken() (string, error) {
	// Generate random bytes
	randomBytes := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base62 (alphanumeric only, no special chars)
	encoded := encodeBase62(randomBytes)

	// Trim or pad to exact length needed
	// We want 48 total: 6 for prefix + 42 for random
	randomPart := encoded
	if len(randomPart) > 42 {
		randomPart = randomPart[:42]
	}

	return TokenPrefix + randomPart, nil
}

// encodeBase62 encodes bytes to a base62 string (0-9, A-Z, a-z)
func encodeBase62(data []byte) string {
	// Use base64 as intermediate, then convert
	b64 := base64.RawStdEncoding.EncodeToString(data)

	// Replace non-alphanumeric chars with deterministic alphanumeric
	var result strings.Builder
	result.Grow(len(b64))

	for _, c := range b64 {
		switch c {
		case '+':
			result.WriteRune('A')
		case '/':
			result.WriteRune('B')
		case '=':
			// Skip padding
		default:
			result.WriteRune(c)
		}
	}

	return result.String()
}

// HashToken creates a bcrypt hash of the token for secure storage.
// The plain token should NEVER be stored - only the hash.
func HashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash token: %w", err)
	}
	return string(hash), nil
}

// ValidateTokenAgainstHash checks if the provided token matches the stored hash.
// Uses bcrypt's constant-time comparison internally.
func ValidateTokenAgainstHash(provided, hash string) bool {
	if provided == "" || hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(provided))
	return err == nil
}

// ValidateToken checks if the provided token matches the expected token.
// Uses constant-time comparison to prevent timing attacks.
// This is used when the expected token comes from env var (not hashed).
func ValidateToken(provided, expected string) bool {
	if provided == "" {
		provided = "dummy_token_for_timing_safety_padding"
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// ValidateTokenFormat checks if a token has the correct format
// without comparing it to an expected value.
func ValidateTokenFormat(token string) bool {
	if len(token) != TokenLength {
		return false
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}

	// Check that the rest is alphanumeric
	randomPart := token[len(TokenPrefix):]
	for _, c := range randomPart {
		if !isAlphanumeric(c) {
			return false
		}
	}

	return true
}

// isAlphanumeric checks if a rune is alphanumeric
func isAlphanumeric(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z')
}

// TokenFile handles reading and writing the API token file
type TokenFile struct {
	path string
}

// NewTokenFile creates a new TokenFile for the given data directory
func NewTokenFile(dataDir string) *TokenFile {
	return &TokenFile{
		path: filepath.Join(dataDir, TokenFileName),
	}
}

// Path returns the full path to the token file
func (tf *TokenFile) Path() string {
	return tf.path
}

// Exists checks if the token file exists
func (tf *TokenFile) Exists() bool {
	_, err := os.Stat(tf.path)
	return err == nil
}

// ReadHash reads the token hash from the file
func (tf *TokenFile) ReadHash() (string, error) {
	data, err := os.ReadFile(tf.path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteHash writes a token hash to the file with secure permissions
func (tf *TokenFile) WriteHash(hash string) error {
	// Ensure directory exists
	dir := filepath.Dir(tf.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file with secure permissions
	if err := os.WriteFile(tf.path, []byte(hash+"\n"), TokenFileMode); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// Delete removes the token file
func (tf *TokenFile) Delete() error {
	if err := os.Remove(tf.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete token file: %w", err)
	}
	return nil
}

// CheckPermissions verifies the token file has secure permissions
func (tf *TokenFile) CheckPermissions() error {
	info, err := os.Stat(tf.path)
	if err != nil {
		return err
	}

	mode := info.Mode().Perm()
	if mode != TokenFileMode {
		return fmt.Errorf("insecure token file permissions: %o (expected %o)", mode, TokenFileMode)
	}

	return nil
}

// GetOrCreateToken generates a new token if none exists, or indicates one already exists.
// Returns: (token, isNew, error)
// - If isNew is true, token contains the plain token (SAVE IT NOW - shown only once)
// - If isNew is false, token is empty (token already exists, cannot be retrieved)
func GetOrCreateToken(dataDir string) (string, bool, error) {
	tf := NewTokenFile(dataDir)

	// Check for existing token hash
	if tf.Exists() {
		// Token already exists - cannot retrieve it
		return "", false, nil
	}

	// Generate new token
	token, err := GenerateToken()
	if err != nil {
		return "", false, err
	}

	// Hash the token
	hash, err := HashToken(token)
	if err != nil {
		return "", false, err
	}

	// Save hash to file (NOT the plain token)
	if err := tf.WriteHash(hash); err != nil {
		return "", false, err
	}

	return token, true, nil
}

// RegenerateToken creates a new token and overwrites the existing hash.
// Returns the new plain token (SAVE IT - shown only once).
func RegenerateToken(dataDir string) (string, error) {
	tf := NewTokenFile(dataDir)

	// Generate new token
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}

	// Hash the token
	hash, err := HashToken(token)
	if err != nil {
		return "", err
	}

	// Save hash to file (overwrites existing)
	if err := tf.WriteHash(hash); err != nil {
		return "", err
	}

	return token, nil
}

// GetTokenHashFromFile reads the stored token hash from file.
// Returns empty string if no hash exists.
func GetTokenHashFromFile(dataDir string) (string, error) {
	tf := NewTokenFile(dataDir)
	if !tf.Exists() {
		return "", nil
	}
	return tf.ReadHash()
}

// GetTokenFromEnv gets the API token from environment variable.
// Returns empty string if not set.
func GetTokenFromEnv() string {
	return os.Getenv("PORCUPIN_API_TOKEN")
}

// TokenExistsInFile checks if a token hash file exists
func TokenExistsInFile(dataDir string) bool {
	tf := NewTokenFile(dataDir)
	return tf.Exists()
}
