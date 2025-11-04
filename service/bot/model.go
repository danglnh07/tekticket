package bot

// Telegram response
type TelegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      any    `json:"result,omitempty"`
}

// Telegram webhook information
type Webhook struct {
	URL                  string `json:"url"`
	HasCustomCertificate bool   `json:"has_customer_certificate"`
	MaxConnections       int    `json:"max_connections"`
	IPAddress            string `json:"ip_address"`
}

// Bot info struct
type BotInfo struct {
	ID        float64 `json:"id"`
	FirstName string  `json:"first_name"`
	Username  string  `json:"username"`
}

// Telegram bot command struct
type Command struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// Chat Type constant
type ChatType string

const (
	PRIVATE    ChatType = "private"
	GROUP      ChatType = "group"
	SUPERGROUP ChatType = "supergroup"
	CHANNEL    ChatType = "channel"
)

// Chat struct, represent a Chat instance
type Chat struct {
	ID   int      `json:"id"`
	Type ChatType `json:"type"`
}

// Message struct
type Message struct {
	ID   int    `json:"message_id"`
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}

// Update object: represent any update (for example, client message/command the bot)
type TelegramUpdate struct {
	ID      int     `json:"update_id"`
	Message Message `json:"message"`
}
