package util

import (
	"crypto/aes"
	"crypto/cipher"
	cryprand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"tekticket/db"

	"github.com/skip2/go-qrcode"
)

// Global logger
var LOGGER = slog.New(slog.NewTextHandler(os.Stdout, nil))

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Generate a random string with length n. The character possible is defined in the alphabet constant
func RandomString(n int) string {
	var sb strings.Builder
	k := len(alphabet)

	for range n {
		c := alphabet[rand.Intn(k)]
		sb.WriteByte(c)
	}

	return sb.String()
}

// Generate QR
func GenerateQR(content string) ([]byte, error) {
	return qrcode.Encode(content, qrcode.Medium, 256)
}

// Generate random OTP code (6 digits code)
func GenerateRandomOTP() string {
	return fmt.Sprintf("%d", rand.Intn(999999-100000+1)+100000)
}

// Generate the URL of image using its ID
func CreateImageLink(domain, id string) string {
	return fmt.Sprintf("%s/images/%s", domain, id)
}

// Encrypt encrypts plaintext using AES-256 GCM.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(cryprand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256 GCM.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, encryptedMessage := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedMessage, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// Methods to encode a string using Base64 URL encoding
func Encode(str string) string {
	return base64.URLEncoding.EncodeToString([]byte(str))
}

// Method to decode a Base64 URL encoded string
func Decode(str string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Helper: format HTML warning message for Telegram
func FormatWarningHTML(text string) string {
	return fmt.Sprintf("<b>%s</b>", strings.ToUpper(text))
}

// Helper: format HTML notification message for Telegram
func FormatNotificationHTML(title, body string) string {
	// Body should already be an HTML template, so we don't do anything to it
	return fmt.Sprintf("<b>%s</b>\n\n%s", strings.ToUpper(title), body)
}

// Helper method: get user ID from access token
func ExtractIDFromToken(token string) (string, error) {
	// Decode base64 token to get the JWT payload
	jwtPayload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		return "", err
	}

	// If decode success, try unmarshal payload to get user ID
	var tokenPayload map[string]any
	if err := json.Unmarshal(jwtPayload, &tokenPayload); err != nil {
		return "", err
	}

	// Try parsing ID from map (avoid panic error)
	if id, ok := tokenPayload["id"].(string); ok {
		return id, nil
	}

	return "", fmt.Errorf("failed to parse ID")
}

// Helper method: extract role from access token
func ExtractRoleFromToken(token, directusAddr, staticAccessToken string) (string, error) {
	// Decode base64 token to get the JWT payload
	jwtPayload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		return "", err
	}

	// If decode success, try unmarshal payload to get user ID
	var tokenPayload map[string]any
	if err := json.Unmarshal(jwtPayload, &tokenPayload); err != nil {
		return "", err
	}

	// Try parsing role ID from map (avoid panic error)
	var (
		roleID string
		ok     bool
	)
	if roleID, ok = tokenPayload["id"].(string); !ok {
		return "", fmt.Errorf("failed to parse role ID from access token")
	}

	// Make request to Directus to get the role name
	url := fmt.Sprintf("%s/roles/%s?fields=id,name,description", directusAddr, roleID)
	var role db.Role
	status, err := db.MakeRequest("GET", url, nil, staticAccessToken, &role)
	if err != nil {
		return "", err
	}

	if status != http.StatusOK {
		return "", fmt.Errorf("failed to get role with this ID: %s", roleID)
	}

	return role.Name, nil
}
