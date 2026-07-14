package model

import (
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	RoleBindingScopeOrganization = "organization"
	RoleBindingScopeWorkspace    = "workspace"

	PermissionActionRead    = "read"
	PermissionActionUse     = "use"
	PermissionActionCreate  = "create"
	PermissionActionUpdate  = "update"
	PermissionActionDelete  = "delete"
	PermissionActionManage  = "manage"
	PermissionActionPublish = "publish"
	PermissionActionApprove = "approve"
	PermissionActionExport  = "export"
)

var permissionSegmentPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

// Permission is a globally stable resource.action capability. Permission codes
// are system definitions and are shared by all organizations.
type Permission struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"uniqueIndex;size:128;not null" json:"code"`
	Resource    string    `gorm:"size:64;not null;index" json:"resource"`
	Action      string    `gorm:"size:64;not null;index" json:"action"`
	Description string    `gorm:"size:255;not null;default:''" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Role belongs to the deployment's single enterprise organization.
type Role struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	OrganizationID uint           `gorm:"uniqueIndex:idx_role_org_slug;index;not null" json:"organization_id"`
	Organization   Organization   `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Slug           string         `gorm:"uniqueIndex:idx_role_org_slug;size:80;not null" json:"slug"`
	Name           string         `gorm:"size:120;not null" json:"name"`
	Description    string         `gorm:"size:255;not null;default:''" json:"description"`
	Builtin        bool           `gorm:"not null;default:false;index" json:"builtin"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type RolePermission struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	RoleID       uint       `gorm:"uniqueIndex:idx_role_permission;index;not null" json:"role_id"`
	Role         Role       `gorm:"foreignKey:RoleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	PermissionID uint       `gorm:"uniqueIndex:idx_role_permission;index;not null" json:"permission_id"`
	Permission   Permission `gorm:"foreignKey:PermissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
}

// RoleBinding grants a role to a user at either organization or workspace
// scope. ScopeID is always populated: organization bindings use the
// OrganizationID, while workspace bindings use the Workspace ID.
type RoleBinding struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	OrganizationID  uint           `gorm:"uniqueIndex:idx_role_binding_identity;index;not null" json:"organization_id"`
	Organization    Organization   `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	UserID          uint           `gorm:"uniqueIndex:idx_role_binding_identity;index;not null" json:"user_id"`
	User            User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"user,omitempty"`
	RoleID          uint           `gorm:"uniqueIndex:idx_role_binding_identity;index;not null" json:"role_id"`
	Role            Role           `gorm:"foreignKey:RoleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	ScopeType       string         `gorm:"uniqueIndex:idx_role_binding_identity;size:20;not null;index" json:"scope_type"`
	ScopeID         uint           `gorm:"uniqueIndex:idx_role_binding_identity;not null;index" json:"scope_id"`
	CreatedByUserID uint           `gorm:"index;not null" json:"created_by_user_id"`
	CreatedByUser   User           `gorm:"foreignKey:CreatedByUserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	ExpiresAt       *time.Time     `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func PermissionCode(resource, action string) (string, bool) {
	resource = strings.ToLower(strings.TrimSpace(resource))
	action = strings.ToLower(strings.TrimSpace(action))
	if !permissionSegmentPattern.MatchString(resource) || !permissionSegmentPattern.MatchString(action) {
		return "", false
	}
	return resource + "." + action, true
}

func NormalizeRoleBindingScope(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), RoleBindingScopeWorkspace) {
		return RoleBindingScopeWorkspace
	}
	return RoleBindingScopeOrganization
}

func RoleBindingScopeValid(scopeType string, scopeID, organizationID uint) bool {
	switch strings.ToLower(strings.TrimSpace(scopeType)) {
	case RoleBindingScopeOrganization:
		return scopeID != 0 && scopeID == organizationID
	case RoleBindingScopeWorkspace:
		return scopeID != 0 && organizationID != 0
	default:
		return false
	}
}
