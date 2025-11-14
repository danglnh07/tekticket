package db

import (
	"encoding/json"
	"strconv"
	"time"
)

// Implement float64 custom unmarshal JSON that handle both float and string,
// since Directus decimal would return a string, not number
type DecimalFloat float64

func (df *DecimalFloat) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as float64 first
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*df = DecimalFloat(f)
		return nil
	}

	// If that fails, try as string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Parse string to float
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}

	*df = DecimalFloat(f)
	return nil
}

func (df *DecimalFloat) MarshalJSON() ([]byte, error) {
	// Convert to float64 and encode as JSON number (not string)
	f := float64(*df)
	return json.Marshal(f)
}

// Implement custom time.Time type since Directus will return a string for time
type DateTime time.Time

func (dt *DateTime) UnmarshalJSON(data []byte) error {
	var (
		datetimeStr string
	)
	if err := json.Unmarshal(data, &datetimeStr); err != nil {
		return err
	}

	// Try parsing with FC3339
	datetime, err := time.Parse(time.RFC3339, datetimeStr)
	if err == nil {
		*dt = DateTime(datetime)
		return nil
	}

	// Try without timezone (assume UTC)
	datetime, err = time.Parse("2006-01-02T15:04:05", datetimeStr)
	if err == nil {
		*dt = DateTime(datetime)
		return nil
	}

	return err
}

func (dt *DateTime) MarshalJSON() ([]byte, error) {
	t := time.Time(*dt)

	// Use RFC3339 format when serializing
	datetimeStr := t.UTC().Format(time.RFC3339)
	return json.Marshal(datetimeStr)
}
