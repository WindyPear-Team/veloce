package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	OrganizationStatusActive    = "active"
	OrganizationStatusSuspended = "suspended"

	OrganizationMemberRoleOwner   = "owner"
	OrganizationMemberRoleAdmin   = "admin"
	OrganizationMemberRoleMember  = "member"
	OrganizationMemberRoleAuditor = "auditor"

	OrganizationMemberStatusInvited   = "invited"
	OrganizationMemberStatusActive    = "active"
	OrganizationMemberStatusSuspended = "suspended"

	WorkspaceTypePersonal   = "personal"
	WorkspaceTypeDepartment = "department"
	WorkspaceTypeProject    = "project"

	WorkspaceStatusActive   = "active"
	WorkspaceStatusArchived = "archived"

	WorkspaceMemberRoleOwner  = "owner"
	WorkspaceMemberRoleAdmin  = "admin"
	WorkspaceMemberRoleMember = "member"
	WorkspaceMemberRoleViewer = "viewer"

	ResourceVisibilityPersonal     = "personal"
	ResourceVisibilityWorkspace    = "workspace"
	ResourceVisibilityOrganization = "organization"
)

// Organization is the single enterprise profile for this private deployment.
// Slug is a stable, human-readable identifier used by URLs and API clients.
// CreatedByUserID intentionally remains a scalar during the first migration
// step; membership and ownership relationships are introduced separately.
type Organization struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Slug            string         `gorm:"uniqueIndex;size:80;not null" json:"slug"`
	Name            string         `gorm:"size:160;not null" json:"name"`
	Description     string         `gorm:"type:text;not null;default:''" json:"description"`
	Status          string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	CreatedByUserID uint           `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// OrganizationMember assigns a user to an organization with an initial
// organization-scoped role. Fine-grained RBAC bindings will be layered on top
// of this bootstrap role in a later migration.
type OrganizationMember struct {
	ID             uint         `gorm:"primaryKey" json:"id"`
	OrganizationID uint         `gorm:"uniqueIndex:idx_organization_member;index;not null" json:"organization_id"`
	Organization   Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	UserID         uint         `gorm:"uniqueIndex:idx_organization_member;index;not null" json:"user_id"`
	// User is returned with employee-management responses so the UI can show
	// the real account name rather than an internal user ID. Sensitive user
	// fields (password and API key) are excluded by User's own JSON tags.
	User      User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"user"`
	Role      string         `gorm:"size:20;not null;default:'member';index" json:"role"`
	Status    string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	JoinedAt  *time.Time     `json:"joined_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Department models the enterprise department tree.
type Department struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	OrganizationID uint           `gorm:"uniqueIndex:idx_department_org_slug;index;not null" json:"organization_id"`
	Organization   Organization   `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	ParentID       *uint          `gorm:"index" json:"parent_id,omitempty"`
	Parent         *Department    `gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	Slug           string         `gorm:"uniqueIndex:idx_department_org_slug;size:80;not null" json:"slug"`
	Name           string         `gorm:"size:160;not null" json:"name"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// Workspace is the collaboration and resource-sharing boundary inside an
// organization. DepartmentID is optional because personal and project
// workspaces do not necessarily belong to a department.
type Workspace struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	OrganizationID  uint           `gorm:"uniqueIndex:idx_workspace_org_slug;index;not null" json:"organization_id"`
	Organization    Organization   `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	DepartmentID    *uint          `gorm:"index" json:"department_id,omitempty"`
	Department      *Department    `gorm:"foreignKey:DepartmentID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	Slug            string         `gorm:"uniqueIndex:idx_workspace_org_slug;size:80;not null" json:"slug"`
	Name            string         `gorm:"size:160;not null" json:"name"`
	Type            string         `gorm:"size:20;not null;default:'project';index" json:"type"`
	Status          string         `gorm:"size:20;not null;default:'active';index" json:"status"`
	CreatedByUserID uint           `gorm:"index;not null" json:"created_by_user_id"`
	CreatedByUser   User           `gorm:"foreignKey:CreatedByUserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// WorkspaceMember assigns a user to a workspace independently of their
// organization bootstrap role.
type WorkspaceMember struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	WorkspaceID uint           `gorm:"uniqueIndex:idx_workspace_member;index;not null" json:"workspace_id"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	UserID      uint           `gorm:"uniqueIndex:idx_workspace_member;index;not null" json:"user_id"`
	User        User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Role        string         `gorm:"size:20;not null;default:'member';index" json:"role"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func NormalizeOrganizationStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OrganizationStatusSuspended:
		return OrganizationStatusSuspended
	default:
		return OrganizationStatusActive
	}
}

func NormalizeOrganizationMemberRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OrganizationMemberRoleOwner:
		return OrganizationMemberRoleOwner
	case OrganizationMemberRoleAdmin:
		return OrganizationMemberRoleAdmin
	case OrganizationMemberRoleAuditor:
		return OrganizationMemberRoleAuditor
	default:
		return OrganizationMemberRoleMember
	}
}

func NormalizeOrganizationMemberStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OrganizationMemberStatusInvited:
		return OrganizationMemberStatusInvited
	case OrganizationMemberStatusSuspended:
		return OrganizationMemberStatusSuspended
	default:
		return OrganizationMemberStatusActive
	}
}

func NormalizeWorkspaceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WorkspaceTypePersonal:
		return WorkspaceTypePersonal
	case WorkspaceTypeDepartment:
		return WorkspaceTypeDepartment
	default:
		return WorkspaceTypeProject
	}
}

func NormalizeWorkspaceStatus(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), WorkspaceStatusArchived) {
		return WorkspaceStatusArchived
	}
	return WorkspaceStatusActive
}

func NormalizeWorkspaceMemberRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WorkspaceMemberRoleOwner:
		return WorkspaceMemberRoleOwner
	case WorkspaceMemberRoleAdmin:
		return WorkspaceMemberRoleAdmin
	case WorkspaceMemberRoleViewer:
		return WorkspaceMemberRoleViewer
	default:
		return WorkspaceMemberRoleMember
	}
}

func NormalizeResourceVisibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ResourceVisibilityWorkspace:
		return ResourceVisibilityWorkspace
	case ResourceVisibilityOrganization:
		return ResourceVisibilityOrganization
	default:
		return ResourceVisibilityPersonal
	}
}

// ScopeOrganization applies an explicit top-level tenant boundary to a GORM
// query. Enterprise repositories should compose this scope instead of issuing
// unscoped resource queries.
func ScopeOrganization(organizationID uint) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if organizationID == 0 {
			return db.Where("1 = 0")
		}
		return db.Where("organization_id = ?", organizationID)
	}
}

// ScopeWorkspace applies both organization and workspace boundaries. Requiring
// both identifiers prevents a workspace ID from being accidentally reused
// outside the already resolved organization context.
func ScopeWorkspace(organizationID, workspaceID uint) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if organizationID == 0 || workspaceID == 0 {
			return db.Where("1 = 0")
		}
		return db.Where("organization_id = ? AND workspace_id = ?", organizationID, workspaceID)
	}
}
