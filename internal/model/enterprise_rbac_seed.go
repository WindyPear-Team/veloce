package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	BuiltinRoleOrganizationAdmin = "organization-admin"
	BuiltinRoleSecurityAdmin     = "security-admin"
	BuiltinRoleAIAdmin           = "ai-admin"
	BuiltinRoleAuditor           = "auditor"
	BuiltinRoleMember            = "member"
)

type enterprisePermissionDefinition struct {
	Resource    string
	Action      string
	Description string
}

type enterpriseRoleDefinition struct {
	Slug            string
	Name            string
	Description     string
	PermissionCodes []string
}

var enterprisePermissionDefinitions = []enterprisePermissionDefinition{
	{Resource: "organization", Action: PermissionActionRead, Description: "View organization settings"},
	{Resource: "organization", Action: PermissionActionManage, Description: "Manage organization settings"},
	{Resource: "member", Action: PermissionActionRead, Description: "View organization members"},
	{Resource: "member", Action: PermissionActionManage, Description: "Invite, update, and remove members"},
	{Resource: "role", Action: PermissionActionRead, Description: "View roles and bindings"},
	{Resource: "role", Action: PermissionActionManage, Description: "Manage roles and bindings"},
	{Resource: "workspace", Action: PermissionActionRead, Description: "View workspaces"},
	{Resource: "workspace", Action: PermissionActionManage, Description: "Manage workspaces"},
	{Resource: "agent", Action: PermissionActionRead, Description: "View agents"},
	{Resource: "agent", Action: PermissionActionUse, Description: "Run agents"},
	{Resource: "agent", Action: PermissionActionManage, Description: "Manage agent drafts"},
	{Resource: "agent", Action: PermissionActionPublish, Description: "Publish agents"},
	{Resource: "knowledge", Action: PermissionActionRead, Description: "View knowledge metadata"},
	{Resource: "knowledge", Action: PermissionActionUse, Description: "Retrieve enterprise knowledge"},
	{Resource: "knowledge", Action: PermissionActionManage, Description: "Manage knowledge bases and sources"},
	{Resource: "tool", Action: PermissionActionUse, Description: "Run allowed tools"},
	{Resource: "tool", Action: PermissionActionManage, Description: "Manage tools and connectors"},
	{Resource: "tool", Action: PermissionActionApprove, Description: "Approve tool executions"},
	{Resource: "model", Action: PermissionActionUse, Description: "Use allowed models"},
	{Resource: "model", Action: PermissionActionManage, Description: "Manage model policies"},
	{Resource: "audit", Action: PermissionActionRead, Description: "View enterprise audit events"},
	{Resource: "audit", Action: PermissionActionExport, Description: "Export enterprise audit events"},
	{Resource: "cost", Action: PermissionActionRead, Description: "View enterprise cost reports"},
	{Resource: "cost", Action: PermissionActionManage, Description: "Manage budgets and cost policies"},
	{Resource: "security", Action: PermissionActionManage, Description: "Manage enterprise security policies"},
}

func EnsureOrganizationRBAC(db *gorm.DB, organizationID, ownerUserID uint) error {
	if db == nil || organizationID == 0 {
		return errors.New("organization database and id are required")
	}
	permissions, err := ensureEnterprisePermissions(db)
	if err != nil {
		return err
	}
	definitions := enterpriseBuiltinRoleDefinitions(permissions)
	roles := make(map[string]Role, len(definitions))
	for _, definition := range definitions {
		role, err := ensureEnterpriseBuiltinRole(db, organizationID, definition, permissions)
		if err != nil {
			return err
		}
		roles[role.Slug] = role
	}
	if ownerUserID == 0 {
		return nil
	}
	adminRole, ok := roles[BuiltinRoleOrganizationAdmin]
	if !ok {
		return errors.New("organization administrator role was not initialized")
	}
	binding := RoleBinding{}
	err = db.Where(
		"organization_id = ? AND user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
		organizationID,
		ownerUserID,
		adminRole.ID,
		RoleBindingScopeOrganization,
		organizationID,
	).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&RoleBinding{
			OrganizationID:  organizationID,
			UserID:          ownerUserID,
			RoleID:          adminRole.ID,
			ScopeType:       RoleBindingScopeOrganization,
			ScopeID:         organizationID,
			CreatedByUserID: ownerUserID,
		}).Error
	}
	return err
}

