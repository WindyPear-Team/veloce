package model

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	CompanyObjectiveStatusActive    = "active"
	CompanyObjectiveStatusPaused    = "paused"
	CompanyObjectiveStatusCompleted = "completed"
	CompanyObjectiveStatusArchived  = "archived"

	CompanyWorkStatusInbox          = "inbox"
	CompanyWorkStatusPlanned        = "planned"
	CompanyWorkStatusOwnerDecision  = "owner_decision"
	CompanyWorkStatusAuthorized     = "authorized"
	CompanyWorkStatusQueued         = "queued"
	CompanyWorkStatusExecuting      = "executing"
	CompanyWorkStatusAwaitingReview = "awaiting_review"
	CompanyWorkStatusVerified       = "verified"
	CompanyWorkStatusDelivered      = "delivered"
	CompanyWorkStatusBlocked        = "blocked"
	CompanyWorkStatusCancelled      = "cancelled"
)

type CompanyObjective struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"index;not null" json:"personal_company_id"`
	OwnerUserID       uint           `gorm:"index;not null" json:"owner_user_id"`
	Title             string         `gorm:"size:200;not null" json:"title"`
	Description       string         `gorm:"type:text;not null;default:''" json:"description"`
	Status            string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	Priority          int            `gorm:"not null;default:0;index" json:"priority"`
	TargetDate        *time.Time     `gorm:"index" json:"target_date,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyWorkItem is the durable business commitment. Execution attempts are
// recorded separately so retry and recovery never overwrite the commitment.
type CompanyWorkItem struct {
	ID                 uint            `gorm:"primaryKey" json:"id"`
	PersonalCompanyID  uint            `gorm:"uniqueIndex:idx_company_work_idempotency;index;not null" json:"personal_company_id"`
	OwnerUserID        uint            `gorm:"index;not null" json:"owner_user_id"`
	ObjectiveID        *uint           `gorm:"index" json:"objective_id,omitempty"`
	Title              string          `gorm:"size:200;not null" json:"title"`
	Description        string          `gorm:"type:text;not null;default:''" json:"description"`
	DefinitionOfDone   string          `gorm:"type:text;not null" json:"definition_of_done"`
	Status             string          `gorm:"size:24;not null;default:'planned';index" json:"status"`
	Priority           int             `gorm:"not null;default:0;index" json:"priority"`
	RiskLevel          string          `gorm:"size:4;not null;default:'r0';index" json:"risk_level"`
	IdempotencyKey     string          `gorm:"uniqueIndex:idx_company_work_idempotency;size:100;not null" json:"idempotency_key"`
	InputSnapshot      string          `gorm:"type:text;not null;default:'{}'" json:"input_snapshot"`
	AllowedTools       string          `gorm:"type:text;not null;default:'[]'" json:"allowed_tools"`
	AssignedEmployeeID *uint           `gorm:"index" json:"assigned_employee_id,omitempty"`
	ReviewerEmployeeID *uint           `gorm:"index" json:"reviewer_employee_id,omitempty"`
	EstimatedCost      decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"estimated_cost"`
	ReservedCost       decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"reserved_cost"`
	ConsumedCost       decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"consumed_cost"`
	DueAt              *time.Time      `gorm:"index" json:"due_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	DeletedAt          gorm.DeletedAt  `gorm:"index" json:"-"`
}

type CompanyWorkAttempt struct {
	ID                uint            `gorm:"primaryKey" json:"id"`
	WorkItemID        uint            `gorm:"uniqueIndex:idx_company_work_attempt;index;not null" json:"work_item_id"`
	AttemptNumber     int             `gorm:"uniqueIndex:idx_company_work_attempt;not null" json:"attempt_number"`
	AdvancedChatRunID string          `gorm:"index;size:80;not null;default:''" json:"advanced_chat_run_id,omitempty"`
	Status            string          `gorm:"size:24;not null;default:'queued';index" json:"status"`
	LeaseToken        string          `gorm:"index;size:100;not null;default:''" json:"-"`
	LeaseExpiresAt    *time.Time      `gorm:"index" json:"lease_expires_at,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
	InputSnapshot     string          `gorm:"type:text;not null;default:'{}'" json:"input_snapshot"`
	ResultSummary     string          `gorm:"type:text;not null;default:''" json:"result_summary"`
	Cost              decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"cost"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CompanyArtifact struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	WorkItemID      uint           `gorm:"index;not null" json:"work_item_id"`
	WorkAttemptID   *uint          `gorm:"index" json:"work_attempt_id,omitempty"`
	Kind            string         `gorm:"size:40;not null" json:"kind"`
	URI             string         `gorm:"type:text;not null" json:"uri"`
	ContentHash     string         `gorm:"size:128;not null;default:''" json:"content_hash"`
	Source          string         `gorm:"type:text;not null;default:''" json:"source"`
	AcceptanceState string         `gorm:"size:24;not null;default:'pending';index" json:"acceptance_state"`
	CreatedAt       time.Time      `json:"created_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyHandoffPackage makes cross-employee transfers explicit and auditable.
// A future worker will require a receiving employee to accept it before the
// downstream work item can leave review.
type CompanyHandoffPackage struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	WorkItemID        uint           `gorm:"index;not null" json:"work_item_id"`
	WorkAttemptID     *uint          `gorm:"index" json:"work_attempt_id,omitempty"`
	FromEmployeeID    *uint          `gorm:"index" json:"from_employee_id,omitempty"`
	ToEmployeeID      *uint          `gorm:"index" json:"to_employee_id,omitempty"`
	Status            string         `gorm:"size:24;not null;default:'pending';index" json:"status"`
	CompletionSummary string         `gorm:"type:text;not null;default:''" json:"completion_summary"`
	Evidence          string         `gorm:"type:text;not null;default:'[]'" json:"evidence"`
	Risks             string         `gorm:"type:text;not null;default:'[]'" json:"risks"`
	NextSteps         string         `gorm:"type:text;not null;default:''" json:"next_steps"`
	AcceptedAt        *time.Time     `json:"accepted_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func NormalizeCompanyObjectiveStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CompanyObjectiveStatusPaused, CompanyObjectiveStatusCompleted, CompanyObjectiveStatusArchived:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return CompanyObjectiveStatusActive
	}
}

func NormalizeCompanyWorkStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CompanyWorkStatusInbox, CompanyWorkStatusPlanned, CompanyWorkStatusOwnerDecision, CompanyWorkStatusAuthorized,
		CompanyWorkStatusQueued, CompanyWorkStatusExecuting, CompanyWorkStatusAwaitingReview, CompanyWorkStatusVerified,
		CompanyWorkStatusDelivered, CompanyWorkStatusBlocked, CompanyWorkStatusCancelled:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return CompanyWorkStatusPlanned
	}
}
