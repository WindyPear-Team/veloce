package middleware

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestResolveTenantContextUsesPersonalDefaults(t *testing.T) {
	db, user := tenantTestDatabase(t, "defaults")
	if err := model.EnsurePersonalTenantForUser(db, &user); err != nil {
		t.Fatalf("bootstrap personal tenant: %v", err)
	}

	tenant, status, err := ResolveTenantContext(db, user.ID, TenantSelection{})
	if err != nil || status != http.StatusOK {
		t.Fatalf("resolve default tenant: status=%d err=%v", status, err)
	}
	if tenant.Organization.Slug != fmt.Sprintf("personal-u-%d", user.ID) {
		t.Fatalf("organization slug = %q", tenant.Organization.Slug)
	}
	if tenant.Workspace == nil || tenant.Workspace.Type != model.WorkspaceTypePersonal {
		t.Fatalf("expected personal workspace, got %+v", tenant.Workspace)
	}
}

func TestResolveTenantContextRejectsUnauthorizedOrganizationAndWorkspace(t *testing.T) {
	db, user := tenantTestDatabase(t, "unauthorized")
	if err := model.EnsurePersonalTenantForUser(db, &user); err != nil {
		t.Fatalf("bootstrap personal tenant: %v", err)
	}
	otherUser := model.User{Username: "tenant-other-user", Email: "tenant-other@example.com", APIKey: "tenant-other-key"}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if err := model.EnsurePersonalTenantForUser(db, &otherUser); err != nil {
		t.Fatalf("bootstrap other personal tenant: %v", err)
	}
	var otherOrganization model.Organization
	if err := db.Where("created_by_user_id = ?", otherUser.ID).First(&otherOrganization).Error; err != nil {
		t.Fatalf("find other organization: %v", err)
	}
	if _, status, err := ResolveTenantContext(db, user.ID, TenantSelection{OrganizationID: fmt.Sprint(otherOrganization.ID)}); err == nil || status != http.StatusForbidden {
		t.Fatalf("unauthorized organization result: status=%d err=%v", status, err)
	}

	var otherWorkspace model.Workspace
	if err := db.Where("organization_id = ?", otherOrganization.ID).First(&otherWorkspace).Error; err != nil {
		t.Fatalf("find other workspace: %v", err)
	}
	if _, status, err := ResolveTenantContext(db, user.ID, TenantSelection{WorkspaceID: fmt.Sprint(otherWorkspace.ID)}); err == nil || status != http.StatusForbidden {
		t.Fatalf("unauthorized workspace result: status=%d err=%v", status, err)
	}
}

func TestResolveTenantContextRejectsInvalidIDs(t *testing.T) {
	db, user := tenantTestDatabase(t, "invalid-id")
	if err := model.EnsurePersonalTenantForUser(db, &user); err != nil {
		t.Fatalf("bootstrap personal tenant: %v", err)
	}
	if _, status, err := ResolveTenantContext(db, user.ID, TenantSelection{OrganizationID: "not-an-id"}); err == nil || status != http.StatusBadRequest {
		t.Fatalf("invalid organization id result: status=%d err=%v", status, err)
	}
	if _, status, err := ResolveTenantContext(db, user.ID, TenantSelection{WorkspaceID: "0"}); err == nil || status != http.StatusBadRequest {
		t.Fatalf("invalid workspace id result: status=%d err=%v", status, err)
	}
}

func tenantTestDatabase(t *testing.T, name string) (*gorm.DB, model.User) {
	t.Helper()
	dsn := fmt.Sprintf("file:tenant-middleware-%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Organization{},
		&model.Department{},
		&model.Workspace{},
		&model.OrganizationMember{},
		&model.WorkspaceMember{},
	); err != nil {
		t.Fatalf("migrate enterprise models: %v", err)
	}
	user := model.User{Username: "tenant-user-" + name, Email: name + "@example.com", APIKey: "tenant-key-" + name}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return db, user
}
