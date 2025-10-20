package cloudinary

import (
	"context"
	"log"
	"path"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

func UploadImage(url string) string {
	cld, err := cloudinary.NewFromURL("cloudinary://194233595147118:CSc0RkmFzQkPElpjh-iarRZxDvQ@dglwqvzpg")
	if err != nil {
		log.Fatal("Khởi tạo Cloudinary thất bại:", err)
	}

	filename := path.Base(url)
	ctx := context.Background()
	resp, err := cld.Upload.Upload(ctx, url, uploader.UploadParams{
		PublicID: filename,
	})
	if err != nil {
		log.Fatal("Upload thất bại:", err)
	}
	log.Println("Upload thành công:", resp.SecureURL)
	return resp.SecureURL
}
