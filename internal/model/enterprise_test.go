package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type enterpriseScopedResource struct {
	ID             uint `gorm:"primaryKey"`
	OrganizationID uint `gorm:"index"`
	WorkspaceID    uint `gorm:"index"`
	Value          string
}

func TestOrganizationMigrationAndUniqueSlug(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:organization-model-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&Organization{}); err != nil {
		t.Fatalf("migrate organization: %v", err)
	}
	if !db.Migrator().HasTable(&Organization{}) {
		t.Fatal("expected organizations table to exist")
	}

	first := Organization{Slug: "acme", Name: "Acme", CreatedByUserID: 1}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if first.Status != OrganizationStatusActive {
		t.Fatalf("default status = %q, want %q", first.Status, OrganizationStatusActive)
	}
	duplicate := Organization{Slug: "acme", Name: "Another Acme", CreatedByUserID: 2}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate organization slug to fail")
	}
}

func TestNormalizeOrganizationStatus(t *testing.T) {
	if got := NormalizeOrganizationStatus(" SUSPENDED "); got != OrganizationStatusSuspended {
		t.Fatalf("normalized status = %q, want suspended", got)
	}
	if got := NormalizeOrganizationStatus("unknown"); got != OrganizationStatusActive {
		t.Fatalf("normalized status = %q, want active", got)
	}
}

func TestEnterpriseOrganizationModelsAndTenantScopedUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:enterprise-model-test?mode=memory&cache=shared"), &gorm.Config{})
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

	userOne := User{Username: "enterprise-user-one", Email: "one@example.com", APIKey: "test-enterprise-key-one"}
	userTwo := User{Username: "enterprise-user-two", Email: "two@example.com", APIKey: "test-enterprise-key-two"}
	if err := db.Create(&userOne).Error; err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if err := db.Create(&userTwo).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}
	orgOne := Organization{Slug: "org-one", Name: "Organization One", CreatedByUserID: userOne.ID}
	orgTwo := Organization{Slug: "org-two", Name: "Organization Two", CreatedByUserID: userTwo.ID}
	if err := db.Create(&orgOne).Error; err != nil {
		t.Fatalf("create first organization: %v", err)
	}
	if err := db.Create(&orgTwo).Error; err != nil {
		t.Fatalf("create second organization: %v", err)
	}

	membership := OrganizationMember{OrganizationID: orgOne.ID, UserID: userOne.ID, Role: OrganizationMemberRoleOwner}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create organization membership: %v", err)
	}
	if membership.Status != OrganizationMemberStatusActive {
		t.Fatalf("default membership status = %q, want active", membership.Status)
	}
	if err := db.Create(&OrganizationMember{OrganizationID: orgOne.ID, UserID: userOne.ID}).Error; err == nil {
		t.Fatal("expected duplicate organization membership to fail")
	}

	engineeringOne := Department{OrganizationID: orgOne.ID, Slug: "engineering", Name: "Engineering"}
	if err := db.Create(&engineeringOne).Error; err != nil {
		t.Fatalf("create first department: %v", err)
	}
	if err := db.Create(&Department{OrganizationID: orgOne.ID, Slug: "engineering", Name: "Duplicate"}).Error; err == nil {
		t.Fatal("expected duplicate department slug in one organization to fail")
	}
	if err := db.Create(&Department{OrganizationID: orgTwo.ID, Slug: "engineering", Name: "Engineering"}).Error; err != nil {
		t.Fatalf("reuse department slug in another organization: %v", err)
	}
	child := Department{OrganizationID: orgOne.ID, ParentID: &engineeringOne.ID, Slug: "platform", Name: "Platform"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child department: %v", err)
	}

	workspace := Workspace{
		OrganizationID:  orgOne.ID,
		DepartmentID:    &engineeringOne.ID,
		Slug:            "ai-platform",
		Name:            "AI Platform",
		Type:            WorkspaceTypeDepartment,
		CreatedByUserID: userOne.ID,
	}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if workspace.Status != WorkspaceStatusActive {
		t.Fatalf("default workspace status = %q, want active", workspace.Status)
	}
	if err := db.Create(&Workspace{OrganizationID: orgOne.ID, Slug: "ai-platform", Name: "Duplicate", CreatedByUserID: userOne.ID}).Error; err == nil {
		t.Fatal("expected duplicate workspace slug in one organization to fail")
	}
	if err := db.Create(&Workspace{OrganizationID: orgTwo.ID, Slug: "ai-platform", Name: "AI Platform", CreatedByUserID: userTwo.ID}).Error; err != nil {
		t.Fatalf("reuse workspace slug in another organization: %v", err)
	}

	workspaceMember := WorkspaceMember{WorkspaceID: workspace.ID, UserID: userOne.ID, Role: WorkspaceMemberRoleOwner}
	if err := db.Create(&workspaceMember).Error; err != nil {
		t.Fatalf("create workspace membership: %v", err)
	}
	if err := db.Create(&WorkspaceMember{WorkspaceID: workspace.ID, UserID: userOne.ID}).Error; err == nil {
		t.Fatal("expected duplicate workspace membership to fail")
	}
}

