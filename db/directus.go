package db

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Directus share structure: most directus request, if success, will return one field 'data' that contains all information
type DirectusResp struct {
	Data any `json:"data"`
}

// Directus error extensions field
type Extension struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Directus error body, contain the error message, and extension of the error
type DirectusErrorBody struct {
	Message   string    `json:"message"`
	Extension Extension `json:"extensions"`
}

// Directus will return a list of errors, even if they are (and most of the time), only 1 error
type DirectusErrorResp struct {
	Errors []DirectusErrorBody `json:"errors"`
}

// Directus error code (https://directus.io/docs/guides/connect/errors)
// The FORBIDDEN code is quite tricky here, because Directus treat no permission error (operate on fields that you have no permission),
// and non-exists fields error (operate on non existing fields, including accessing item with ID) as the same FORBIDDEN
const (
	FAILED_VALIDATION      = "FAILED_VALIDATION"
	FORBIDDEN              = "FORBIDDEN"
	INVALID_TOKEN          = "INVALID_TOKEN"
	TOKEN_EXPIRED          = "TOKEN_EXPIRED"
	INVALID_CREDENTIALS    = "INVALID_CREDENTIALS"
	INVALID_IP             = "INVALID_IP"
	INVALID_OTP            = "INVALID_OTP"
	INVALID_PAYLOAD        = "INVALID_PAYLOAD"
	INVALID_QUERY          = "INVALID_QUERY"
	UNSUPPORTED_MEDIA_TYPE = "UNSUPPORTED_MEDIA_TYPE"
	REQUESTS_EXCEEDED      = "REQUESTS_EXCEEDED"
	ROUTE_NOT_FOUND        = "ROUTE_NOT_FOUND"
	SERVICE_UNAVAILABLE    = "SERVICE_UNAVAILABLE"
	UNPROCESSABLE_CONTENT  = "UNPROCESSABLE_CONTENT"
)

// Implement error interface for DirectusErrorResp
func (de *DirectusErrorResp) Error() string {
	message := strings.Builder{}
	for _, directusErr := range de.Errors {
		message.WriteString(fmt.Sprintf("Error: %s (%s)\n", directusErr.Message, directusErr.Extension.Code))
	}
	return message.String()
}

// Check if an error is of type DirectusErrorResp
func IsDirectusError(err error) bool {
	var directusErr *DirectusErrorResp
	return err != nil && errors.As(err, &directusErr)
}

func MakeRequest(method, url string, body any, token string, result any) (int, error) {
	var (
		req *http.Request
		err error
	)

	// Build HTTP request based on body payload
	if body != nil {
		// build body
		data, err := json.Marshal(body)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(data))
		if err != nil {
			return http.StatusInternalServerError, err
		}
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}

	// Set request header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Make request to Directus API
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Check status code. Typically, Directus error code ranges from 4xx to 5xx (https://directus.io/docs/guides/connect/errors)
	if resp.StatusCode >= 400 {
		// Directus return a list of errors
		var errs DirectusErrorResp
		json.NewDecoder(resp.Body).Decode(&errs)
		return resp.StatusCode, &errs
	}

	// Parse Directus response
	if resp.StatusCode != http.StatusNoContent {
		// Only parse if Directus actually return something
		directusResp := DirectusResp{Data: result}
		if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
			return http.StatusInternalServerError, err
		}
	}

	return resp.StatusCode, nil
}
