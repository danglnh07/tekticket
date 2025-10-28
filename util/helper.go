package util

import (
	"bytes"
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
func MakeRequest(method, url string, body map[string]any, token string) (*http.Response, int, error) {
	var (
		req *http.Request
		err error
	)

	if body != nil {
		// build body
		data, err := json.Marshal(body)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(data))
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}

	// Set request header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Make request to Directus API
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	// Check if status code is success
	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("response status not ok: %s", string(message)+" "+resp.Status)
	}

	return resp, resp.StatusCode, nil
}

// Generate the URL of image using its ID
func CreateImageLink(id string) string {
	return fmt.Sprintf("http://localhost:8080/images/%s", id)
}
