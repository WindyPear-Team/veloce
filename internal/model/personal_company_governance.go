package model

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	CompanyApprovalPending  = "pending"
	CompanyApprovalApproved = "approved"
	CompanyApprovalRejected = "rejected"
	CompanyApprovalExpired  = "expired"
)

type CompanyApprovalRequest struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint       `gorm:"index;not null" json:"personal_company_id"`
	WorkItemID        uint       `gorm:"uniqueIndex;index;not null" json:"work_item_id"`
	RiskLevel         string     `gorm:"size:4;not null;index" json:"risk_level"`
	Status            string     `gorm:"size:20;not null;default:'pending';index" json:"status"`
	RequestedAction   string     `gorm:"type:text;not null" json:"requested_action"`
	ParametersHash    string     `gorm:"size:128;not null;default:''" json:"parameters_hash"`
	DecisionReason    string     `gorm:"type:text;not null;default:''" json:"decision_reason"`
	DecidedByUserID   *uint      `gorm:"index" json:"decided_by_user_id,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	ExpiresAt         *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CompanyBudgetLedger is append-only. Balance is derived from entries and the
// company limits, rather than mutable counters controlled by an agent.
type CompanyBudgetLedger struct {
	ID                uint            `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint            `gorm:"index;not null" json:"personal_company_id"`
	WorkItemID        *uint           `gorm:"index" json:"work_item_id,omitempty"`
	WorkAttemptID     *uint           `gorm:"index" json:"work_attempt_id,omitempty"`
	EntryType         string          `gorm:"size:24;not null;index" json:"entry_type"`
	Amount            decimal.Decimal `gorm:"type:decimal(20,6);not null" json:"amount"`
	ReferenceType     string          `gorm:"size:40;not null;default:''" json:"reference_type"`
	ReferenceID       string          `gorm:"size:100;not null;default:''" json:"reference_id"`
	CreatedByUserID   uint            `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt         time.Time       `json:"created_at"`
}

type CompanyAuditEvent struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint      `gorm:"index;not null" json:"personal_company_id"`
	WorkItemID        *uint     `gorm:"index" json:"work_item_id,omitempty"`
	ActorType         string    `gorm:"size:20;not null" json:"actor_type"`
	ActorID           string    `gorm:"size:100;not null;default:''" json:"actor_id"`
	EventType         string    `gorm:"size:80;not null;index" json:"event_type"`
	Payload           string    `gorm:"type:text;not null;default:'{}'" json:"payload"`
	CreatedAt         time.Time `json:"created_at"`
}

func NormalizeCompanyApprovalStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CompanyApprovalApproved, CompanyApprovalRejected, CompanyApprovalExpired:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return CompanyApprovalPending
	}
}
