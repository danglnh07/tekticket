package util

import (
	"log/slog"
	"math/rand"
	"os"
	"strings"

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
