package cloudinary

import (
	"context"
	"fmt"
	"path"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// Cloudinary service
type CloudinaryService struct {
	cld *cloudinary.Cloudinary
}

// Constuctor for cloudinary service
func NewCld(cloudName, cloudKey, cloudSecret string) (*CloudinaryService, error) {
	cld, err := cloudinary.NewFromURL("cloudinary://" + cloudKey + ":" + cloudSecret + "@" + cloudName)
	if err != nil {
		return nil, err
	}
	return &CloudinaryService{cld: cld}, nil
}

// Upload image into cloud service
func (cld *CloudinaryService) UploadImage(url string) (string, error) {

	filename := path.Base(url)
	ctx := context.Background()
	resp, err := cld.cld.Upload.Upload(ctx, url, uploader.UploadParams{
		PublicID: filename,
	})
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	return resp.SecureURL, nil
}