func EnsureOrganizationRoleBinding(db *gorm.DB, organizationID, userID, createdByUserID uint, roleSlug string) error {
	if db == nil || organizationID == 0 || userID == 0 || createdByUserID == 0 {
		return errors.New("organization, user, and creator are required")
	}
	var role Role
	if err := db.Where("organization_id = ? AND slug = ?", organizationID, roleSlug).First(&role).Error; err != nil {
		return err
	}
	var binding RoleBinding
	err := db.Where(
		"organization_id = ? AND user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
		organizationID,
		userID,
		role.ID,
		RoleBindingScopeOrganization,
		organizationID,
	).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&RoleBinding{
			OrganizationID:  organizationID,
			UserID:          userID,
			RoleID:          role.ID,
			ScopeType:       RoleBindingScopeOrganization,
			ScopeID:         organizationID,
			CreatedByUserID: createdByUserID,
		}).Error
	}
	return err
}

func ensureEnterprisePermissions(db *gorm.DB) (map[string]Permission, error) {
	result := make(map[string]Permission, len(enterprisePermissionDefinitions))
	for _, definition := range enterprisePermissionDefinitions {
		code, ok := PermissionCode(definition.Resource, definition.Action)
		if !ok {
			return nil, fmt.Errorf("invalid built-in permission %s.%s", definition.Resource, definition.Action)
		}
		permission := Permission{}
		err := db.Where("code = ?", code).First(&permission).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			permission = Permission{Code: code, Resource: definition.Resource, Action: definition.Action, Description: definition.Description}
			if err := db.Create(&permission).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		result[code] = permission
	}
	return result, nil
}

func ensureEnterpriseBuiltinRole(db *gorm.DB, organizationID uint, definition enterpriseRoleDefinition, permissions map[string]Permission) (Role, error) {
	role := Role{}
	err := db.Where("organization_id = ? AND slug = ?", organizationID, definition.Slug).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		role = Role{
			OrganizationID: organizationID,
			Slug:           definition.Slug,
			Name:           definition.Name,
			Description:    definition.Description,
			Builtin:        true,
		}
		if err := db.Create(&role).Error; err != nil {
			return Role{}, err
		}
	} else if err != nil {
		return Role{}, err
	}
	for _, code := range definition.PermissionCodes {
		permission, ok := permissions[code]
		if !ok {
			return Role{}, fmt.Errorf("built-in role %s references unknown permission %s", definition.Slug, code)
		}
		var rolePermission RolePermission
		err := db.Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).First(&rolePermission).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
				return Role{}, err
			}
		} else if err != nil {
			return Role{}, err
		}
	}
	return role, nil
}

func enterpriseBuiltinRoleDefinitions(permissions map[string]Permission) []enterpriseRoleDefinition {
	all := make([]string, 0, len(permissions))
	for _, definition := range enterprisePermissionDefinitions {
		code, _ := PermissionCode(definition.Resource, definition.Action)
		all = append(all, code)
	}
	return []enterpriseRoleDefinition{
		{Slug: BuiltinRoleOrganizationAdmin, Name: "Organization Administrator", Description: "Full organization administration", PermissionCodes: all},
		{Slug: BuiltinRoleSecurityAdmin, Name: "Security Administrator", Description: "Security, role, approval, and audit administration", PermissionCodes: permissionCodes(
			"organization.read", "member.read", "role.read", "role.manage", "security.manage", "audit.read", "audit.export", "tool.approve",
		)},
		{Slug: BuiltinRoleAIAdmin, Name: "AI Administrator", Description: "Agent, knowledge, tool, model, and workspace administration", PermissionCodes: permissionCodes(
			"organization.read", "workspace.read", "workspace.manage", "agent.read", "agent.use", "agent.manage", "agent.publish",
			"knowledge.read", "knowledge.use", "knowledge.manage", "tool.use", "tool.manage", "model.use", "model.manage", "cost.read",
		)},
		{Slug: BuiltinRoleAuditor, Name: "Auditor", Description: "Read-only enterprise governance and audit access", PermissionCodes: permissionCodes(
			"organization.read", "member.read", "workspace.read", "agent.read", "knowledge.read", "audit.read", "audit.export", "cost.read",
		)},
		{Slug: BuiltinRoleMember, Name: "Member", Description: "Standard enterprise AI usage", PermissionCodes: permissionCodes(
			"organization.read", "workspace.read", "agent.read", "agent.use", "knowledge.read", "knowledge.use", "tool.use", "model.use",
		)},
	}
}

func permissionCodes(codes ...string) []string {
	return codes
}
