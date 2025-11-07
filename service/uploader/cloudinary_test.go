package uploader

import (
	"context"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUploadImage(t *testing.T) {
	ctx := context.Background()
	result, err := service.UploadImage(
		ctx,
		util.RandomString(6),
		"https://s3-api.fpt.vn/fptvn-storage/2025-09-04/1756983257_thumbdragonball.jpg",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.URL)
	require.NotEmpty(t, result.SecureURL)
	util.LOGGER.Info("Test CloudinaryService", "url", result.URL, "secure_url", result.SecureURL)
}
