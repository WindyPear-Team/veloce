package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUserCreateJoinsSingleEnterpriseWhenEnabled(t *testing.T) {
	db := openEnterpriseBootstrapTestDB(t, "create")
	if err := SetSystemSettingWithDB(db, "system_mode", EnterpriseSystemMode); err != nil {
		t.Fatalf("enable enterprise mode: %v", err)
	}

	owner := User{Username: "enterprise-owner", Email: "owner@example.com", APIKey: "enterprise-owner-key"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	employee := User{Username: "enterprise-employee", Email: "employee@example.com", APIKey: "enterprise-employee-key"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}

	organization := assertSingleEnterprise(t, db, owner.ID)
	assertEnterpriseEmployee(t, db, organization.ID, owner, OrganizationMemberRoleOwner, BuiltinRoleOrganizationAdmin)
	assertEnterpriseEmployee(t, db, organization.ID, employee, OrganizationMemberRoleMember, BuiltinRoleMember)
	assertEnterpriseCounts(t, db, 1, 2, 2, 2)
}

func TestExistingUserEnterpriseBackfillIsIdempotent(t *testing.T) {
	db := openEnterpriseBootstrapTestDB(t, "backfill")
	users := []User{
		{Username: "existing-owner", Email: "existing-owner@example.com", APIKey: "existing-owner-key"},
		{Username: "existing-member", Email: "existing-member@example.com", APIKey: "existing-member-key"},
	}
	for index := range users {
		if err := db.Create(&users[index]).Error; err != nil {
			t.Fatalf("create existing user %d: %v", index, err)
		}
	}
	legacyOrganization := Organization{Slug: fmt.Sprintf("personal-u-%d", users[1].ID), Name: "Legacy Personal Organization", CreatedByUserID: users[1].ID}
	if err := db.Create(&legacyOrganization).Error; err != nil {
		t.Fatalf("create legacy personal organization: %v", err)
	}

	if err := SetSystemSettingWithDB(db, "system_mode", EnterpriseSystemMode); err != nil {
		t.Fatalf("enable enterprise mode: %v", err)
	}
	if err := EnsureEnterpriseTenantForExistingUsers(db); err != nil {
		t.Fatalf("backfill enterprise tenant: %v", err)
	}
	if err := EnsureEnterpriseTenantForExistingUsers(db); err != nil {
		t.Fatalf("repeat enterprise tenant backfill: %v", err)
	}
	organization := assertSingleEnterprise(t, db, users[0].ID)
	assertEnterpriseEmployee(t, db, organization.ID, users[0], OrganizationMemberRoleOwner, BuiltinRoleOrganizationAdmin)
	assertEnterpriseEmployee(t, db, organization.ID, users[1], OrganizationMemberRoleMember, BuiltinRoleMember)
	assertEnterpriseCounts(t, db, 1, 2, 2, 2)
	var retiredLegacy Organization
	if err := db.Where("id = ?", legacyOrganization.ID).First(&retiredLegacy).Error; err != nil {
		t.Fatalf("find retired legacy organization: %v", err)
	}
	if retiredLegacy.Status != OrganizationStatusSuspended {
		t.Fatalf("legacy organization status = %q, want suspended", retiredLegacy.Status)
	}
}

func openEnterpriseBootstrapTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:enterprise-bootstrap-%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&User{}, &Organization{}, &Department{}, &Workspace{}, &OrganizationMember{}, &WorkspaceMember{},
		&Permission{}, &Role{}, &RolePermission{}, &RoleBinding{}, &SystemSetting{},
	); err != nil {
		t.Fatalf("migrate enterprise models: %v", err)
	}
	return db
}

func assertSingleEnterprise(t *testing.T, db *gorm.DB, ownerUserID uint) Organization {
	t.Helper()
	var organizations []Organization
	if err := db.Find(&organizations).Error; err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	active := make([]Organization, 0, 1)
	for _, organization := range organizations {
		if organization.Status == OrganizationStatusActive {
			active = append(active, organization)
		}
	}
	if len(active) != 1 {
		t.Fatalf("active organization count = %d, want 1", len(active))
	}
	organization := active[0]
	if organization.Slug != EnterpriseOrganizationSlug || organization.CreatedByUserID != ownerUserID {
		t.Fatalf("unexpected enterprise organization: %+v", organization)
	}
	return organization
}

func assertEnterpriseEmployee(t *testing.T, db *gorm.DB, organizationID uint, user User, memberRole, roleSlug string) {
	t.Helper()
	var membership OrganizationMember
	if err := db.Where("organization_id = ? AND user_id = ?", organizationID, user.ID).First(&membership).Error; err != nil {
		t.Fatalf("find organization membership: %v", err)
	}
	if membership.Role != memberRole || membership.Status != OrganizationMemberStatusActive {
		t.Fatalf("unexpected organization membership: %+v", membership)
	}
	var workspace Workspace
	if err := db.Where("organization_id = ? AND slug = ?", organizationID, personalWorkspaceSlug(user.ID)).First(&workspace).Error; err != nil {
		t.Fatalf("find personal workspace: %v", err)
	}
	if workspace.Type != WorkspaceTypePersonal || workspace.CreatedByUserID != user.ID {
		t.Fatalf("unexpected personal workspace: %+v", workspace)
	}
	var workspaceMember WorkspaceMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error; err != nil {
		t.Fatalf("find workspace membership: %v", err)
	}
	var role Role
	if err := db.Where("organization_id = ? AND slug = ?", organizationID, roleSlug).First(&role).Error; err != nil {
		t.Fatalf("find employee role: %v", err)
	}
	var binding RoleBinding
	if err := db.Where("organization_id = ? AND user_id = ? AND role_id = ?", organizationID, user.ID, role.ID).First(&binding).Error; err != nil {
		t.Fatalf("find employee role binding: %v", err)
	}
}

func assertEnterpriseCounts(t *testing.T, db *gorm.DB, organizations, organizationMembers, workspaces, workspaceMembers int64) {
	t.Helper()
	counts := []struct {
		model interface{}
		want  int64
		name  string
	}{
		{model: &Organization{}, want: organizations, name: "active organizations"},
		{model: &OrganizationMember{}, want: organizationMembers, name: "organization members"},
		{model: &Workspace{}, want: workspaces, name: "workspaces"},
		{model: &WorkspaceMember{}, want: workspaceMembers, name: "workspace members"},
	}
	for _, item := range counts {
		var got int64
		query := db.Model(item.model)
		if item.name == "active organizations" {
			query = query.Where("status = ?", OrganizationStatusActive)
		}
		if err := query.Count(&got).Error; err != nil {
			t.Fatalf("count %s: %v", item.name, err)
		}
		if got != item.want {
			t.Fatalf("%s count = %d, want %d", item.name, got, item.want)
		}
	}
}
