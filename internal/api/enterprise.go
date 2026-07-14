package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WindyPear-Team/veloce/internal/middleware"
	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/WindyPear-Team/veloce/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

type enterpriseRoleInput struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type enterpriseBindingInput struct {
	UserID    uint   `json:"user_id"`
	RoleID    uint   `json:"role_id"`
	ScopeType string `json:"scope_type"`
	ScopeID   uint   `json:"scope_id"`
}

type enterpriseTaskInput struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DepartmentID       *uint  `json:"department_id"`
	OwnerUserID        *uint  `json:"owner_user_id"`
	AssigneeUserIDs    []uint `json:"assignee_user_ids"`
	ParticipantUserIDs []uint `json:"participant_user_ids"`
}

type enterpriseTaskStatusInput struct {
	Status string `json:"status"`
}

type enterpriseDeviceInput struct {
	ExternalDeviceID    string `json:"external_device_id"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	OwnerUserID         *uint  `json:"owner_user_id"`
	ManagedByEnterprise bool   `json:"managed_by_enterprise"`
}
type enterpriseDeviceAssignmentInput struct {
	DeviceID       uint       `json:"device_id"`
	UserID         *uint      `json:"user_id"`
	DepartmentID   *uint      `json:"department_id"`
	TaskID         *uint      `json:"task_id"`
	AllowedTools   []string   `json:"allowed_tools"`
	Classification string     `json:"classification"`
	ExpiresAt      *time.Time `json:"expires_at"`
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

func (api *EnterpriseAPI) PreviewPermissions(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok || !enterpriseFeatureAvailable(c) {
		return
	}
	tenant, ok := middleware.CurrentTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Enterprise tenant context is unavailable"})
		return
	}
	workspaceID := uint(0)
	if tenant.Workspace != nil {
		workspaceID = tenant.Workspace.ID
	}
	c.JSON(http.StatusOK, gin.H{"permissions": middleware.EffectivePermissions(model.DB, user.ID, tenant.Organization.ID, workspaceID)})
}

func (api *EnterpriseAPI) ListPermissions(c *gin.Context) {
	if !enterpriseFeatureAvailable(c) {
		return
	}
	var permissions []model.Permission
	if err := model.DB.Order("resource ASC, action ASC").Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list permissions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": permissions})
}

func (api *EnterpriseAPI) ListMembers(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var members []model.OrganizationMember
	if err := model.DB.Preload("User").Where("organization_id = ?", tenant.Organization.ID).Order("id ASC").Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list members"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (api *EnterpriseAPI) ListRoles(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var roles []model.Role
	if err := model.DB.Where("organization_id = ?", tenant.Organization.ID).Order("builtin DESC, name ASC").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list roles"})
		return
	}
	for i := range roles {
		roles[i].Description = strings.TrimSpace(roles[i].Description)
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

func (api *EnterpriseAPI) CreateRole(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var input enterpriseRoleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}
	role, err := enterpriseCreateOrUpdateRole(tenant.Organization.ID, 0, input)
	if err != nil {
		enterpriseWriteRoleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, role)
}

func (api *EnterpriseAPI) UpdateRole(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	roleID, err := parseEnterpriseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var input enterpriseRoleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}
	role, err := enterpriseCreateOrUpdateRole(tenant.Organization.ID, roleID, input)
	if err != nil {
		enterpriseWriteRoleError(c, err)
		return
	}
	c.JSON(http.StatusOK, role)
}

func (api *EnterpriseAPI) ListRoleBindings(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var bindings []model.RoleBinding
	if err := model.DB.Preload("Role").Where("organization_id = ?", tenant.Organization.ID).Order("id ASC").Find(&bindings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list role bindings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bindings": bindings})
}

func (api *EnterpriseAPI) CreateRoleBinding(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var input enterpriseBindingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role binding"})
		return
	}
	input.ScopeType = model.NormalizeRoleBindingScope(input.ScopeType)
	if !model.RoleBindingScopeValid(input.ScopeType, input.ScopeID, tenant.Organization.ID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role binding scope"})
		return
	}
	var membership model.OrganizationMember
	if err := model.DB.Where("organization_id = ? AND user_id = ? AND status = ?", tenant.Organization.ID, input.UserID, model.OrganizationMemberStatusActive).First(&membership).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Active organization member not found"})
		return
	}
	var role model.Role
	if err := model.DB.Where("id = ? AND organization_id = ?", input.RoleID, tenant.Organization.ID).First(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role not found"})
		return
	}
	if input.ScopeType == model.RoleBindingScopeWorkspace {
		var workspace model.Workspace
		if err := model.DB.Where("id = ? AND organization_id = ?", input.ScopeID, tenant.Organization.ID).First(&workspace).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Workspace not found"})
			return
		}
	}
	binding := model.RoleBinding{OrganizationID: tenant.Organization.ID, UserID: input.UserID, RoleID: input.RoleID, ScopeType: input.ScopeType, ScopeID: input.ScopeID, CreatedByUserID: user.ID}
	if err := model.DB.Where("organization_id = ? AND user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?", binding.OrganizationID, binding.UserID, binding.RoleID, binding.ScopeType, binding.ScopeID).FirstOrCreate(&binding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to grant role"})
		return
	}
	c.JSON(http.StatusCreated, binding)
}

func (api *EnterpriseAPI) DeleteRoleBinding(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	id, err := parseEnterpriseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := model.DB.Where("id = ? AND organization_id = ?", id, tenant.Organization.ID).Delete(&model.RoleBinding{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke role"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role binding not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Role revoked"})
}

func (api *EnterpriseAPI) ListTasks(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var tasks []model.EnterpriseTask
	query := model.DB.Where("organization_id = ?", tenant.Organization.ID).
		Where("created_by_user_id = ? OR owner_user_id = ? OR id IN (?)", user.ID, user.ID, model.DB.Model(&model.EnterpriseTaskAssignment{}).Select("task_id").Where("user_id = ?", user.ID)).
		Order("updated_at DESC")
	if err := query.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list tasks"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (api *EnterpriseAPI) CreateTask(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var input enterpriseTaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task"})
		return
	}
	input.Title, input.Description = strings.TrimSpace(input.Title), strings.TrimSpace(input.Description)
	if input.Title == "" || len([]rune(input.Title)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task title is required and must not exceed 200 characters"})
		return
	}
	ownerID := user.ID
	if input.OwnerUserID != nil {
		ownerID = *input.OwnerUserID
	}
	if !enterpriseActiveMember(tenant.Organization.ID, ownerID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task owner must be an active organization member"})
		return
	}
	if input.DepartmentID != nil {
		var department model.Department
		if err := model.DB.Where("id = ? AND organization_id = ?", *input.DepartmentID, tenant.Organization.ID).First(&department).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Department not found"})
			return
		}
	}
	task := model.EnterpriseTask{OrganizationID: tenant.Organization.ID, DepartmentID: input.DepartmentID, CreatedByUserID: user.ID, OwnerUserID: ownerID, Title: input.Title, Description: input.Description, Status: model.EnterpriseTaskStatusAssigned}
	if tenant.Workspace != nil {
		task.WorkspaceID = &tenant.Workspace.ID
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		assignments := []model.EnterpriseTaskAssignment{{TaskID: task.ID, UserID: ownerID, Role: model.EnterpriseTaskAssignmentOwner, AssignedBy: user.ID}}
		for _, assigneeID := range uniqueEnterpriseUserIDs(input.AssigneeUserIDs) {
			if assigneeID != ownerID && enterpriseActiveMemberWithDB(tx, tenant.Organization.ID, assigneeID) {
				assignments = append(assignments, model.EnterpriseTaskAssignment{TaskID: task.ID, UserID: assigneeID, Role: model.EnterpriseTaskAssignmentAssignee, AssignedBy: user.ID})
			}
		}
		for _, participantID := range uniqueEnterpriseUserIDs(input.ParticipantUserIDs) {
			if participantID != ownerID && enterpriseActiveMemberWithDB(tx, tenant.Organization.ID, participantID) {
				assignments = append(assignments, model.EnterpriseTaskAssignment{TaskID: task.ID, UserID: participantID, Role: model.EnterpriseTaskAssignmentParticipant, AssignedBy: user.ID})
			}
		}
		return tx.Create(&assignments).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}
	c.JSON(http.StatusCreated, task)
}

func (api *EnterpriseAPI) UpdateTaskStatus(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	id, err := parseEnterpriseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var input enterpriseTaskStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task status"})
		return
	}
	status := model.NormalizeEnterpriseTaskStatus(input.Status)
	var task model.EnterpriseTask
	if err := model.DB.Where("id = ? AND organization_id = ?", id, tenant.Organization.ID).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if task.OwnerUserID != user.ID && task.CreatedByUserID != user.ID && !enterpriseTaskAssignedTo(task.ID, user.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Task access denied"})
		return
	}
	updates := map[string]interface{}{"status": status}
	if status == model.EnterpriseTaskStatusRunning {
		updates["started_at"] = time.Now()
	}
	if status == model.EnterpriseTaskStatusCompleted || status == model.EnterpriseTaskStatusCancelled {
		updates["completed_at"] = time.Now()
	}
	if err := model.DB.Model(&task).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}
	model.DB.First(&task, task.ID)
	c.JSON(http.StatusOK, task)
}

func (api *EnterpriseAPI) ListDevices(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var devices []model.EnterpriseDevice
	if err := model.DB.Where("organization_id = ?", tenant.Organization.ID).Order("created_at DESC").Find(&devices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list devices"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}
func (api *EnterpriseAPI) CreateDevice(c *gin.Context) {
	_, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var input enterpriseDeviceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device"})
		return
	}
	input.ExternalDeviceID, input.Name, input.Kind = strings.TrimSpace(input.ExternalDeviceID), strings.TrimSpace(input.Name), strings.TrimSpace(input.Kind)
	if input.ExternalDeviceID == "" || input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device id and name are required"})
		return
	}
	if input.Kind == "" {
		input.Kind = "connector"
	}
	device := model.EnterpriseDevice{OrganizationID: tenant.Organization.ID, ExternalDeviceID: input.ExternalDeviceID, Name: input.Name, Kind: input.Kind, OwnerUserID: input.OwnerUserID, ManagedByEnterprise: input.ManagedByEnterprise, Status: model.EnterpriseDeviceStatusActive}
	if err := model.DB.Create(&device).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Failed to create device"})
		return
	}
	c.JSON(http.StatusCreated, device)
}
func (api *EnterpriseAPI) ListDeviceAssignments(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var assignments []model.EnterpriseDeviceAssignment
	if err := model.DB.Where("organization_id = ?", tenant.Organization.ID).Order("created_at DESC").Find(&assignments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list device assignments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assignments": assignments})
}
func (api *EnterpriseAPI) AssignDevice(c *gin.Context) {
	user, ok := enterpriseCurrentUser(c)
	if !ok {
		return
	}
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var input enterpriseDeviceAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device assignment"})
		return
	}
	assignment, err := service.AssignEnterpriseDevice(model.DB, service.EnterpriseDeviceAssignmentInput{OrganizationID: tenant.Organization.ID, DeviceID: input.DeviceID, DepartmentID: input.DepartmentID, UserID: input.UserID, TaskID: input.TaskID, AllowedTools: input.AllowedTools, Classification: input.Classification, AssignedBy: user.ID, ExpiresAt: input.ExpiresAt})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, assignment)
}
func (api *EnterpriseAPI) ListQuotaAccounts(c *gin.Context) {
	tenant, ok := enterpriseTenant(c)
	if !ok {
		return
	}
	var accounts []model.QuotaAccount
	if err := model.DB.Where("organization_id = ?", tenant.Organization.ID).Order("scope_type ASC, id ASC").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list quota accounts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": accounts})
}

func enterpriseActiveMember(organizationID, userID uint) bool {
	return enterpriseActiveMemberWithDB(model.DB, organizationID, userID)
}
func enterpriseActiveMemberWithDB(db *gorm.DB, organizationID, userID uint) bool {
	var count int64
	return db.Model(&model.OrganizationMember{}).Where("organization_id = ? AND user_id = ? AND status = ?", organizationID, userID, model.OrganizationMemberStatusActive).Count(&count).Error == nil && count == 1
}
func enterpriseTaskAssignedTo(taskID, userID uint) bool {
	var count int64
	return model.DB.Model(&model.EnterpriseTaskAssignment{}).Where("task_id = ? AND user_id = ?", taskID, userID).Count(&count).Error == nil && count > 0
}
func uniqueEnterpriseUserIDs(values []uint) []uint {
	seen := map[uint]struct{}{}
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value != 0 {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	return result
}

func enterpriseTenant(c *gin.Context) (*middleware.TenantContext, bool) {
	if !enterpriseFeatureAvailable(c) {
		return nil, false
	}
	tenant, ok := middleware.CurrentTenantContext(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Enterprise tenant context is required"})
	}
	return tenant, ok
}

func enterpriseCreateOrUpdateRole(organizationID, roleID uint, input enterpriseRoleInput) (model.Role, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	if input.Slug == "" || input.Name == "" {
		return model.Role{}, errors.New("role slug and name are required")
	}
	role := model.Role{}
	if roleID != 0 {
		if err := model.DB.Where("id = ? AND organization_id = ?", roleID, organizationID).First(&role).Error; err != nil {
			return role, err
		}
		if role.Builtin {
			return role, errors.New("built-in roles cannot be changed")
		}
	} else {
		role = model.Role{OrganizationID: organizationID, Slug: input.Slug, Name: input.Name, Description: strings.TrimSpace(input.Description)}
	}
	return role, model.DB.Transaction(func(tx *gorm.DB) error {
		if roleID == 0 {
			if err := tx.Create(&role).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&role).Updates(map[string]interface{}{"slug": input.Slug, "name": input.Name, "description": strings.TrimSpace(input.Description)}).Error; err != nil {
			return err
		}
		if input.Permissions == nil {
			return nil
		}
		var permissions []model.Permission
		if len(input.Permissions) > 0 {
			if err := tx.Where("code IN ?", input.Permissions).Find(&permissions).Error; err != nil {
				return err
			}
			if len(permissions) != len(input.Permissions) {
				return errors.New("one or more permissions do not exist")
			}
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		for _, permission := range permissions {
			if err := tx.Create(&model.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func enterpriseWriteRoleError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
func parseEnterpriseID(value string) (uint, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return uint(id), nil
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
