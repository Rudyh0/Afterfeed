package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Time-based one-time passwords (RFC 6238) with the defaults every
// authenticator app supports: SHA-1, 6 digits, 30-second steps.

const (
	totpStep   = 30
	totpDigits = 6
	// totpSkew is how many steps before and after the current one are accepted,
	// to allow for clocks that are slightly off.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a new random secret, base32-encoded.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b)
}

// TOTPURI returns the otpauth URI that authenticator apps read from a QR code.
func TOTPURI(secret, account string) string {
	label := url.PathEscape("Afterfeed:" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", "Afterfeed")
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpStep))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func hotp(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, code%1_000_000)
}

// TOTPCode returns the code for secret at time t.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	return hotp(key, uint64(t.Unix()/totpStep)), nil
}

// VerifyTOTP checks code against secret at time t. It returns the time step
// that matched, so the caller can refuse a code that was already used.
func VerifyTOTP(secret, code string, t time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return 0, false
	}
	now := t.Unix() / totpStep
	for d := -totpSkew; d <= totpSkew; d++ {
		s := now + int64(d)
		if hmac.Equal([]byte(hotp(key, uint64(s))), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// NewRecoveryCodes returns n random recovery codes such as "k7m2-q9xd".
func NewRecoveryCodes(n int) []string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	codes := make([]string, n)
	for i := range codes {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		var sb strings.Builder
		for j, c := range b {
			if j == 4 {
				sb.WriteByte('-')
			}
			sb.WriteByte(alphabet[int(c)%len(alphabet)])
		}
		codes[i] = sb.String()
	}
	return codes
}

// hashRecoveryCode normalises and hashes a recovery code. Codes have enough
// entropy that a fast hash is sufficient.
func hashRecoveryCode(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	c = strings.ReplaceAll(c, " ", "")
	c = strings.ReplaceAll(c, "-", "")
	sum := sha256.Sum256([]byte(c))
	return hex.EncodeToString(sum[:])
}
