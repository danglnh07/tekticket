package uploader

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// Cloudinary service
type CloudinaryService struct {
	cld *cloudinary.Cloudinary
}

// Constuctor for cloudinary service
func NewCld(cloudName, cloudKey, cloudSecret string) (*CloudinaryService, error) {
	cld, err := cloudinary.NewFromParams(cloudName, cloudKey, cloudSecret)
	if err != nil {
		return nil, err
	}
	return &CloudinaryService{cld: cld}, nil
}

// Upload image into cloud service.
// Image here can be: local file path, io.Reader, base64, URL or storage bucket

func (cld *CloudinaryService) UploadImage(ctx context.Context, image, name string) (*uploader.UploadResult, error) {
	var src any

	// Detect base64 input
	if strings.HasPrefix(image, "data:") || strings.Contains(image, ";base64,") {
		// Pass directly if it's a valid data URI
		src = image
	} else if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		// Remote URL
		src = image
	} else if _, err := os.Stat(image); err == nil {
		// Local file path
		src = image
	} else {
		// Fallback: treat as raw base64
		data, err := base64.StdEncoding.DecodeString(image)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 image: %w", err)
		}
		src = bytes.NewReader(data)
	}

	resp, err := cld.cld.Upload.Upload(ctx, src, uploader.UploadParams{
		PublicID: name,
	})
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	return resp, nil
}
