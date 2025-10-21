package db

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Share fields of all models: ID, create at and updated at timestamp
type Model struct {
	// ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id,omitempty"`
	ID          uuid.UUID `gorm:"type:uuid;not null;default:gen_random_uuid();primaryKey" json:"id"`
	DateCreated time.Time `gorm:"not null;default:now()" json:"created_at"`
	DateUpdated time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func NewModel() Model {
	return Model{
		ID:          uuid.New(),
		DateCreated: time.Now(),
		DateUpdated: time.Now(),
	}
}

// Enum defined
type Role string

type UserStatus string

type Status string

type EventStatus string

type SeatStatus string

type BookingStatus string

type QRStatus string

type PaymentStatus string

type NotificationStatus string

// Constant defined
const (
	// Constant role defined
	Customer  Role = "customer"
	Organiser Role = "organiser"
	Staff     Role = "staff"
	Admin     Role = "admin"

	// User status
	Inactive UserStatus = "inactive"
	Active   UserStatus = "active"
	Banned   UserStatus = "banned"

	// Share status constant defined
	Draft     Status = "draft"
	Published Status = "published"
	Canceled  Status = "canceled"

	// Event status
	EventPending   EventStatus = "pending"
	EventPublished EventStatus = "published"
	EventCanceled  EventStatus = "canceled"

	// Seat status
	SeatEmpty    SeatStatus = "empty"
	SeatReserved SeatStatus = "reserved"
	SeatBooked   SeatStatus = "booked"

	// Booking status
	BookingPending  BookingStatus = "pending"
	BookingComplete BookingStatus = "complete"
	BookingCanceled BookingStatus = "canceled"
	BookingTimeout  BookingStatus = "timeout"

	// QR status
	QRValid   QRStatus = "valid"
	QRExpired QRStatus = "expired"
	QRUsed    QRStatus = "used"

	// Payment/Refund status
	PaymentFailed  PaymentStatus = "failed"
	PaymentSuccess PaymentStatus = "success"

	// Notification status
	NotificationQueued   NotificationStatus = "queued"
	NotificationFailed   NotificationStatus = "failed"
	NotificationSent     NotificationStatus = "sent"
	NotificationReceived NotificationStatus = "received"
	NotificationRead     NotificationStatus = "read"
)

// User: user of the system, consist of 4 roles:
// 1. customer (who want to book ticket)
// 2. organiser: event's organiser, who want to sell their ticket thorugh the platform
// 3. staff: event's suppport staff
// 4. admin: platform's administrator
type User struct {
	Model
	Username     string     `gorm:"type:varchar(50);not null;uniqueIndex" json:"username"`
	Email        string     `gorm:"type:varchar(50);not null;uniqueIndex" json:"email"`
	Phone        string     `gorm:"type:varchar(15);not null;uniqueIndex" json:"phone"`
	Password     string     `gorm:"type:varchar(60);not null" json:"password"`
	Avatar       string     `gorm:"not null" json:"avatar"` // When register, used the default avatar URL
	Role         Role       `gorm:"type:varchar(20);not null" json:"role"`
	Status       UserStatus `gorm:"type:varchar(20);not null;default:inactive" json:"status"`
	TokenVersion int        `gorm:"not null;default:0" json:"token_version"` // JWT refresh token version

	// Relationships
	Events                []Event                `gorm:"foreignKey:CreatorID" json:"events,omitempty"`
	Bookings              []Booking              `gorm:"foreignKey:CustomerID" json:"bookings,omitempty"`
	MembershipLogs        []UserMembershipLog    `gorm:"foreignKey:CustomerID" json:"membership_logs,omitempty"`
	OperatedLogs          []UserMembershipLog    `gorm:"foreignKey:OperatorID" json:"operated_logs,omitempty"`
	Checkins              []Checkin              `gorm:"foreignKey:StaffID" json:"checkins,omitempty"`
	ReservedSeats         []Seat                 `gorm:"foreignKey:ReservedBy" json:"reserved_seats,omitempty"`
	NotificationRecipents []NotificationRecipent `gorm:"foreignKey:ReceiverID" json:"notification_recipents,omitempty"`
}

// Membership: platform loyalty system to keep user engagement by providing additional privilege
// User membership can be calculated using their point: lower_tier_base <= user's point < higher_tier_base,
// then user is at lower tier -> no need for mapping
type Membership struct {
	Model
	Tier         string  `gorm:"type:varchar(20);not null;uniqueIndex" json:"tier"`
	BasePoint    uint    `gorm:"not null;uniqueIndex;default:0" json:"base_point"`      // The minimum point to reach/stay at this rank
	Discount     float64 `gorm:"type:decimal(10,2);not null;default:0" json:"discount"` // In %
	EarlyBuyTime int     `gorm:"not null;default:0" json:"early_buy_time"`              // In minutes
	Status       Status  `gorm:"type:varchar(20);not null;default:draft" json:"status"`
}

// User's point log: use to track when user's point fluctuate, useful for revoke point if refund happen, or tracking inactivity
// for membership downgrade
// `change_type` can be: earn (customer make a payment), lost (refund happen), adjust (admin manually adjust, like server-wide
// event gift,...)
// `points_delta`: is the change in point (positive if increase, negative if decrease)
// `resulting_points` is the result after point change. The latest record is the current customer's point
// Business rules:
// 1. Customer get points for their payment: 100.000 VND = 10 points
// 2. If the payment get refunded, the point get substracted
// 3. After payment/refund success, automatically upgrade/downgrade membership meet condition
// 4. After 12 months of inactive, automatically downgrade to 1-tier lower (if at lowest tier, do nothing)
// Constraint: make sure that the `users.id` has a role = 'customer'
type UserMembershipLog struct {
	Model
	CustomerID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"customer_id"`
	ChangeType       string     `gorm:"type:varchar(20);not null" json:"change_type"` // earn, spend, adjust
	PointsDelta      int        `gorm:"not null" json:"points_delta"`
	ResultingPoints  uint       `gorm:"not null" json:"resulting_points"`
	RelatedPaymentID *uuid.UUID `gorm:"type:uuid;index" json:"related_payment_id,omitempty"`
	OperatorID       *uuid.UUID `gorm:"type:uuid;index" json:"operator_id,omitempty"` // Can be null
	Reason           string     `gorm:"type:varchar(100)" json:"reason"`

	// Relationships
	Customer       User     `gorm:"foreignKey:CustomerID" json:"customer"`
	RelatedPayment *Payment `gorm:"foreignKey:RelatedPaymentID" json:"related_payment"`
	Operator       *User    `gorm:"foreignKey:OperatorID" json:"operator"` // Can be null
}

// Event's category: useful for filter409d1a28eb87
// Event's category should be managed by admin.
// There should always have an 'other' category incase an event of unknown category get added
type Category struct {
	Model
	Name        string `gorm:"type:varchar(100);not null;unique;index" json:"name"`
	Description string `gorm:"type:varchar(255)" json:"description"`
	Status      Status `gorm:"type:varchar(20);not null;default:draft;index" json:"status"` // draft, published, canceled

	// Relationships
	Events []Event `gorm:"foreignKey:CategoryID" json:"events,omitempty"`
}

// Event information.
// Constraint: `creator_id` must have role = 'organiser'
// Business rule: all event when created must be `pending`. Only admin can publish event. When published, sent notifications
// to organiser and customers
// `slug` is used for SEO friendly
type Event struct {
	Model
	CreatorID    uuid.UUID   `gorm:"type:uuid;not null" json:"creator_id"`
	Name         string      `gorm:"type:varchar(50);not null;index" json:"name"`
	Description  string      `gorm:"type:varchar(255);not null" json:"description"`
	CategoryID   uuid.UUID   `gorm:"type:uuid;not null;index" json:"category_id"`
	Address      string      `gorm:"type:varchar;not null;index" json:"address"`
	City         string      `gorm:"type:varchar;not null;index" json:"city"`
	Country      string      `gorm:"type:varchar;not null;index" json:"country"`
	PreviewImage string      `gorm:"type:varchar;not null" json:"preview_image"`
	Slug         string      `gorm:"type:varchar;not null;index" json:"slug"`
	Status       EventStatus `gorm:"type:varchar(20);not null;default:pending" json:"status"` // pending, published, canceled

	// Relationships
	Creator   User            `gorm:"foreignKey:CreatorID" json:"creator"`
	Category  Category        `gorm:"foreignKey:CategoryID" json:"category"`
	Schedules []EventSchedule `gorm:"foreignKey:EventID" json:"schedules,omitempty"`
	SeatZones []SeatZone      `gorm:"foreignKey:EventID" json:"seat_zones,omitempty"`
	Tickets   []Ticket        `gorm:"foreignKey:EventID" json:"tickets,omitempty"`
	Bookings  []Booking       `gorm:"foreignKey:EventID" json:"bookings,omitempty"`
}

// Use to track event opening times and check in time -> support multiple-day events
// Constraint:
// end_time - start_time > ? mins (number can be dynamically configured)
// end_checkin_time > start_checkin_time > ? mins (number can be dynamically configured)
// start_checkin_time <= start_time AND end_checkin_time <= end_time
// start_time - created_at > ? days (number can be dynamically configured)
type EventSchedule struct {
	Model
	EventID          uuid.UUID `gorm:"type:uuid;not null" json:"event_id"`
	StartTime        time.Time `gorm:"not null;default:now()" json:"start_time"`
	EndTime          time.Time `gorm:"not null" json:"end_time"`
	StartCheckinTime time.Time `gorm:"not null;default:now()" json:"start_checkin_time"`
	EndCheckinTime   time.Time `gorm:"not null" json:"end_checkin_time"`

	// Relationships
	Event Event `gorm:"foreignKey:EventID" json:"event"`
}

// Seat zone of the event's venue. This will allow event's organiser to set different zone with dedicated rank/privilege
type SeatZone struct {
	Model
	EventID     uuid.UUID `gorm:"type:uuid;not null" json:"event_id"`
	Description string    `gorm:"type:varchar;not null" json:"description"`
	TotalSeats  uint      `gorm:"not null" json:"total_seats"`
	Status      Status    `gorm:"type:varchar;not null;default:published" json:"status"` // draft, published, canceled

	// Relationships
	Event   Event    `gorm:"foreignKey:EventID" json:"event"`
	Seats   []Seat   `gorm:"foreignKey:SeatZoneID" json:"seats"`
	Tickets []Ticket `gorm:"foreignKey:SeatZoneID" json:"tickets"`
}

// Individual seat.
// When a customer book a seat, its status will be `reserved`. Seat that is reserved cannot be booked by anyone.
// If customer reserved that seat for too long without paying, the seat will be release (status = 'empty') and other can pick it
type Seat struct {
	Model
	SeatZoneID uuid.UUID  `gorm:"type:uuid;not null" json:"seat_zone_id"`
	SeatNumber string     `gorm:"type:varchar;not null" json:"seat_number"`
	Status     SeatStatus `gorm:"type:varchar;not null;default:empty" json:"status"` // empty, reserved, booked
	ReservedBy *uuid.UUID `gorm:"type:uuid" json:"reserved_by,omitempty"`

	// Relationships
	SeatZone     SeatZone      `gorm:"foreignKey:SeatZoneID" json:"seat_zone"`
	ReservedUser *User         `gorm:"foreignKey:ReservedBy" json:"reserved_user"`
	BookingItems []BookingItem `gorm:"foreignKey:SeatID" json:"booking_items,omitempty"`
}

// Ticket type. Can be categorize by rank (VIP, standard,...) with privileged. Ticket type will be assign with a seat zone
type Ticket struct {
	Model
	EventID     uuid.UUID `gorm:"type:uuid;not null" json:"event_id"`
	SeatZoneID  uuid.UUID `gorm:"type:uuid;not null" json:"seat_zone_id"`
	Rank        string    `gorm:"type:varchar(20);not null" json:"rank"`
	Description string    `gorm:"type:varchar;not null" json:"description"`
	BasePrice   uint      `gorm:"not null" json:"base_price"`
	Status      Status    `gorm:"type:varchar(20);not null;default:draft" json:"status"` // draft, published, canceled

	// Relationships
	Event            Event                   `gorm:"foreignKey:EventID" json:"event"`
	SeatZone         SeatZone                `gorm:"foreignKey:SeatZoneID" json:"seat_zone"`
	SellingSchedules []TicketSellingSchedule `gorm:"foreignKey:TicketID" json:"selling_schedules,omitempty"`
	BookingItems     []BookingItem           `gorm:"foreignKey:TicketID" json:"booking_items,omitempty"`
}

// Schedule for selling tickets: Allow the same ticket type to be sold multiple times of different timeframe
// Constraint:
// total <= seat_zones.total_seats
// end_selling_time - start_selling_time > ? minutes (number can be dynamically configured)
type TicketSellingSchedule struct {
	Model
	TicketID         uuid.UUID `gorm:"type:uuid;not null" json:"ticket_id"`
	Total            uint      `gorm:"not null" json:"total"`
	Available        uint      `gorm:"not null" json:"available"`
	StartSellingTime time.Time `gorm:"not null;default:now()" json:"start_selling_time"`
	EndSellingTime   time.Time `gorm:"not null" json:"end_selling_time"`
	Status           Status    `gorm:"type:varchar(20);not null;default:draft" json:"status"` // draft, published, canceled

	// Relationships
	Ticket Ticket `gorm:"foreignKey:TicketID" json:"ticket"`
}

// Booking information.
// Constraint: customer_id must have role = `customer`
// Business rules:
// 1. Customer can book multiple tickets/seats, but they must belong to the same event
// 2. When create booking, status must be `pending` (waiting for payment).
// 3. Only when payment success, status -> `complete`. All trigger will happen in this state: seat status -> `booked`, generate QR
// tickets for each booking item,...
// 4. A booking can only be canceled when its status is `pending`. If payment success (status = `complete`), they cannot canceled it
// (customer must use refund function). Even if the booking complete and event get canceled, its status would still be `complete` (it
// still a refund, just different trigger)
// 5.If a booking get in pending state pass a time limit, update status = `timeout`. Release all the reserve seat
// (seat's status = 'empty')
type Booking struct {
	Model
	CustomerID uuid.UUID     `gorm:"type:uuid;not null;index" json:"customer_id"`
	EventID    uuid.UUID     `gorm:"type:uuid;not null" json:"event_id"`
	Status     BookingStatus `gorm:"type:varchar(20);not null;default:pending" json:"status"` // pending, complete, canceled, timeout

	// Relationships
	Customer     User          `gorm:"foreignKey:CustomerID" json:"customer"`
	Event        Event         `gorm:"foreignKey:EventID" json:"event"`
	BookingItems []BookingItem `gorm:"foreignKey:BookingID" json:"booking_items,omitempty"`
	Payments     []Payment     `gorm:"foreignKey:BookingID" json:"payments,omitempty"`
}

// Individual item of a booking, which contains the ticket's ID, seat's ID, QR ticket,...
// Constraint:
// 1. ticket and booking must point to the same event
// 2. seat and ticket must point to the same seat zone
// 3. 0 < price <= base price
// Business rules:
// 1. Only (booking.status = `pending` AND payment.status = `success`) that we'll generate QR. This will make QR become unique, because
// such condition cannot happen
// twice if implement correctly
// 2. `status` is QR's status.
// 3. QR contains an encryption of booking_item_id and a secret key. With the key, hackers/cheaters wouldn't be able to fake QR if they
// don't know the secret key
type BookingItem struct {
	Model
	BookingID uuid.UUID      `gorm:"type:uuid;not null" json:"booking_id"`
	TicketID  uuid.UUID      `gorm:"type:uuid;not null" json:"ticket_id"`
	SeatID    uuid.UUID      `gorm:"type:uuid;not null" json:"seat_id"`
	Price     uint           `gorm:"not null" json:"price"`
	QR        sql.NullString `gorm:"type:varchar;unique" json:"qr"`
	Status    sql.NullString `gorm:"type:varchar(20);default:valid" json:"status"` // valid, expired, used

	// Relationships
	Booking Booking `gorm:"foreignKey:BookingID" json:"booking"`
	Ticket  Ticket  `gorm:"foreignKey:TicketID" json:"ticket"`
	Seat    Seat    `gorm:"foreignKey:SeatID" json:"seat"`
}

// Payment information. This will tie to booking, since we want to pay for a whole booking instead of individual item in a booking.
// In case of payment failed and retry, the relationship to booking should be 1-many, not 1-1.
// `transaction_id` is the ID of payment provider (for Stripe transaction, it would be `payment_intent_id`) which can be used to get
// transaction information,
// refund,...
// `status` will be standardize. For example, if Stripe return status `payment_intent.succeeded` -> `success`. This standardize help if
// we want to integrate other
// payment service
// Business rules:
// 1. The actual price calculation: price = (base_price * (1 + charge)) * (1 - discount) (0 <= charge, discount < 1)
type Payment struct {
	Model
	BookingID      uuid.UUID     `gorm:"type:uuid;not null" json:"booking_id"`
	TransactionID  string        `gorm:"type:varchar;not null" json:"transaction_id"`
	Amount         uint          `gorm:"not null" json:"amount"`
	PaymentGateway string        `gorm:"type:varchar;not null" json:"payment_gateway"`            // Stripe, Paypal
	PaymentMethod  string        `gorm:"type:varchar(50);not null" json:"payment_method"`         // Visa, card, e-wallet
	Status         PaymentStatus `gorm:"type:varchar(20);not null;default:pending" json:"status"` // failed, success

	// Relationships
	Booking        Booking             `gorm:"foreignKey:BookingID" json:"booking"`
	Refunds        []Refund            `gorm:"foreignKey:PaymentID" json:"refunds"`
	MembershipLogs []UserMembershipLog `gorm:"foreignKey:RelatedPaymentID" json:"membership_logs"`
}

// Refund information. Because refund can also fail, its relationship with `payments` is 1-many.
// Constraint: amount <= payments.amount
// Same case with payments.status, the value will also be standardize
// Reason can be simple with 2 values: user_canceled and host_canceled
// Business rule:
// 1. If user_canceled, and if the refund happen in ?? hours after the payment is made, then they can have a full refund. If pass that
// timeframe, they can only have a partial refund = ?? % amount (number can be dynamically configured)
// 2. If host_canceled, it always a full refund
type Refund struct {
	Model
	PaymentID uuid.UUID     `gorm:"type:uuid;not null" json:"payment_id"`
	Amount    uint          `gorm:"not null" json:"amount"`
	Reason    string        `gorm:"type:varchar(20);not null;default:user_canceled" json:"reason"` // user_canceled, host_canceled
	Status    PaymentStatus `gorm:"type:varchar(20);not null;default:pending" json:"status"`       // failed, success

	// Relationships
	Payment Payment `gorm:"foreignKey:PaymentID" json:"payment"`
}

// Check in. To avoid double checkin, we keep the booking_item_id as unique, and enforce 1-1 relationship with booking_items
// Business rule/flow:
// 1. Staff will scan the QR code present by customer
// 2. QR redirect staff to a intermediary page. In this page, the staff would fill the checkin information (staff username -> can be
// inferred from cookie, device ID,
// gate,...) and add their password
// (their account password), then send the actual request (include the token that embed in the original QR and staff information) to
// server
// 3. Server will first check if the staff is valid -> fetch staff information, check password (BCrypt compare)
// 4. Server then will check if the token is valid (check booking_item_id if exists, secret correct)
// 5. Proceed with checkin if the above conditions met
// Pros and Cons:
// Pros: Allow for data collection (staff, device, checkin location or whatever data you want to collect)
// Pros: adding security, since only staff would be able to proceed with the checkin by requiring staff's password -> user cannot scan
// the QR at home.
// Pros: hiding server API. Customer wouldn't be able to get the actual API for checking even if they can decrypt the QR
// Cons: not complete automate, still need human interaction
type Checkin struct {
	Model
	StaffID       uuid.UUID `gorm:"type:uuid;not null" json:"staff_id"`
	BookingItemID uuid.UUID `gorm:"type:uuid;not null;unique" json:"booking_item_id"`

	// Relationships
	Staff       User        `gorm:"foreignKey:StaffID" json:"staff"`
	BookingItem BookingItem `gorm:"foreignKey:BookingItemID" json:"booking_item"`
}

// Notification category, can be used to categorized notification template
type NotificationCategory struct {
	Model
	Name        string `gorm:"type:varchar;not null" json:"name"`
	Description string `gorm:"type:varchar;not null" json:"description"`
	Status      Status `gorm:"type:varchar;not null;default:draft" json:"status"` // draft, published, canceled

	// Relationships
	Templates []NotificationTemplate `gorm:"foreignKey:TemplateCategoryID" json:"templates,omitempty"`
}

// Dynamic notification template.
// The template string should be an HTML (that can be embeded in both email and Telegram)
type NotificationTemplate struct {
	Model
	Name               string    `gorm:"type:varchar;not null" json:"name"`
	TemplateCategoryID uuid.UUID `gorm:"type:uuid;not null" json:"template_category_id"`
	SubjectTmpl        string    `gorm:"type:varchar;not null" json:"subject_tmpl"`
	BodyTmpl           string    `gorm:"type:varchar;not null" json:"body_tmpl"`
	Version            string    `gorm:"type:varchar;not null" json:"version"`
	IsUsed             bool      `gorm:"not null;default:false" json:"is_used"`
	Status             Status    `gorm:"type:varchar;not null;default:draft" json:"status"` // draft, published, canceled

	// Relationships
	Category      NotificationCategory `gorm:"foreignKey:TemplateCategoryID" json:"category"`
	Notifications []Notification       `gorm:"foreignKey:TemplateID" json:"notifications"`
}

// The actual notification data
type Notification struct {
	Model
	TemplateID uuid.UUID `gorm:"type:uuid;not null" json:"template_id"`
	Data       string    `gorm:"type:jsonb;not null" json:"data"`
	Priority   string    `gorm:"type:varchar;not null;default:normal" json:"priority"` // normal, critical

	// Relationships
	Template  NotificationTemplate   `gorm:"foreignKey:TemplateID" json:"template"`
	Recipents []NotificationRecipent `gorm:"foreignKey:NotificationID" json:"recipents"`
}

// The recipents (receivers) of notification
type NotificationRecipent struct {
	Model
	NotificationID uuid.UUID `gorm:"type:uuid;not null" json:"notification_id"`
	ReceiverID     uuid.UUID `gorm:"type:uuid;not null" json:"receiver_id"`

	// Relationships
	Notification Notification `gorm:"foreignKey:NotificationID" json:"notification"`
	Receiver     User         `gorm:"foreignKey:ReceiverID" json:"receiver"`
}

// NotificationInAppChannel represents in-app notification delivery
type NotificationInAppChannel struct {
	ID         uuid.UUID          `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	RecipentID uuid.UUID          `gorm:"type:uuid;not null" json:"recipent_id"`
	Status     NotificationStatus `gorm:"type:varchar;not null;default:queued" json:"status"` // queued, failed, sent, received, read
	ReadAt     *time.Time         `gorm:"type:timestamp" json:"read_at,omitempty"`

	// Relationships
	Recipent NotificationRecipent `gorm:"foreignKey:RecipentID" json:"recipent"`
}

// UserTelegram tracks user Telegram chat IDs
type UserTelegram struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	TelegramChatID string    `gorm:"type:varchar;not null" json:"telegram_chat_id"`

	// Relationships
	User             User                          `gorm:"foreignKey:UserID" json:"user"`
	TelegramChannels []NotificationTelegramChannel `gorm:"foreignKey:TelegramChatID" json:"telegram_channels,omitempty"`
}

