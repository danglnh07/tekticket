package db

import (
	"time"

	"github.com/google/uuid"
)

// User represents the users table in the database
type User struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username     string    `gorm:"size:50;uniqueIndex;not null" json:"username"`
	Email        string    `gorm:"size:50;uniqueIndex;not null" json:"email"`
	Phone        string    `gorm:"size:15;uniqueIndex;not null" json:"phone"`
	Password     string    `gorm:"size:100;not null" json:"-"` // store bcrypt hash
	Avatar       string    `gorm:"not null" json:"avatar"`
	Role         string    `gorm:"not null;default:customer" json:"role"`
	Status       string    `gorm:"size:20;not null;default:inactive" json:"status"`
	TokenVersion int       `gorm:"not null;default:0" json:"token_version"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
