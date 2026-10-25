package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Password and PIN hashes use PBKDF2-SHA256 from the standard library, stored
// as "pbkdf2-sha256$<iterations>$<salt>$<hash>".
const (
	passwordIterations = 600_000
	pinIterations      = 300_000
	saltLen            = 16
	keyLen             = 32
)

// MinPasswordLength is the shortest password Afterfeed accepts.
const MinPasswordLength = 10

func hashSecret(secret string, iterations int) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, iterations, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func verifySecret(secret, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false, errors.New("unknown hash format")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false, errors.New("invalid iteration count")
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil {
		return false, err
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	got, err := pbkdf2.Key(sha256.New, secret, salt, iterations, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// HashPassword returns a hash of a password for storage.
func HashPassword(password string) (string, error) { return hashSecret(password, passwordIterations) }

// VerifyPassword reports whether password matches the stored hash.
func VerifyPassword(password, hash string) bool {
	ok, err := verifySecret(password, hash)
	return err == nil && ok
}

// HashPIN returns a hash of a vault PIN for storage.
func HashPIN(pin string) (string, error) { return hashSecret(pin, pinIterations) }

// VerifyPIN reports whether pin matches the stored hash.
func VerifyPIN(pin, hash string) bool {
	ok, err := verifySecret(pin, hash)
	return err == nil && ok
}

// ValidPIN reports whether pin is exactly six digits.
func ValidPIN(pin string) bool {
	if len(pin) != 6 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// RandomPassword returns a random password that is easy to type.
func RandomPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, 0, 19)
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			out = append(out, '-')
		}
		out = append(out, alphabet[int(c)%len(alphabet)])
	}
	return string(out)
}
