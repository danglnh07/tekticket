package db

/*
 * This file will contains all of Directus collections that are used.
 * Note that not all models and fields are included
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
	ID           string       `json:"id,omitempty"`
	Status       string       `json:"status,omitempty"`
	Tier         string       `json:"tier,omitempty"`
	BasePoint    int          `json:"base_point,omitempty"`
	EarlyBuyTime int          `json:"early_buy_time,omitempty"`
	Discount     DecimalFloat `json:"discount,omitempty"`
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
	SeatZones      []SeatZone      `json:"seat_zones,omitempty"`
	Tickets        []Ticket        `json:"tickets,omitempty"`
	Bookings       []Booking       `json:"bookings,omitempty"`
}

// event_schedules
type EventSchedule struct {
	ID               string    `json:"id,omitempty"`
	StartTime        *DateTime `json:"start_time,omitempty"`
	EndTime          *DateTime `json:"end_time,omitempty"`
	StartCheckinTime *DateTime `json:"start_checkin_time,omitempty"`
	EndCheckinTime   *DateTime `json:"end_checkin_time,omitempty"`
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
	ID               string    `json:"id,omitempty"`
	Total            int       `json:"total,omitempty"`
	Avaible          int       `json:"available,omitempty"`
	StartSellingTime *DateTime `json:"start_selling_time,omitempty"`
	EndSellingTime   *DateTime `json:"end_selling_time,omitempty"`
	Status           string    `json:"status,omitempty"`
	Ticket           *Ticket   `json:"ticket_id,omitempty"`
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
	ID             string    `json:"id,omitempty"`
	DateCreated    *DateTime `json:"date_created,omitempty"`
	TransactionID  string    `json:"transaction_id,omitempty"`
	Amount         int       `json:"amount,omitempty"`
	PaymentGateway string    `json:"payment_gateway,omitempty"`
	PaymentMethod  string    `json:"payment_method,omitempty"`
	Status         string    `json:"status,omitempty"`
	Booking        *Booking  `json:"booking_id,omitempty"`
	Refunds        []Refund  `json:"refunds,omitempty"`
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
	ID            string       `json:"id,omitempty"`
	CheckinDate   string       `json:"date_created,omitempty"`
	Staff         *User        `json:"staff_id,omitempty"`
	BookingItem   *BookingItem `json:"booking_item_id,omitempty"`
	CheckinDevice string       `json:"checkin_device,omitempty"`
}

// settings
type Setting struct {
	ID                        string       `json:"id"`
	Version                   string       `json:"version"`
	InUsed                    bool         `json:"in_used"`
	Status                    string       `json:"status"`
	MoneyToPointRate          int          `json:"money_to_point_rate"`
	MinEventDurationMinutes   int          `json:"min_event_duration_minutes"`
	MinEventLeadDays          int          `json:"min_event_lead_days"`
	MaxReservationHoldMinutes int          `json:"max_reservation_hold_minutes"`
	MinSellingDurationMinutes int          `json:"min_selling_duration_minutes"`
	PaymentFeePercent         DecimalFloat `json:"payment_fee_percent"`
	MaxFullRefundHours        int          `json:"max_full_refund_hours"`
	Email                     string       `json:"email"`                  // Platform email
	AppPassword               string       `json:"app_password"`           // Platform email's app password
	SecretKey                 string       `json:"secret_key"`             // Platfrom secret key
	ResetPasswordURL          string       `json:"reset_password_url"`     // The frontend URL of the reset password page
	CheckinURL                string       `json:"checkin_url"`            // The frontend URL of the checkin page
	StripePublishableKey      string       `json:"stripe_publishable_key"` // Stripe publishable key
	StripeSecretKey           string       `json:"stripe_secret_key"`      // Stripe secret key
	AblyApiKey                string       `json:"ably_api_key"`           // Ably API key
	TelegramBotToken          string       `json:"telegram_bot_token"`     // Telegram bot token
	ServerDomain              string       `json:"server_domain"`          // Server domain, used for external API calling
	MaxWorkers                int          `json:"max_workers"`            // The total of background workers running in the background
}

// Image response: the response when uploading image in Directus
type DirectusImage struct {
	ID string `json:"id"`
}
