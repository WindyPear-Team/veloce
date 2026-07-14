package model

import (
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

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
