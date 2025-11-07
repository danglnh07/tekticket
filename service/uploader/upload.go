package uploader

import (
	"context"
	"fmt"
	"net/http"
	"tekticket/db"
	"tekticket/util"

	"github.com/google/uuid"
)

type Uploader struct {
	cloudinary *CloudinaryService
	config     *util.Config
}

func NewUploader(cloudinary *CloudinaryService, config *util.Config) *Uploader {
	return &Uploader{
		cloudinary: cloudinary,
		config:     config,
	}
}

// Upload image into both Cloudinary and Directus
func (uploader *Uploader) UploadImage(ctx context.Context, image any) (int, string, error) {
	// Upload the image into Cloudinary first
	id := uuid.New()
	cloudResp, err := uploader.cloudinary.UploadImage(ctx, id.String(), image)
	if err != nil {
		return http.StatusInternalServerError, "", nil
	}
	util.LOGGER.Info("Upload cloudinary success", "id", id.String(), "url", cloudResp.SecureURL)

	// Upload image into Directus
	url := fmt.Sprintf("%s/files/import", uploader.config.DirectusAddr)
	var imageResp db.DirectusImage
	status, err := db.MakeRequest(
		"POST",
		url,
		map[string]any{
			"url": cloudResp.SecureURL,
			"data": map[string]any{
				"id":      id.String(),
				"storage": "cloudinary",
				"type":    "image/png",
			},
		},
		uploader.config.DirectusStaticToken,
		&imageResp,
	)
	if err != nil {
		util.LOGGER.Error("Failed to upload image into Directus", "error", err)
		return status, "", err
	}

	return status, imageResp.ID, nil
}
