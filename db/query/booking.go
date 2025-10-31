package query

import (
	"errors"
	"tekticket/db"
)

func CreateBooking(booking db.Booking) (int64, error) {
	result := db.NewQueries().DB.Create(&booking)
	if result.Error != nil {
		return 0, errors.New("khong the tao booking")
	}
	return result.RowsAffected, nil
}
