package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

/*
 * This file will contains all of Directus collections that are used inthe system.
 * Note that not all fields are included, since this application is only for customer
 */

// directus_roles
type Role struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// directus_users
type User struct {
	ID                 string              `json:"id,omitempty"`
	FirstName          string              `json:"first_name,omitempty"`
	LastName           string              `json:"last_name,omitempty"`
	Email              string              `json:"email,omitempty"`
	Password           string              `json:"password,omitempty"`
	Avatar             string              `json:"avatar,omitempty"`
	Location           string              `json:"location,omitempty"`
	Status             string              `json:"status,omitempty"`
	Role               *Role               `json:"role,omitempty"`
	UserMembershipLogs []UserMembershipLog `json:"user_membership_logs,omitempty"`
	Bookings           []Booking           `json:"bookings,omitempty"`
	UserTelegrams      []UserTelegram      `json:"user_telegrams,omitempty"`
}

// user_telegrams
type UserTelegram struct {
	ID             string `json:"id,omitempty"`
	TelegramChatID string `json:"telegram_chat_id,omitempty"`
	User           *User  `json:"user_id,omitempty"`
}

// memberships
type Membership struct {
	ID           string `json:"id,omitempty"`
	Status       string `json:"status,omitempty"`
	Tier         string `json:"tier,omitempty"`
	BasePoint    int    `json:"base_point,omitempty"`
	EarlyBuyTime int    `json:"early_buy_time,omitempty"`
	Discount     string `json:"discount,omitempty"` // Directus will return a string even if they are set as decimal
}

// user_membership_logs
type UserMembershipLog struct {
	ID              string `json:"id,omitempty"`
	PointDelta      int    `json:"points_delta,omitempty"`
	ResultingPoints int    `json:"resulting_points,omitempty"`
}

// categories
type Category struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// events
type Event struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	Description    string          `json:"description,omitempty"`
	Address        string          `json:"address,omitempty"`
	City           string          `json:"city,omitempty"`
	Country        string          `json:"country,omitempty"`
	Slug           string          `json:"slug,omitempty"`
	PreviewImage   string          `json:"preview_image,omitempty"`
	Status         string          `json:"status,omitempty"`
	Creator        *User           `json:"creator_id,omitempty"`
	Category       *Category       `json:"category_id,omitempty"`
	EventSchedules []EventSchedule `json:"event_schedules,omitempty"`
	SeatZones      []SeatZone      `json:"seat_zone,omitempty"`
	Tickets        []Ticket        `json:"tickets,omitempty"`
	Bookings       []Booking       `json:"bookings,omitempty"`
}

// event_schedules
type EventSchedule struct {
	ID               string `json:"id,omitempty"`
	StartTime        string `json:"start_time,omitempty"`
	EndTime          string `json:"end_time,omitempty"`
	StartCheckinTime string `json:"start_checkin_time,omitempty"`
	EndCheckinTime   string `json:"end_checkin_time,omitempty"`
}

// seat_zones
type SeatZone struct {
	ID          string   `json:"id,omitempty"`
	Description string   `json:"description,omitempty"`
	TotalSeats  int      `json:"total_seats,omitempty"`
	Status      string   `json:"status,omitempty"`
	Event       *Event   `json:"event_id,omitempty"`
	Seats       []Seat   `json:"seats,omitempty"`
	Tickets     []Ticket `json:"tickets,omitempty"`
}

// seats
type Seat struct {
	ID         string    `json:"id,omitempty"`
	SeatNumber string    `json:"seat_number,omitempty"`
	Status     string    `json:"status,omitempty"`
	ReserveBy  *User     `json:"reserved_by,omitempty"`
	SeatZone   *SeatZone `json:"seat_zone_id,omitempty"`
}

// tickets
type Ticket struct {
	ID          string    `json:"id,omitempty"`
	Rank        string    `json:"rank,omitempty"`
	Description string    `json:"description,omitempty"`
	BasePrice   int       `json:"base_price,omitempty"`
	Status      string    `json:"status,omitempty"`
	Event       *Event    `json:"event_id,omitempty"`
	SeatZone    *SeatZone `json:"seat_zone_id,omitempty"`
}

// ticket_selling_schedules
type TicketSellingSchedule struct {
	ID               string  `json:"id,omitempty"`
	Total            int     `json:"total,omitempty"`
	Avaible          int     `json:"available,omitempty"`
	StartSellingTime string  `json:"start_selling_time,omitempty"`
	EndSellingTime   string  `json:"end_selling_time,omitempty"`
	Status           string  `json:"status,omitempty"`
	Ticket           *Ticket `json:"ticket_id,omitempty"`
}

// bookings
type Booking struct {
	ID           string        `json:"id,omitempty"`
	Status       string        `json:"status,omitempty"`
	Customer     *User         `json:"customer_id,omitempty"`
	Event        *Event        `json:"event_id,omitempty"`
	BookingItems []BookingItem `json:"booking_items,omitempty"`
	Payments     []Payment     `json:"payments,omitempty"`
}

// booking_items
type BookingItem struct {
	ID            string         `json:"id,omitempty"`
	Price         int            `json:"price,omitempty"`
	QR            string         `json:"qr,omitempty"`
	Status        string         `json:"status,omitempty"`
	Booking       *Booking       `json:"booking_id,omitempty"`
	Ticket        *Ticket        `json:"ticket_id,omitempty"`
	Seat          *Seat          `json:"seat_id,omitempty"`
	EventSchedule *EventSchedule `json:"event_schedule_id,omitempty"`
}

// payments
type Payment struct {
	ID             string   `json:"id,omitempty"`
	TransactionID  string   `json:"transaction_id,omitempty"`
	Amount         int      `json:"amount,omitempty"`
	PaymentGateway string   `json:"payment_gateway,omitempty"`
	PaymentMethod  string   `json:"payment_method,omitempty"`
	Status         string   `json:"status,omitempty"`
	Booking        *Booking `json:"booking_id,omitempty"`
	Refunds        []Refund `json:"refunds,omitempty"`
}

// refunds
type Refund struct {
	ID      string   `json:"id,omitempty"`
	Amount  int      `json:"amount,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Status  string   `json:"status,omitempty"`
	Payment *Payment `json:"payment_id,omitempty"`
}

// checkins
type Checkin struct {
	ID          string       `json:"id,omitempty"`
	CheckinDate string       `json:"date_created,omitempty"`
	Staff       *User        `json:"staff_id,omitempty"`
	BookingItem *BookingItem `json:"booking_item_id,omitempty"`
}

// Directus share structure: most directus request, if success, will return a one field: data
type DirectusResp struct {
	Data any `json:"data"`
}

// Directus error response
type Extension struct {
	Code string `json:"code"`
}

type DirectusErrorBody struct {
	Message   string    `json:"message"`
	Extension Extension `json:"extensions"`
}

type DirectusErrorResp struct {
	Errors []DirectusErrorBody `json:"errors"`
}

func MakeRequest(method, url string, body map[string]any, token string, result any) (int, error) {
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
		message := strings.Builder{}
		for _, directusErr := range errs.Errors {
			message.WriteString(fmt.Sprintf("Error: %s (%s)\n", directusErr.Message, directusErr.Extension.Code))
		}

		return resp.StatusCode, fmt.Errorf("response status not ok (%d): %s", resp.StatusCode, message.String())
	}

	// Parse Directus response
	directusResp := DirectusResp{Data: result}
	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		return http.StatusInternalServerError, err
	}

	return resp.StatusCode, nil
}

// Image response: the response when uploading image in Directus
type DirectusImage struct {
	ID string `json:"id"`
}
