package worker

import (
	"os"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSendVerifyEmail(t *testing.T) {
	// Send email
	err := processor.(*RedisTaskProcessor).SendVerifyEmail(SendVerifyEmailPayload{
		ID:       util.RandomString(12),
		Email:    os.Getenv("RECEIVE_EMAIL"),
		Username: util.RandomString(10),
		OTP:      util.GenerateRandomOTP(),
	})
	require.NoError(t, err)
}