func TestNormalizeEnterpriseBootstrapValues(t *testing.T) {
	if got := NormalizeOrganizationMemberRole(" ADMIN "); got != OrganizationMemberRoleAdmin {
		t.Fatalf("organization role = %q, want admin", got)
	}
	if got := NormalizeOrganizationMemberRole("unknown"); got != OrganizationMemberRoleMember {
		t.Fatalf("organization role = %q, want member", got)
	}
	if got := NormalizeOrganizationMemberStatus("invited"); got != OrganizationMemberStatusInvited {
		t.Fatalf("organization member status = %q, want invited", got)
	}
	if got := NormalizeWorkspaceType("personal"); got != WorkspaceTypePersonal {
		t.Fatalf("workspace type = %q, want personal", got)
	}
	if got := NormalizeWorkspaceStatus("ARCHIVED"); got != WorkspaceStatusArchived {
		t.Fatalf("workspace status = %q, want archived", got)
	}
	if got := NormalizeWorkspaceMemberRole("viewer"); got != WorkspaceMemberRoleViewer {
		t.Fatalf("workspace member role = %q, want viewer", got)
	}
}

func TestEnterpriseQueryScopesEnforceTenantBoundaries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:enterprise-scope-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&enterpriseScopedResource{}); err != nil {
		t.Fatalf("migrate scoped resource: %v", err)
	}
	resources := []enterpriseScopedResource{
		{OrganizationID: 1, WorkspaceID: 10, Value: "org-1-workspace-10"},
		{OrganizationID: 1, WorkspaceID: 11, Value: "org-1-workspace-11"},
		{OrganizationID: 2, WorkspaceID: 10, Value: "org-2-workspace-10"},
	}
	if err := db.Create(&resources).Error; err != nil {
		t.Fatalf("create scoped resources: %v", err)
	}

	var organizationResources []enterpriseScopedResource
	if err := db.Scopes(ScopeOrganization(1)).Find(&organizationResources).Error; err != nil {
		t.Fatalf("query organization scope: %v", err)
	}
	if len(organizationResources) != 2 {
		t.Fatalf("organization scope returned %d rows, want 2", len(organizationResources))
	}

	var workspaceResources []enterpriseScopedResource
	if err := db.Scopes(ScopeWorkspace(1, 10)).Find(&workspaceResources).Error; err != nil {
		t.Fatalf("query workspace scope: %v", err)
	}
	if len(workspaceResources) != 1 || workspaceResources[0].Value != "org-1-workspace-10" {
		t.Fatalf("unexpected workspace scoped rows: %+v", workspaceResources)
	}

	var zeroScopeCount int64
	if err := db.Model(&enterpriseScopedResource{}).Scopes(ScopeOrganization(0)).Count(&zeroScopeCount).Error; err != nil {
		t.Fatalf("query zero organization scope: %v", err)
	}
	if zeroScopeCount != 0 {
		t.Fatalf("zero organization scope returned %d rows, want 0", zeroScopeCount)
	}
}
