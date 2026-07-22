package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	TicketStatusOpen     = "open"
	TicketStatusPending  = "pending"
	TicketStatusAnswered = "answered"
	TicketStatusClosed   = "closed"
)

// Ticket is a support request opened by a user from the dashboard.
type Ticket struct {
	ID        uint            `gorm:"primaryKey" json:"id"`
	UserID    uint            `gorm:"index;not null" json:"user_id"`
	Subject   string          `gorm:"size:160;not null" json:"subject"`
	Category  string          `gorm:"size:80;not null;default:'general'" json:"category"`
	Priority  string          `gorm:"size:20;not null;default:'normal'" json:"priority"`
	Status    string          `gorm:"size:20;not null;default:'open';index" json:"status"`
	ClosedAt  *time.Time      `json:"closed_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `gorm:"index" json:"-"`
	Messages  []TicketMessage `json:"messages,omitempty"`
}

// TicketMessage is a conversation entry made by the requester or support staff.
type TicketMessage struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	TicketID  uint           `gorm:"index;not null" json:"ticket_id"`
	UserID    uint           `gorm:"index;not null" json:"user_id"`
	IsStaff   bool           `gorm:"not null;default:false" json:"is_staff"`
	Content   string         `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
