package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WindyPear-Team/veloce/internal/config"
	"gorm.io/gorm"
)

// AfterCreate provisions the personal tenant boundary for every newly created
// user while enterprise features are enabled. Using the create transaction
// keeps user and tenant bootstrap atomic across all registration methods.
func (user *User) AfterCreate(tx *gorm.DB) error {
	if !config.EnterpriseFeaturesEnabled || user == nil || user.ID == 0 {
		return nil
	}
	return ensurePersonalTenantWithDB(tx, user)
}

// EnsurePersonalTenantForUser is safe to call repeatedly. It creates the
// user's personal organization, owner memberships, and personal workspace if
// any part of the bootstrap data is missing.
func EnsurePersonalTenantForUser(db *gorm.DB, user *User) error {
	if db == nil {
		return errors.New("database is required")
	}
	if user == nil || user.ID == 0 {
		return errors.New("persisted user is required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return ensurePersonalTenantWithDB(tx, user)
	})
}

// EnsurePersonalTenantsForExistingUsers backfills enterprise tenant data for
// installations that already contained users when the feature was enabled.
func EnsurePersonalTenantsForExistingUsers(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is required")
	}
	var users []User
	return db.Order("id ASC").FindInBatches(&users, 100, func(batch *gorm.DB, _ int) error {
		for index := range users {
			if err := ensurePersonalTenantWithDB(batch, &users[index]); err != nil {
				return fmt.Errorf("bootstrap personal tenant for user %d: %w", users[index].ID, err)
			}
		}
		return nil
	}).Error
}

func ensurePersonalTenantWithDB(db *gorm.DB, user *User) error {
	organization := Organization{}
	organizationSlug := personalOrganizationSlug(user.ID)
	err := db.Where("slug = ?", organizationSlug).First(&organization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		organization = Organization{
			Slug:            organizationSlug,
			Name:            personalOrganizationName(user),
			Status:          OrganizationStatusActive,
			CreatedByUserID: user.ID,
		}
		if err := db.Create(&organization).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := EnsureOrganizationRBAC(db, organization.ID, user.ID); err != nil {
		return err
	}

	joinedAt := time.Now()
	organizationMember := OrganizationMember{}
	err = db.Where("organization_id = ? AND user_id = ?", organization.ID, user.ID).First(&organizationMember).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		organizationMember = OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			Role:           OrganizationMemberRoleOwner,
			Status:         OrganizationMemberStatusActive,
			JoinedAt:       &joinedAt,
		}
		if err := db.Create(&organizationMember).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	workspace := Workspace{}
	err = db.Where("organization_id = ? AND slug = ?", organization.ID, "personal").First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		workspace = Workspace{
			OrganizationID:  organization.ID,
			Slug:            "personal",
			Name:            "Personal",
			Type:            WorkspaceTypePersonal,
			Status:          WorkspaceStatusActive,
			CreatedByUserID: user.ID,
		}
		if err := db.Create(&workspace).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	workspaceMember := WorkspaceMember{}
	err = db.Where("workspace_id = ? AND user_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		workspaceMember = WorkspaceMember{
			WorkspaceID: workspace.ID,
			UserID:      user.ID,
			Role:        WorkspaceMemberRoleOwner,
		}
		return db.Create(&workspaceMember).Error
	}
	return err
}

func personalOrganizationSlug(userID uint) string {
	return fmt.Sprintf("personal-u-%d", userID)
}

func personalOrganizationName(user *User) string {
	name := strings.TrimSpace(user.Username)
	if name == "" {
		name = strings.TrimSpace(user.Email)
	}
	if name == "" {
		name = fmt.Sprintf("User %d", user.ID)
	}
	return name + " Personal Organization"
}
