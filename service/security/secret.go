package security

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// Method to hash a string using SHA-256
func Hash(str string) string {
	hasher := sha256.New()
	hasher.Write([]byte(str))
	return hex.EncodeToString(hasher.Sum(nil))
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

// Methods to hash passwords using bcrypt
func BcryptHash(str string) (string, error) {
	// Use bcrypt to hash the password
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(str), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// Method to compare a bcrypt hashed password with a plain text password
func BcryptCompare(hashedStr, plainStr string) bool {
	// Compare the hashed password with the plain text password
	err := bcrypt.CompareHashAndPassword([]byte(hashedStr), []byte(plainStr))
	return err == nil
}

