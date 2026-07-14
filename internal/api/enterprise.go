package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/WindyPear-Team/veloce/internal/middleware"
	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/WindyPear-Team/veloce/internal/service"
	"github.com/gin-gonic/gin"
)

type EnterpriseAPI struct{}

type enterpriseWorkspaceResponse struct {
	Workspace model.Workspace `json:"workspace"`
	Role      string          `json:"role"`
}

type enterpriseContextInput struct {
	WorkspaceID   *uint  `json:"workspace_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

func (api *EnterpriseAPI) GetOrganization(c *gin.Context) {
	if _, ok := enterpriseCurrentUser(c); !ok || !enterpriseFeatureAvailable(c) {
		return
	}
	tenant, ok := middleware.CurrentTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Enterprise tenant context is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"organization": tenant.Organization, "role": tenant.OrganizationMember.Role})
}

func (api *EnterpriseAPI) ListWorkspaces(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok || !enterpriseFeatureAvailable(c) {
		return
	}
	tenant, ok := middleware.CurrentTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Enterprise tenant context is unavailable"})
		return
	}
	var memberships []model.WorkspaceMember
	if err := model.DB.Preload("Workspace").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id AND workspaces.deleted_at IS NULL").
		Where("workspace_members.user_id = ? AND workspace_members.deleted_at IS NULL", user.ID).
		Where("workspaces.organization_id = ? AND workspaces.status = ?", tenant.Organization.ID, model.WorkspaceStatusActive).
		Order("workspace_members.id ASC").Find(&memberships).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workspaces"})
		return
	}
	items := make([]enterpriseWorkspaceResponse, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Workspace.ID == 0 {
			continue
		}
		items = append(items, enterpriseWorkspaceResponse{Workspace: membership.Workspace, Role: membership.Role})
	}
	c.JSON(http.StatusOK, gin.H{"workspaces": items})
}

func (api *EnterpriseAPI) SelectContext(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok || !enterpriseFeatureAvailable(c) {
		return
	}
	var input enterpriseContextInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid enterprise context"})
		return
	}
	selection := middleware.TenantSelection{
		WorkspaceSlug: strings.TrimSpace(input.WorkspaceSlug),
	}
	if input.WorkspaceID != nil {
		selection.WorkspaceID = strconv.FormatUint(uint64(*input.WorkspaceID), 10)
	}
	tenant, status, err := middleware.ResolveTenantContext(model.DB, user.ID, selection)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tenant)
}

func enterpriseFeatureAvailable(c *gin.Context) bool {
	if service.EnterpriseFeaturesEnabled() {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Enterprise features are disabled"})
	return false
}

func enterpriseCurrentUser(c *gin.Context) (*model.User, bool) {
	value, exists := c.Get("user")
	user, ok := value.(*model.User)
	if !exists || !ok || user == nil || user.ID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return nil, false
	}
	return user, true
}
