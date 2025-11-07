package uploader

import (
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

func (cld *CloudinaryService) IsLocalImage(image string) bool {
	_, err := os.Stat(image)
	return err == nil
}

func (cld *CloudinaryService) IsBase64Image(image string) bool {
	// If the base64 image is formattly correctly
	if strings.HasPrefix(image, "data:") || strings.Contains(image, ";base64,") {
		return true
	}

	// If not, then we try to decode it
	_, err := base64.StdEncoding.DecodeString(image)
	return err == nil
}

func (cld *CloudinaryService) IsRemoteURLImage(image string) bool {
	return strings.Contains(image, "http") || strings.Contains(image, "https")
}

// Upload image into cloud service.
// Image here can be: local file path, io.Reader, base64, URL or storage bucket.
func (cld *CloudinaryService) UploadImage(ctx context.Context, name string, image any) (*uploader.UploadResult, error) {
	resp, err := cld.cld.Upload.Upload(ctx, image, uploader.UploadParams{
		PublicID: name,
	})

	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	return resp, nil
}