// NotificationTelegramChannel represents Telegram notification delivery
type NotificationTelegramChannel struct {
	ID             uuid.UUID          `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	RecipentID     uuid.UUID          `gorm:"type:uuid;not null" json:"recipent_id"`
	TelegramChatID uuid.UUID          `gorm:"type:uuid;not null" json:"telegram_chat_id"`
	Status         NotificationStatus `gorm:"type:varchar;not null;default:queued" json:"status"` // queued, sent, failed

	// Relationships
	Recipent     NotificationRecipent `gorm:"foreignKey:RecipentID" json:"recipent"`
	UserTelegram UserTelegram         `gorm:"foreignKey:TelegramChatID" json:"user_telegram"`
}

// NotificationEmailChannel represents email notification delivery
type NotificationEmailChannel struct {
	ID         uuid.UUID          `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	RecipentID uuid.UUID          `gorm:"type:uuid;not null" json:"recipent_id"`
	Status     NotificationStatus `gorm:"type:varchar;not null;default:queued" json:"status"` // queued, sent, failed

	// Relationships
	Recipent NotificationRecipent `gorm:"foreignKey:RecipentID" json:"recipent"`
}

// Setting represents system configuration
type Setting struct {
	Model
	MoneyToPointRate          int     `gorm:"not null;default:10000" json:"money_to_point_rate"`
	MinEventDurationMinutes   int     `gorm:"not null;default:45" json:"min_event_duration_minutes"`
	MinEventLeadDays          int     `gorm:"not null;default:14" json:"min_event_lead_days"`
	MaxReservationHoldMinutes int     `gorm:"not null;default:5" json:"max_reservation_hold_minutes"`
	MinSellingDurationMinutes int     `gorm:"not null;default:30" json:"min_selling_duration_minutes"`
	PaymentFeePercent         float64 `gorm:"type:decimal(10,2);not null;default:5.00" json:"payment_fee_percent"`
	MaxFullRefundHours        int     `gorm:"not null;default:24" json:"max_full_refund_hours"`
	SystemEmail               string  `gorm:"type:varchar;not null" json:"system_email"`
	Version                   string  `gorm:"type:varchar;not null" json:"version"`
	InUsed                    bool    `gorm:"not null;default:false" json:"in_used"`
	Status                    Status  `gorm:"type:varchar;not null;default:draft" json:"status"` // draft, published, canceled
}
