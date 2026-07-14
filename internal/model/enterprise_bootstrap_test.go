package model

import (
	"fmt"
	"testing"

	"github.com/WindyPear-Team/veloce/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUserCreateBootstrapsPersonalTenantWhenEnabled(t *testing.T) {
	db := openEnterpriseBootstrapTestDB(t, "create")
	previous := config.EnterpriseFeaturesEnabled
	config.EnterpriseFeaturesEnabled = true
	defer func() { config.EnterpriseFeaturesEnabled = previous }()

	user := User{Username: "new-enterprise-user", Email: "new@example.com", APIKey: "bootstrap-create-key"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	assertPersonalTenant(t, db, user)

	if err := EnsurePersonalTenantForUser(db, &user); err != nil {
		t.Fatalf("repeat personal tenant bootstrap: %v", err)
	}
	assertPersonalTenantCounts(t, db, user.ID, 1, 1, 1, 1)
}

func TestExistingUserPersonalTenantBackfillIsIdempotent(t *testing.T) {
	db := openEnterpriseBootstrapTestDB(t, "backfill")
	previous := config.EnterpriseFeaturesEnabled
	config.EnterpriseFeaturesEnabled = false
	defer func() { config.EnterpriseFeaturesEnabled = previous }()

	users := []User{
		{Username: "existing-user-one", Email: "existing-one@example.com", APIKey: "bootstrap-backfill-key-one"},
		{Username: "existing-user-two", Email: "existing-two@example.com", APIKey: "bootstrap-backfill-key-two"},
	}
	for index := range users {
		if err := db.Create(&users[index]).Error; err != nil {
			t.Fatalf("create existing user %d: %v", index, err)
		}
	}

	config.EnterpriseFeaturesEnabled = true
	if err := EnsurePersonalTenantsForExistingUsers(db); err != nil {
		t.Fatalf("backfill personal tenants: %v", err)
	}
	if err := EnsurePersonalTenantsForExistingUsers(db); err != nil {
		t.Fatalf("repeat personal tenant backfill: %v", err)
	}
	for _, user := range users {
		assertPersonalTenant(t, db, user)
		assertPersonalTenantCounts(t, db, user.ID, 1, 1, 1, 1)
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
		&User{},
		&Organization{},
		&Department{},
		&Workspace{},
		&OrganizationMember{},
		&WorkspaceMember{},
	); err != nil {
		t.Fatalf("migrate enterprise models: %v", err)
	}
	return db
}

func assertPersonalTenant(t *testing.T, db *gorm.DB, user User) {
	t.Helper()
	var organization Organization
	if err := db.Where("slug = ?", personalOrganizationSlug(user.ID)).First(&organization).Error; err != nil {
		t.Fatalf("find personal organization: %v", err)
	}
	if organization.CreatedByUserID != user.ID || organization.Status != OrganizationStatusActive {
		t.Fatalf("unexpected personal organization: %+v", organization)
	}

	var organizationMember OrganizationMember
	if err := db.Where("organization_id = ? AND user_id = ?", organization.ID, user.ID).First(&organizationMember).Error; err != nil {
		t.Fatalf("find organization membership: %v", err)
	}
	if organizationMember.Role != OrganizationMemberRoleOwner || organizationMember.Status != OrganizationMemberStatusActive {
		t.Fatalf("unexpected organization membership: %+v", organizationMember)
	}

	var workspace Workspace
	if err := db.Where("organization_id = ? AND slug = ?", organization.ID, "personal").First(&workspace).Error; err != nil {
		t.Fatalf("find personal workspace: %v", err)
	}
	if workspace.Type != WorkspaceTypePersonal || workspace.CreatedByUserID != user.ID {
		t.Fatalf("unexpected personal workspace: %+v", workspace)
	}

	var workspaceMember WorkspaceMember
	if err := db.Where("workspace_id = ? AND user_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error; err != nil {
		t.Fatalf("find workspace membership: %v", err)
	}
	if workspaceMember.Role != WorkspaceMemberRoleOwner {
		t.Fatalf("unexpected workspace membership: %+v", workspaceMember)
	}
}

func assertPersonalTenantCounts(t *testing.T, db *gorm.DB, userID uint, organizations, organizationMembers, workspaces, workspaceMembers int64) {
	t.Helper()
	var gotOrganizations int64
	if err := db.Model(&Organization{}).Where("created_by_user_id = ?", userID).Count(&gotOrganizations).Error; err != nil {
		t.Fatalf("count organizations: %v", err)
	}
	var gotOrganizationMembers int64
	if err := db.Model(&OrganizationMember{}).Where("user_id = ?", userID).Count(&gotOrganizationMembers).Error; err != nil {
		t.Fatalf("count organization members: %v", err)
	}
	var gotWorkspaces int64
	if err := db.Model(&Workspace{}).Where("created_by_user_id = ?", userID).Count(&gotWorkspaces).Error; err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	var gotWorkspaceMembers int64
	if err := db.Model(&WorkspaceMember{}).Where("user_id = ?", userID).Count(&gotWorkspaceMembers).Error; err != nil {
		t.Fatalf("count workspace members: %v", err)
	}
	if gotOrganizations != organizations || gotOrganizationMembers != organizationMembers || gotWorkspaces != workspaces || gotWorkspaceMembers != workspaceMembers {
		t.Fatalf(
			"personal tenant counts = organizations:%d organization_members:%d workspaces:%d workspace_members:%d",
			gotOrganizations,
			gotOrganizationMembers,
			gotWorkspaces,
			gotWorkspaceMembers,
		)
	}
}
