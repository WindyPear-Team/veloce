package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestPersonalCompanyModelsMigrateAndKeepWorkScoped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:personal-company-model-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&PersonalCompany{}, &CompanyCharterRevision{}, &PersonalCompanyEmployee{}, &CompanyRoleTemplate{}, &CompanyEmployeeVersion{}, &CompanyCapabilityEvidence{}, &CompanyRecruitmentPlan{}, &CompanyObjective{},
		&CompanyWorkItem{}, &CompanyWorkAttempt{}, &CompanyArtifact{}, &CompanyHandoffPackage{},
		&CompanyApprovalRequest{}, &CompanyBudgetLedger{}, &CompanyAuditEvent{},
	); err != nil {
		t.Fatalf("migrate personal company models: %v", err)
	}
	company := PersonalCompany{OwnerUserID: 42, AgentGroupID: "product", Name: "Studio", State: PersonalCompanyStateOperating, DailyBudget: decimal.NewFromInt(10)}
	if err := db.Create(&company).Error; err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := db.Create(&PersonalCompany{OwnerUserID: 42, AgentGroupID: "research", Name: "Research Studio"}).Error; err != nil {
		t.Fatalf("create a second studio operation: %v", err)
	}
	if err := db.Create(&PersonalCompany{OwnerUserID: 42, AgentGroupID: "product", Name: "Duplicate"}).Error; err == nil {
		t.Fatal("accepted a duplicate operation for the same studio")
	}
	work := CompanyWorkItem{PersonalCompanyID: company.ID, OwnerUserID: 42, Title: "Research", DefinitionOfDone: "Evidence recorded", IdempotencyKey: "research-1"}
	if err := db.Create(&work).Error; err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if err := db.Create(&work).Error; err == nil {
		t.Fatal("accepted a duplicate work idempotency key")
	}
	if err := db.Create(&CompanyWorkAttempt{WorkItemID: work.ID, AttemptNumber: 1}).Error; err != nil {
		t.Fatalf("create first work attempt: %v", err)
	}
	if err := db.Create(&CompanyWorkAttempt{WorkItemID: work.ID, AttemptNumber: 1}).Error; err == nil {
		t.Fatal("accepted a duplicate work attempt number")
	}
	template := CompanyRoleTemplate{PersonalCompanyID: company.ID, TemplateKey: "research", Name: "Research", DefinitionOfDone: "Cited findings", CreatedByUserID: 42}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("create role template: %v", err)
	}
	employee := PersonalCompanyEmployee{PersonalCompanyID: company.ID, EmployeeKey: "research-1", Name: "Research candidate", Role: "research"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := db.Create(&CompanyEmployeeVersion{PersonalCompanyID: company.ID, EmployeeID: employee.ID, Version: 1, RoleTemplateID: &template.ID, CreatedByUserID: 42}).Error; err != nil {
		t.Fatalf("create employee version: %v", err)
	}
	if err := db.Create(&CompanyEmployeeVersion{PersonalCompanyID: company.ID, EmployeeID: employee.ID, Version: 1, CreatedByUserID: 42}).Error; err == nil {
		t.Fatal("accepted a duplicate employee version")
	}
}

func TestPersonalCompanyNormalizersFailClosed(t *testing.T) {
	if got := NormalizePersonalCompanyState("safe_mode"); got != PersonalCompanyStateSafeMode {
		t.Fatalf("state = %q, want safe_mode", got)
	}
	if got := NormalizePersonalCompanyAutonomy("r4"); got != PersonalCompanyAutonomyR0 {
		t.Fatalf("autonomy = %q, want r0", got)
	}
	if got := NormalizeCompanyWorkStatus("untrusted"); got != CompanyWorkStatusPlanned {
		t.Fatalf("work status = %q, want planned", got)
	}
}

func TestPersonalCompanyForeignKeysRejectOrphans(t *testing.T) {
	db := openConstraintTestDB(t, "personal-company-foreign-key-constraints")
	if err := db.AutoMigrate(&PersonalCompany{}, &CompanyObjective{}, &CompanyWorkItem{}, &CompanyWorkAttempt{}, &CompanyArtifact{}, &CompanyApprovalRequest{}, &CompanyBudgetLedger{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CompanyWorkItem{PersonalCompanyID: 999, OwnerUserID: 1, Title: "Orphan", DefinitionOfDone: "Never", IdempotencyKey: "orphan-work"}).Error; err == nil {
		t.Fatal("accepted work item without a company")
	}
	company := PersonalCompany{OwnerUserID: 1, AgentGroupID: "constraint-studio", Name: "Studio"}
	if err := db.Create(&company).Error; err != nil {
		t.Fatal(err)
	}
	work := CompanyWorkItem{PersonalCompanyID: company.ID, OwnerUserID: 1, Title: "Work", DefinitionOfDone: "Done", IdempotencyKey: "constraint-work"}
	if err := db.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CompanyWorkAttempt{WorkItemID: 999, AttemptNumber: 1}).Error; err == nil {
		t.Fatal("accepted attempt without a work item")
	}
	if err := db.Create(&CompanyArtifact{WorkItemID: work.ID, WorkAttemptID: uintPtr(999), Kind: "result", URI: "memory://result"}).Error; err == nil {
		t.Fatal("accepted artifact with a missing work attempt")
	}
}

func uintPtr(value uint) *uint {
	return &value
}
