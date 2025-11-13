package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"tekticket/db"
)

type Uploader struct {
	directusAddr        string
	directusStaticToken string
}

func NewUploader(directusAddr, directusStaticToken string) *Uploader {
	return &Uploader{
		directusAddr:        directusAddr,
		directusStaticToken: directusStaticToken,
	}
}

func (uploader *Uploader) Upload(filename string, image []byte) (string, int, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create part with custom Content-Type header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", "image/png")

	part, err := writer.CreatePart(h)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}

	if _, err := part.Write(image); err != nil {
		return "", http.StatusInternalServerError, err
	}
	writer.Close()

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/files", uploader.directusAddr), body)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}

	req.Header.Set("Authorization", "Bearer "+uploader.directusStaticToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		directusErr := db.DirectusErrorResp{}
		if err := json.NewDecoder(resp.Body).Decode(&directusErr); err != nil {
			return "", http.StatusInternalServerError, err
		}

		return "", resp.StatusCode, &directusErr
	}

	// Parse Directus response
	if resp.StatusCode != http.StatusNoContent {
		// Only parse if Directus actually return something
		var directusImg db.DirectusImage
		directusResp := db.DirectusResp{Data: &directusImg}
		if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
			return "", http.StatusInternalServerError, err
		}

		return directusImg.ID, http.StatusOK, nil
	}

	return "", http.StatusNoContent, nil
}
