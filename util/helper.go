package util

import (
	"bytes"
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

	"github.com/gosimple/slug"
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

// Generate slug
func GenerateSlug(content string) string {
	return slug.Make(content)
}

// Generate random OTP code (6 digits code)
func GenerateRandomOTP() string {
	return fmt.Sprintf("%d", rand.Intn(999999-100000+1)+100000)
}

// Helper: make request to Directus
type DirectusResp struct {
	Data any `json:"data"`
}

func MakeRequest(method, url string, body map[string]any, token string, result any) (int, error) {
	var (
		req *http.Request
		err error
	)

	if body != nil {
		// build body
		data, err := json.Marshal(body)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(data))
		if err != nil {
			return http.StatusInternalServerError, err
		}
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}

	// Set request header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Make request to Directus API
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Check if status code is success
	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("response status not ok: %s", string(message)+" "+resp.Status)
	}

	// Parse Directus response
	directusResp := DirectusResp{Data: result}
	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		return http.StatusInternalServerError, err
	}

	return resp.StatusCode, nil
}

// Generate the URL of image using its ID
func CreateImageLink(id string) string {
	return fmt.Sprintf("http://localhost:8080/images/%s", id)
}

// NormalizeChoseDate ensures chose_date is in full ISO format.
// If input is YYYY-MM-DD, it converts to the start of day in UTC (T00:00:00Z).
func NormalizeChoseDate(d string) string {
	if d == "" {
		return ""
	}
	if strings.Contains(d, "T") { // already full ISO
		return d
	}
	return d + "T00:00:00Z"
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
func Decode(str string) string {
	data, err := base64.URLEncoding.DecodeString(str)
	if err != nil {
		return ""
	}
	return string(data)
}
