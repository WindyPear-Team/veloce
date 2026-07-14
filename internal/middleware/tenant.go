package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/WindyPear-Team/veloce/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	tenantContextKey       = "enterprise_tenant_context"
	organizationIDHeader   = "X-Organization-ID"
	organizationSlugHeader = "X-Organization-Slug"
	workspaceIDHeader      = "X-Workspace-ID"
	workspaceSlugHeader    = "X-Workspace-Slug"
)

type TenantContext struct {
	Organization       model.Organization       `json:"organization"`
	OrganizationMember model.OrganizationMember `json:"organization_member"`
	Workspace          *model.Workspace         `json:"workspace,omitempty"`
	WorkspaceMember    *model.WorkspaceMember   `json:"workspace_member,omitempty"`
}

// TenantContextMiddleware resolves and validates the current organization and
// workspace after authentication. It is a no-op while enterprise features are
// disabled, preserving the existing community behavior.
func TenantContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.EnterpriseFeaturesEnabled() {
			c.Next()
			return
		}
		value, exists := c.Get("user")
		user, ok := value.(*model.User)
		if !exists || !ok || user == nil || user.ID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		tenant, status, err := ResolveTenantContext(model.DB, user.ID, TenantSelection{
			OrganizationID:   strings.TrimSpace(c.GetHeader(organizationIDHeader)),
			OrganizationSlug: strings.TrimSpace(c.GetHeader(organizationSlugHeader)),
			WorkspaceID:      strings.TrimSpace(c.GetHeader(workspaceIDHeader)),
			WorkspaceSlug:    strings.TrimSpace(c.GetHeader(workspaceSlugHeader)),
		})
		if err != nil {
			c.JSON(status, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Set(tenantContextKey, tenant)
		c.Next()
	}
}

type TenantSelection struct {
	OrganizationID   string
	OrganizationSlug string
	WorkspaceID      string
	WorkspaceSlug    string
}

func ResolveTenantContext(db *gorm.DB, userID uint, selection TenantSelection) (*TenantContext, int, error) {
	if db == nil {
		return nil, http.StatusInternalServerError, errors.New("enterprise database is unavailable")
	}
	organizationMember, organization, status, err := resolveOrganization(db, userID, selection)
	if err != nil {
		return nil, status, err
	}
	workspaceMember, workspace, status, err := resolveWorkspace(db, userID, organization.ID, selection)
	if err != nil {
		return nil, status, err
	}
	return &TenantContext{
		Organization:       organization,
		OrganizationMember: organizationMember,
		Workspace:          workspace,
		WorkspaceMember:    workspaceMember,
	}, http.StatusOK, nil
}

func CurrentTenantContext(c *gin.Context) (*TenantContext, bool) {
	value, exists := c.Get(tenantContextKey)
	if !exists {
		return nil, false
	}
	tenant, ok := value.(*TenantContext)
	return tenant, ok && tenant != nil
}

func resolveOrganization(db *gorm.DB, userID uint, selection TenantSelection) (model.OrganizationMember, model.Organization, int, error) {
	query := db.Model(&model.OrganizationMember{}).
		Joins("JOIN organizations ON organizations.id = organization_members.organization_id AND organizations.deleted_at IS NULL").
		Where("organization_members.user_id = ? AND organization_members.status = ? AND organization_members.deleted_at IS NULL", userID, model.OrganizationMemberStatusActive).
		Where("organizations.status = ?", model.OrganizationStatusActive)

	if selection.OrganizationID != "" {
		organizationID, err := parsePositiveID(selection.OrganizationID, "organization")
		if err != nil {
			return model.OrganizationMember{}, model.Organization{}, http.StatusBadRequest, err
		}
		query = query.Where("organizations.id = ?", organizationID)
	} else if selection.OrganizationSlug != "" {
		query = query.Where("organizations.slug = ?", selection.OrganizationSlug)
	}

	var membership model.OrganizationMember
	if err := query.Order("organization_members.id ASC").First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.OrganizationMember{}, model.Organization{}, http.StatusForbidden, errors.New("organization access denied")
		}
		return model.OrganizationMember{}, model.Organization{}, http.StatusInternalServerError, errors.New("failed to resolve organization")
	}
	var organization model.Organization
	if err := db.First(&organization, membership.OrganizationID).Error; err != nil {
		return model.OrganizationMember{}, model.Organization{}, http.StatusInternalServerError, errors.New("failed to load organization")
	}
	return membership, organization, http.StatusOK, nil
}

func resolveWorkspace(db *gorm.DB, userID, organizationID uint, selection TenantSelection) (*model.WorkspaceMember, *model.Workspace, int, error) {
	query := db.Model(&model.WorkspaceMember{}).
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id AND workspaces.deleted_at IS NULL").
		Where("workspace_members.user_id = ? AND workspace_members.deleted_at IS NULL", userID).
		Where("workspaces.organization_id = ? AND workspaces.status = ?", organizationID, model.WorkspaceStatusActive)

	explicit := selection.WorkspaceID != "" || selection.WorkspaceSlug != ""
	if selection.WorkspaceID != "" {
		workspaceID, err := parsePositiveID(selection.WorkspaceID, "workspace")
		if err != nil {
			return nil, nil, http.StatusBadRequest, err
		}
		query = query.Where("workspaces.id = ?", workspaceID)
	} else if selection.WorkspaceSlug != "" {
		query = query.Where("workspaces.slug = ?", selection.WorkspaceSlug)
	}

	var membership model.WorkspaceMember
	err := query.Order("CASE WHEN workspaces.type = 'personal' THEN 0 ELSE 1 END").Order("workspace_members.id ASC").First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if explicit {
			return nil, nil, http.StatusForbidden, errors.New("workspace access denied")
		}
		return nil, nil, http.StatusOK, nil
	}
	if err != nil {
		return nil, nil, http.StatusInternalServerError, errors.New("failed to resolve workspace")
	}
	var workspace model.Workspace
	if err := db.First(&workspace, membership.WorkspaceID).Error; err != nil {
		return nil, nil, http.StatusInternalServerError, errors.New("failed to load workspace")
	}
	return &membership, &workspace, http.StatusOK, nil
}

func parsePositiveID(value, resource string) (uint64, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New(resource + " id must be a positive integer")
	}
	return id, nil
}
