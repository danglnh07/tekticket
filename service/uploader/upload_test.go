package uploader

import (
	"net/http"
	"os"
	"strings"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	service *Uploader
)

func TestMain(m *testing.M) {
	// Omit test if this is CI environment
	if strings.TrimSpace(os.Getenv("CI")) != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		return
	}

	// Create test dependency
	service = NewUploader(os.Getenv("DIRECTUS_ADDR"), os.Getenv("DIRECTUS_STATIC_TOKEN"))
	os.Exit(m.Run())
}

func TestUpload(t *testing.T) {
	image, err := util.GenerateQR(util.RandomString(10))
	require.NoError(t, err)
	require.NotEmpty(t, image)

	id, status, err := service.Upload("test-new-upload.png", image)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, id)
}
