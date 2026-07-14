package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	EnterpriseOrganizationSlug = "enterprise"
	EnterpriseSystemMode       = "enterprise"
)

// AfterCreate joins every new user to the single enterprise represented by
// this private deployment. The first user creates and owns the enterprise;
// subsequent users become standard members.
func (user *User) AfterCreate(tx *gorm.DB) error {
	if !EnterpriseModeEnabledWithDB(tx) || user == nil || user.ID == 0 {
		return nil
	}
	return ensureEnterpriseTenantWithDB(tx, user)
}

func EnterpriseModeEnabledWithDB(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(GetSystemSettingWithDB(db, "system_mode", "operation")), EnterpriseSystemMode)
}

// EnsureEnterpriseTenantForUser is idempotent. One deployment owns exactly one
// enterprise organization, while each employee receives a personal workspace
// inside that organization.
func EnsureEnterpriseTenantForUser(db *gorm.DB, user *User) error {
	if db == nil {
		return errors.New("database is required")
	}
	if user == nil || user.ID == 0 {
		return errors.New("persisted user is required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return ensureEnterpriseTenantWithDB(tx, user)
	})
}

// EnsureEnterpriseTenantForExistingUsers backfills the single enterprise and
// employee workspaces for installations that already contain users.
func EnsureEnterpriseTenantForExistingUsers(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is required")
	}
	var users []User
	err := db.Order("id ASC").FindInBatches(&users, 100, func(batch *gorm.DB, _ int) error {
		for index := range users {
			if err := ensureEnterpriseTenantWithDB(batch, &users[index]); err != nil {
				return fmt.Errorf("bootstrap enterprise tenant for user %d: %w", users[index].ID, err)
			}
		}
		return nil
	}).Error
	if err != nil {
		return err
	}
	// Early enterprise-development builds created one organization per user.
	// They never owned shared enterprise resources, so retire those records once
	// all users have been attached to the singleton enterprise.
	return db.Model(&Organization{}).
		Where("slug LIKE ? AND slug <> ?", "personal-u-%", EnterpriseOrganizationSlug).
		Update("status", OrganizationStatusSuspended).Error
}

func ensureEnterpriseTenantWithDB(db *gorm.DB, user *User) error {
	organization := Organization{}
	err := db.Where("slug = ?", EnterpriseOrganizationSlug).First(&organization).Error
	createdOrganization := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		organization = Organization{
			Slug:            EnterpriseOrganizationSlug,
			Name:            "Enterprise",
			Status:          OrganizationStatusActive,
			CreatedByUserID: user.ID,
		}
		if err := db.Create(&organization).Error; err != nil {
			return err
		}
		createdOrganization = true
	} else if err != nil {
		return err
	}

	ownerUserID := uint(0)
	if createdOrganization || organization.CreatedByUserID == user.ID {
		ownerUserID = user.ID
	}
	if err := EnsureOrganizationRBAC(db, organization.ID, ownerUserID); err != nil {
		return err
	}

	memberRole := OrganizationMemberRoleMember
	builtinRole := BuiltinRoleMember
	if ownerUserID != 0 {
		memberRole = OrganizationMemberRoleOwner
		builtinRole = BuiltinRoleOrganizationAdmin
	}
	joinedAt := time.Now()
	organizationMember := OrganizationMember{}
	err = db.Where("organization_id = ? AND user_id = ?", organization.ID, user.ID).First(&organizationMember).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		organizationMember = OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			Role:           memberRole,
			Status:         OrganizationMemberStatusActive,
			JoinedAt:       &joinedAt,
		}
		if err := db.Create(&organizationMember).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := EnsureOrganizationRoleBinding(db, organization.ID, user.ID, user.ID, builtinRole); err != nil {
		return err
	}

	workspaceSlug := personalWorkspaceSlug(user.ID)
	workspace := Workspace{}
	err = db.Where("organization_id = ? AND slug = ?", organization.ID, workspaceSlug).First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		workspace = Workspace{
			OrganizationID:  organization.ID,
			Slug:            workspaceSlug,
			Name:            personalWorkspaceName(user),
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
		return db.Create(&WorkspaceMember{
			WorkspaceID: workspace.ID,
			UserID:      user.ID,
			Role:        WorkspaceMemberRoleOwner,
		}).Error
	}
	return err
}

func personalWorkspaceSlug(userID uint) string {
	return fmt.Sprintf("personal-u-%d", userID)
}

func personalWorkspaceName(user *User) string {
	name := strings.TrimSpace(user.Username)
	if name == "" {
		name = strings.TrimSpace(user.Email)
	}
	if name == "" {
		name = fmt.Sprintf("User %d", user.ID)
	}
	return name + " Personal Workspace"
}
