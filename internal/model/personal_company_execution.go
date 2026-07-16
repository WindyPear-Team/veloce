package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	CompanySignalStatusInbox     = "inbox"
	CompanySignalStatusTriaged   = "triaged"
	CompanySignalStatusIgnored   = "ignored"
	CompanyOutboxStatusPending   = "pending"
	CompanyOutboxStatusPublished = "published"
)

// CompanySignal is the durable entry point for schedules, owner requests and
// monitor events. Signals are never direct execution commands.
type CompanySignal struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"uniqueIndex:idx_company_signal_dedup;index;not null" json:"personal_company_id"`
	OwnerUserID       uint           `gorm:"index;not null" json:"owner_user_id"`
	Source            string         `gorm:"size:40;not null" json:"source"`
	DeduplicationKey  string         `gorm:"uniqueIndex:idx_company_signal_dedup;size:120;not null" json:"deduplication_key"`
	Payload           string         `gorm:"type:text;not null;default:'{}'" json:"payload"`
	Status            string         `gorm:"size:20;not null;default:'inbox';index" json:"status"`
	WorkItemID        *uint          `gorm:"index" json:"work_item_id,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyOutboxEvent is written in the same transaction as a state change so
// a future worker can publish reliable notifications without losing the fact.
type CompanyOutboxEvent struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"uniqueIndex:idx_company_outbox_key;index;not null" json:"personal_company_id"`
	EventKey          string         `gorm:"uniqueIndex:idx_company_outbox_key;size:140;not null" json:"event_key"`
	EventType         string         `gorm:"size:80;not null;index" json:"event_type"`
	Payload           string         `gorm:"type:text;not null;default:'{}'" json:"payload"`
	Status            string         `gorm:"size:20;not null;default:'pending';index" json:"status"`
	PublishedAt       *time.Time     `json:"published_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}
