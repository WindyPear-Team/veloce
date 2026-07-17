package model

import (
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openConstraintTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared&_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnterpriseConstraintsRejectCrossOrganizationReferences(t *testing.T) {
	db := openConstraintTestDB(t, "enterprise-cross-organization-constraints")
	if err := db.AutoMigrate(&User{}, &Group{}, &Organization{}, &Department{}, &Workspace{}, &Role{}, &RoleBinding{}, &EnterpriseTask{}, &EnterpriseDevice{}, &EnterpriseDeviceAssignment{}, &QuotaAccount{}); err != nil {
		t.Fatal(err)
	}
	group := Group{Name: "constraint-test-group"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	users := []User{{Username: "constraint-user-one", Email: "constraint-one@example.com", APIKey: "constraint-key-one", GroupID: group.ID}, {Username: "constraint-user-two", Email: "constraint-two@example.com", APIKey: "constraint-key-two", GroupID: group.ID}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	organizations := []Organization{{Slug: "constraint-org-one", Name: "Organization One", CreatedByUserID: users[0].ID}, {Slug: "constraint-org-two", Name: "Organization Two", CreatedByUserID: users[1].ID}}
	if err := db.Create(&organizations).Error; err != nil {
		t.Fatal(err)
	}
	department := Department{OrganizationID: organizations[0].ID, Slug: "engineering", Name: "Engineering"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Workspace{OrganizationID: organizations[1].ID, DepartmentID: &department.ID, Slug: "foreign-department", Name: "Invalid"}).Error; err == nil {
		t.Fatal("accepted workspace department from another organization")
	}
	if err := db.Create(&EnterpriseTask{OrganizationID: organizations[1].ID, DepartmentID: &department.ID, CreatedByUserID: users[1].ID, OwnerUserID: users[1].ID, Title: "Invalid task"}).Error; err == nil {
		t.Fatal("accepted task department from another organization")
	}
	role := Role{OrganizationID: organizations[0].ID, Slug: "viewer", Name: "Viewer"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RoleBinding{OrganizationID: organizations[1].ID, UserID: users[1].ID, RoleID: role.ID, ScopeType: RoleBindingScopeOrganization, ScopeID: organizations[1].ID, CreatedByUserID: users[1].ID}).Error; err == nil {
		t.Fatal("accepted role from another organization")
	}
	device := EnterpriseDevice{OrganizationID: organizations[0].ID, ExternalDeviceID: "constraint-device", Name: "Device"}
	if err := db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&EnterpriseDeviceAssignment{OrganizationID: organizations[1].ID, DeviceID: device.ID, ScopeType: EnterpriseDeviceAssignmentUser, UserID: &users[1].ID, AssignedBy: users[1].ID}).Error; err == nil {
		t.Fatal("accepted device from another organization")
	}
	if err := db.Create(&QuotaAccount{OrganizationID: organizations[0].ID, ScopeType: QuotaScopeTask, ScopeKey: "missing-task"}).Error; err == nil {
		t.Fatal("accepted quota scope without its required target")
	}
}

func TestEnterpriseMigrationSupportsSQLiteCompositeForeignKeys(t *testing.T) {
	db := openConstraintTestDB(t, "enterprise-phased-migration")
	models := []interface{}{&User{}, &Group{}, &Organization{}, &Department{}, &Workspace{}, &Role{}, &RoleBinding{}, &EnterpriseTask{}, &EnterpriseDevice{}, &EnterpriseDeviceAssignment{}, &QuotaAccount{}, &QuotaLedger{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasConstraint(&EnterpriseTask{}, "ParentTask") {
		t.Fatal("expected parent-task composite foreign key")
	}
}

func TestEnterpriseWorkModelsMigrateAndEnforceScopeUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:enterprise-work-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Organization{}, &EnterpriseTask{}, &EnterpriseTaskAssignment{}, &EnterpriseDevice{}, &EnterpriseDeviceAssignment{}, &QuotaAccount{}, &QuotaLedger{}); err != nil {
		t.Fatal(err)
	}
	organization := Organization{Slug: "enterprise-work", Name: "Enterprise", CreatedByUserID: 1}
	if err := db.Create(&organization).Error; err != nil {
		t.Fatal(err)
	}
	task := EnterpriseTask{OrganizationID: organization.ID, CreatedByUserID: 1, OwnerUserID: 1, Title: "Prepare report"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	assignment := EnterpriseTaskAssignment{TaskID: task.ID, UserID: 1, Role: EnterpriseTaskAssignmentOwner, AssignedBy: 1}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&assignment).Error; err == nil {
		t.Fatal("duplicate task assignment was accepted")
	}
	device := EnterpriseDevice{OrganizationID: organization.ID, ExternalDeviceID: "connector-1", Name: "Sales laptop"}
	if err := db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&EnterpriseDevice{OrganizationID: organization.ID, ExternalDeviceID: "connector-1", Name: "Duplicate"}).Error; err == nil {
		t.Fatal("duplicate organization device was accepted")
	}
	scopeKey := "task:" + strconv.FormatUint(uint64(task.ID), 10)
	account := QuotaAccount{OrganizationID: organization.ID, ScopeType: QuotaScopeTask, ScopeKey: scopeKey, TaskID: &task.ID}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&QuotaAccount{OrganizationID: organization.ID, ScopeType: QuotaScopeTask, ScopeKey: scopeKey, TaskID: &task.ID}).Error; err == nil {
		t.Fatal("duplicate quota account was accepted")
	}
}

func TestDeviceAssignmentScopeValidation(t *testing.T) {
	departmentID, userID, taskID := uint(1), uint(2), uint(3)
	if !DeviceAssignmentScopeValid(EnterpriseDeviceAssignmentDepartment, &departmentID, nil, nil) {
		t.Fatal("department assignment rejected")
	}
	if !DeviceAssignmentScopeValid(EnterpriseDeviceAssignmentUser, nil, &userID, nil) {
		t.Fatal("user assignment rejected")
	}
	if !DeviceAssignmentScopeValid(EnterpriseDeviceAssignmentTask, nil, nil, &taskID) {
		t.Fatal("task assignment rejected")
	}
	if DeviceAssignmentScopeValid(EnterpriseDeviceAssignmentTask, &departmentID, nil, &taskID) {
		t.Fatal("mixed device assignment accepted")
	}
}
