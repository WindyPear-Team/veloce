package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPermissionCodeNamingConvention(t *testing.T) {
	code, ok := PermissionCode("Knowledge_Base", "READ")
	if !ok || code != "knowledge_base.read" {
		t.Fatalf("permission code = %q, ok=%t", code, ok)
	}
	for _, test := range []struct {
		resource string
		action   string
	}{
		{resource: "", action: "read"},
		{resource: "Agent.Admin", action: "read"},
		{resource: "agent", action: "*"},
	} {
		if code, ok := PermissionCode(test.resource, test.action); ok || code != "" {
			t.Fatalf("invalid permission segments produced code %q", code)
		}
	}
}

func TestEnterpriseRBACMigrationAndUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:enterprise-rbac-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Organization{},
		&Workspace{},
		&Permission{},
		&Role{},
		&RolePermission{},
		&RoleBinding{},
	); err != nil {
		t.Fatalf("migrate rbac models: %v", err)
	}

	user := User{Username: "rbac-user", Email: "rbac@example.com", APIKey: "rbac-test-key"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	otherUser := User{Username: "rbac-user-two", Email: "rbac-two@example.com", APIKey: "rbac-test-key-two"}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}
	orgOne := Organization{Slug: "rbac-org-one", Name: "RBAC One", CreatedByUserID: user.ID}
	orgTwo := Organization{Slug: "rbac-org-two", Name: "RBAC Two", CreatedByUserID: otherUser.ID}
	if err := db.Create(&orgOne).Error; err != nil {
		t.Fatalf("create first organization: %v", err)
	}
	if err := db.Create(&orgTwo).Error; err != nil {
		t.Fatalf("create second organization: %v", err)
	}

	permission := Permission{Code: "agent.read", Resource: "agent", Action: PermissionActionRead}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := db.Create(&Permission{Code: permission.Code, Resource: "agent", Action: PermissionActionRead}).Error; err == nil {
		t.Fatal("expected duplicate permission code to fail")
	}

	roleOne := Role{OrganizationID: orgOne.ID, Slug: "ai-admin", Name: "AI Admin"}
	if err := db.Create(&roleOne).Error; err != nil {
		t.Fatalf("create first role: %v", err)
	}
	if err := db.Create(&Role{OrganizationID: orgOne.ID, Slug: "ai-admin", Name: "Duplicate"}).Error; err == nil {
		t.Fatal("expected duplicate role slug in organization to fail")
	}
	roleTwo := Role{OrganizationID: orgTwo.ID, Slug: "ai-admin", Name: "AI Admin"}
	if err := db.Create(&roleTwo).Error; err != nil {
		t.Fatalf("reuse role slug in another organization: %v", err)
	}

	rolePermission := RolePermission{RoleID: roleOne.ID, PermissionID: permission.ID}
	if err := db.Create(&rolePermission).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}
	if err := db.Create(&RolePermission{RoleID: roleOne.ID, PermissionID: permission.ID}).Error; err == nil {
		t.Fatal("expected duplicate role permission to fail")
	}

	binding := RoleBinding{
		OrganizationID:  orgOne.ID,
		UserID:          user.ID,
		RoleID:          roleOne.ID,
		ScopeType:       RoleBindingScopeOrganization,
		ScopeID:         orgOne.ID,
		CreatedByUserID: user.ID,
	}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	duplicate := binding
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate role binding to fail")
	}
}

func TestRoleBindingScopeValidation(t *testing.T) {
	if !RoleBindingScopeValid(RoleBindingScopeOrganization, 10, 10) {
		t.Fatal("expected matching organization scope to be valid")
	}
	if RoleBindingScopeValid(RoleBindingScopeOrganization, 11, 10) {
		t.Fatal("expected mismatched organization scope to be invalid")
	}
	if !RoleBindingScopeValid(RoleBindingScopeWorkspace, 99, 10) {
		t.Fatal("expected non-zero workspace scope to be valid")
	}
	if RoleBindingScopeValid("invalid", 10, 10) {
		t.Fatal("expected unknown scope type to be invalid")
	}
}

func TestEnsureOrganizationRBACSeedsBuiltinRolesAndOwnerBindingIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:enterprise-rbac-seed-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Organization{},
		&Workspace{},
		&Permission{},
		&Role{},
		&RolePermission{},
		&RoleBinding{},
	); err != nil {
		t.Fatalf("migrate rbac models: %v", err)
	}
	owner := User{Username: "rbac-owner", Email: "rbac-owner@example.com", APIKey: "rbac-owner-key"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	organization := Organization{Slug: "rbac-seed-org", Name: "RBAC Seed", CreatedByUserID: owner.ID}
	if err := db.Create(&organization).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := EnsureOrganizationRBAC(db, organization.ID, owner.ID); err != nil {
		t.Fatalf("seed organization rbac: %v", err)
	}
	if err := EnsureOrganizationRBAC(db, organization.ID, owner.ID); err != nil {
		t.Fatalf("repeat organization rbac seed: %v", err)
	}

	var permissionCount int64
	if err := db.Model(&Permission{}).Count(&permissionCount).Error; err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permissionCount != int64(len(enterprisePermissionDefinitions)) {
		t.Fatalf("permission count = %d, want %d", permissionCount, len(enterprisePermissionDefinitions))
	}
	var roleCount int64
	if err := db.Model(&Role{}).Where("organization_id = ? AND builtin = ?", organization.ID, true).Count(&roleCount).Error; err != nil {
		t.Fatalf("count built-in roles: %v", err)
	}
	if roleCount != 5 {
		t.Fatalf("built-in role count = %d, want 5", roleCount)
	}
	var adminRole Role
	if err := db.Where("organization_id = ? AND slug = ?", organization.ID, BuiltinRoleOrganizationAdmin).First(&adminRole).Error; err != nil {
		t.Fatalf("find organization admin role: %v", err)
	}
	var bindingCount int64
	if err := db.Model(&RoleBinding{}).Where(
		"organization_id = ? AND user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
		organization.ID,
		owner.ID,
		adminRole.ID,
		RoleBindingScopeOrganization,
		organization.ID,
	).Count(&bindingCount).Error; err != nil {
		t.Fatalf("count owner bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("owner binding count = %d, want 1", bindingCount)
	}
}
