package security

import (
	"math/rand"
	"tekticket/db"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestToken(t *testing.T) {
	// Create test data
	id := uuid.New()
	tokenType := []TokenType{AccessToken, RefreshToken}[rand.Intn(2)]
	version := rand.Intn(10)

	// Create token
	token, err := service.CreateToken(id, db.Customer, tokenType, version)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Verify token
	result, err := service.VerifyToken(token)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Compare the test data with the extract claims
	require.Equal(t, id, result.ID)
	require.Equal(t, tokenType, result.TokenType)
	require.Equal(t, version, result.Version)
}
