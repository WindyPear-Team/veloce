package model

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	EnterpriseTaskStatusDraft     = "draft"
	EnterpriseTaskStatusAssigned  = "assigned"
	EnterpriseTaskStatusRunning   = "running"
	EnterpriseTaskStatusBlocked   = "blocked"
	EnterpriseTaskStatusCompleted = "completed"
	EnterpriseTaskStatusCancelled = "cancelled"

	EnterpriseTaskAssignmentOwner       = "owner"
	EnterpriseTaskAssignmentAssignee    = "assignee"
	EnterpriseTaskAssignmentParticipant = "participant"

	EnterpriseDeviceStatusActive         = "active"
	EnterpriseDeviceStatusDisabled       = "disabled"
	EnterpriseDeviceAssignmentDepartment = "department"
	EnterpriseDeviceAssignmentUser       = "user"
	EnterpriseDeviceAssignmentTask       = "task"
	EnterpriseDeviceAssignmentActive     = "active"
	EnterpriseDeviceAssignmentRevoked    = "revoked"

	QuotaScopeOrganization        = "organization"
	QuotaScopeDepartment          = "department"
	QuotaScopeUser                = "user"
	QuotaScopeTask                = "task"
	QuotaLedgerAllocation         = "allocation"
	QuotaLedgerReservation        = "reservation"
	QuotaLedgerConsumption        = "consumption"
	QuotaLedgerRelease            = "release"
	EnterprisePoolScopeTask       = "task"
	EnterprisePoolScopeDepartment = "department"
)

// EnterpriseTask is the enterprise execution and accounting boundary. Existing
// chat runs, schedules, and Studio work are migrated into it incrementally.
type EnterpriseTask struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	OrganizationID  uint           `gorm:"index;not null" json:"organization_id"`
	DepartmentID    *uint          `gorm:"index" json:"department_id,omitempty"`
	WorkspaceID     *uint          `gorm:"index" json:"workspace_id,omitempty"`
	CreatedByUserID uint           `gorm:"index;not null" json:"created_by_user_id"`
	OwnerUserID     uint           `gorm:"index;not null" json:"owner_user_id"`
	ParentTaskID    *uint          `gorm:"index" json:"parent_task_id,omitempty"`
	Title           string         `gorm:"size:200;not null" json:"title"`
	Description     string         `gorm:"type:text;not null;default:''" json:"description"`
	Status          string         `gorm:"size:20;not null;default:'draft';index" json:"status"`
	Priority        int            `gorm:"not null;default:0;index" json:"priority"`
	DueAt           *time.Time     `gorm:"index" json:"due_at,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type EnterpriseTaskAssignment struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	TaskID     uint           `gorm:"uniqueIndex:idx_enterprise_task_assignment;index;not null" json:"task_id"`
	UserID     uint           `gorm:"uniqueIndex:idx_enterprise_task_assignment;index;not null" json:"user_id"`
	Role       string         `gorm:"uniqueIndex:idx_enterprise_task_assignment;size:20;not null" json:"role"`
	AssignedBy uint           `gorm:"index;not null" json:"assigned_by"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// DepartmentMember is the explicit audience for department-scoped shared
