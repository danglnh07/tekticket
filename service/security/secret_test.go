package security

import (
	"strconv"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test encode/decode logic
func TestEncodeDecode(t *testing.T) {
	// Create test data
	str := util.RandomString(10)

	// Try encoding and decode
	encode := Encode(str)
	require.NotEmpty(t, encode)
	require.Equal(t, str, Decode(encode))
}

// Test Bcrypt hash and compare logic
func TestBcryptHash(t *testing.T) {
	// Create test data
	str := util.RandomString(10)

	// Bcrypt hash
	hashed, err := BcryptHash(str)
	require.NoError(t, err)

	// Compare hash and raw string
	require.Equal(t, true, BcryptCompare(hashed, str))
}

// Test gen random OTP
func TestGenerateRandomOTP(t *testing.T) {
	code := GenerateRandomOTP()
	require.NotEmpty(t, code)
	require.Len(t, code, 6)

	// Since OTP is 6-digits code, technically it should be able to be parsed into a number in range [100000, 999999]
	number, err := strconv.Atoi(code)
	require.NoError(t, err)
	require.GreaterOrEqual(t, number, 100000)
	require.LessOrEqual(t, number, 999999)
}
