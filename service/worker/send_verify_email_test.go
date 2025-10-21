package worker

import (
	"os"
	"tekticket/service/security"
	"tekticket/util"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSendVerifyEmail(t *testing.T) {
	// Send email
	id, err := uuid.NewUUID()
	require.NoError(t, err)
	err = processor.(*RedisTaskProcessor).SendVerifyEmail(SendVerifyEmailPayload{
		ID:       id,
		Email:    os.Getenv("RECEIVE_EMAIL"),
		Username: util.RandomString(10),
		OTP:      security.GenerateRandomOTP(),
	})
	require.NoError(t, err)
}