// pools. Organization membership alone does not grant department data access.
type DepartmentMember struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	OrganizationID uint           `gorm:"uniqueIndex:idx_department_member;index;not null" json:"organization_id"`
	DepartmentID   uint           `gorm:"uniqueIndex:idx_department_member;index;not null" json:"department_id"`
	UserID         uint           `gorm:"uniqueIndex:idx_department_member;index;not null" json:"user_id"`
	CreatedAt      time.Time      `json:"created_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// EnterpriseSharedPool provides a task or department-level audience for chat
// sessions and uploaded files. The bindings retain source ownership while the
// pool defines who may discover and reuse them.
type EnterpriseSharedPool struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	OrganizationID  uint           `gorm:"uniqueIndex:idx_enterprise_pool_scope;index;not null" json:"organization_id"`
	ScopeType       string         `gorm:"uniqueIndex:idx_enterprise_pool_scope;size:20;not null" json:"scope_type"`
	ScopeKey        string         `gorm:"uniqueIndex:idx_enterprise_pool_scope;size:80;not null" json:"scope_key"`
	DepartmentID    *uint          `gorm:"index" json:"department_id,omitempty"`
	TaskID          *uint          `gorm:"index" json:"task_id,omitempty"`
	Name            string         `gorm:"size:160;not null" json:"name"`
	CreatedByUserID uint           `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type EnterpriseSharedSession struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	PoolID    uint           `gorm:"uniqueIndex:idx_enterprise_pool_session;index;not null" json:"pool_id"`
	SessionID string         `gorm:"uniqueIndex:idx_enterprise_pool_session;size:80;not null" json:"session_id"`
	SharedBy  uint           `gorm:"index;not null" json:"shared_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type EnterpriseSharedFile struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	PoolID    uint           `gorm:"uniqueIndex:idx_enterprise_pool_file;index;not null" json:"pool_id"`
	FileID    string         `gorm:"uniqueIndex:idx_enterprise_pool_file;size:80;not null" json:"file_id"`
	SharedBy  uint           `gorm:"index;not null" json:"shared_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type EnterpriseDevice struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	OrganizationID      uint           `gorm:"uniqueIndex:idx_enterprise_device_org_external;index;not null" json:"organization_id"`
	ExternalDeviceID    string         `gorm:"uniqueIndex:idx_enterprise_device_org_external;size:100;not null" json:"external_device_id"`
	Name                string         `gorm:"size:160;not null" json:"name"`
	Kind                string         `gorm:"size:40;not null;default:'connector'" json:"kind"`
	OwnerUserID         *uint          `gorm:"index" json:"owner_user_id,omitempty"`
	ManagedByEnterprise bool           `gorm:"not null;default:true" json:"managed_by_enterprise"`
	Status              string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type EnterpriseDeviceAssignment struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	OrganizationID uint           `gorm:"index;not null" json:"organization_id"`
	DeviceID       uint           `gorm:"index;not null" json:"device_id"`
	ScopeType      string         `gorm:"size:20;not null;index" json:"scope_type"`
	DepartmentID   *uint          `gorm:"index" json:"department_id,omitempty"`
	UserID         *uint          `gorm:"index" json:"user_id,omitempty"`
	TaskID         *uint          `gorm:"index" json:"task_id,omitempty"`
	AllowedTools   string         `gorm:"type:text;not null;default:'[]'" json:"allowed_tools"`
	Classification string         `gorm:"size:40;not null;default:''" json:"classification"`
	Status         string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	AssignedBy     uint           `gorm:"index;not null" json:"assigned_by"`
	ExpiresAt      *time.Time     `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// QuotaAccount represents an allocatable budget at one organizational scope.
// ScopeKey makes its uniqueness portable across SQLite, PostgreSQL and MySQL.
type QuotaAccount struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	OrganizationID uint            `gorm:"uniqueIndex:idx_quota_account_scope;index;not null" json:"organization_id"`
	ScopeType      string          `gorm:"uniqueIndex:idx_quota_account_scope;size:20;not null" json:"scope_type"`
	ScopeKey       string          `gorm:"uniqueIndex:idx_quota_account_scope;size:80;not null" json:"scope_key"`
	DepartmentID   *uint           `gorm:"index" json:"department_id,omitempty"`
	UserID         *uint           `gorm:"index" json:"user_id,omitempty"`
	TaskID         *uint           `gorm:"index" json:"task_id,omitempty"`
	LimitAmount    decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"limit_amount"`
	ReservedAmount decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"reserved_amount"`
	ConsumedAmount decimal.Decimal `gorm:"type:decimal(20,6);not null;default:0" json:"consumed_amount"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type QuotaLedger struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	OrganizationID  uint            `gorm:"index;not null" json:"organization_id"`
	AccountID       uint            `gorm:"index;not null" json:"account_id"`
	TaskID          *uint           `gorm:"index" json:"task_id,omitempty"`
	EntryType       string          `gorm:"size:20;not null;index" json:"entry_type"`
	Amount          decimal.Decimal `gorm:"type:decimal(20,6);not null" json:"amount"`
	ReferenceType   string          `gorm:"size:40;not null;default:''" json:"reference_type"`
	ReferenceID     string          `gorm:"size:100;not null;default:''" json:"reference_id"`
	CreatedByUserID uint            `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt       time.Time       `json:"created_at"`
}

func NormalizeEnterpriseTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EnterpriseTaskStatusAssigned, EnterpriseTaskStatusRunning, EnterpriseTaskStatusBlocked, EnterpriseTaskStatusCompleted, EnterpriseTaskStatusCancelled:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return EnterpriseTaskStatusDraft
	}
}

func DeviceAssignmentScopeValid(scope string, departmentID, userID, taskID *uint) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case EnterpriseDeviceAssignmentDepartment:
		return departmentID != nil && *departmentID != 0 && userID == nil && taskID == nil
	case EnterpriseDeviceAssignmentUser:
		return userID != nil && *userID != 0 && departmentID == nil && taskID == nil
	case EnterpriseDeviceAssignmentTask:
		return taskID != nil && *taskID != 0 && departmentID == nil && userID == nil
	default:
		return false
	}
}
