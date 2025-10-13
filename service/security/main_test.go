package security

import (
	"os"
	"testing"
	"time"
)

var (
	secretKey              = []byte("SOME-SECRET-KEY")
	tokenExpiration        = time.Duration(60 * time.Minute)
	refreshTokenExpiration = time.Duration(1440 * time.Minute)

	service = NewJWTService(secretKey, tokenExpiration, refreshTokenExpiration)
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
