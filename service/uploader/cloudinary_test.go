package uploader

import (
	"os"
	"strings"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	service *CloudinaryService
	err     error
)

func TestMain(m *testing.M) {
	// Omit test if this is CI environment
	if strings.TrimSpace(os.Getenv("CI")) != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		return
	}

	// Create test dependency
	service, err = NewCld(os.Getenv("CLOUDINARY_NAME"), os.Getenv("CLOUDINARY_APIKEY"), os.Getenv("CLOUDINARY_APISECRET"))
	if err != nil {
		util.LOGGER.Error("failed to create cloudinary service for test", "error", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestUploadImage(t *testing.T) {
	url, err := service.UploadImage("https://s3-api.fpt.vn/fptvn-storage/2025-09-04/1756983257_thumbdragonball.jpg")
	require.NoError(t, err)
	require.NotEmpty(t, url)
	util.LOGGER.Info("Test CloudinaryService", "url", url)
}
