package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	advancedChatConnectorDeviceStatusOffline = "offline"
	advancedChatConnectorDeviceStatusOnline  = "online"

	advancedChatConnectorTaskStatusPendingApproval = "pending_approval"
	advancedChatConnectorTaskStatusQueued          = "queued"
	advancedChatConnectorTaskStatusRunning         = "running"
	advancedChatConnectorTaskStatusCompleted       = "completed"
	advancedChatConnectorTaskStatusFailed          = "failed"

	advancedChatConnectorOnlineWindow = 60 * time.Second
	advancedChatConnectorTaskWait     = 10 * time.Minute
	advancedChatAgentSkillsLoadWait   = 20 * time.Second

	advancedChatAgentSkillsMaxFiles      = 40
	advancedChatAgentSkillsMaxFileBytes  = 64 * 1024
	advancedChatAgentSkillsMaxTotalBytes = 256 * 1024

	advancedChatConnectorToolListFiles     = "workspace_list_files"
	advancedChatConnectorToolReadFile      = "workspace_read_file"
	advancedChatConnectorToolWriteFile     = "workspace_write_file"
	advancedChatConnectorToolReplaceText   = "workspace_replace_text"
	advancedChatConnectorToolRunCommand    = "workspace_run_command"
	advancedChatConnectorToolWebSearch     = "workspace_web_search"
	advancedChatConnectorToolWebFetch      = "workspace_web_fetch"
	advancedChatConnectorToolWindowsDrives = "workspace_list_windows_drives"

	advancedChatConnectorPreviewOldContent          = "preview_old_content"
	advancedChatConnectorPreviewOldContentAvailable = "preview_old_content_available"
	advancedChatConnectorPreviewToolCallID          = "preview_tool_call_id"
	advancedChatConnectorTaskID                     = "connector_task_id"
)

type AdvancedChatConnectorDevice struct {
	ID        string     `gorm:"primaryKey;size:80" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"user_id"`
	User      model.User `gorm:"foreignKey:UserID" json:"-"`
	TokenHash string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Name      string     `gorm:"size:120;not null" json:"name"`
	Remark    string     `gorm:"size:200;not null;default:''" json:"remark"`
	Hostname  string     `gorm:"size:120" json:"hostname"`
	OS        string     `gorm:"size:40" json:"os"`
	Arch      string     `gorm:"size:40" json:"arch"`
	Version   string     `gorm:"size:80" json:"version"`
	Status    string     `gorm:"size:20;index;not null" json:"status"`
	// Kept for existing databases that already migrated the previous connector schema.
	// New connector versions no longer register workspace paths on the device.
	Workspaces string     `gorm:"type:text;not null" json:"-"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type AdvancedChatConnectorTask struct {
	ID            string     `gorm:"primaryKey;size:80" json:"id"`
	UserID        uint       `gorm:"index;not null" json:"user_id"`
	DeviceID      string     `gorm:"index;size:80;not null" json:"device_id"`
	RunID         string     `gorm:"index;size:80" json:"run_id"`
	Action        string     `gorm:"size:80;not null" json:"action"`
	WorkspacePath string     `gorm:"type:text;not null" json:"workspace_path"`
	Payload       string     `gorm:"type:text;not null" json:"-"`
	Status        string     `gorm:"size:20;index;not null" json:"status"`
	Result        string     `gorm:"type:text;not null" json:"result"`
	ErrorMessage  string     `gorm:"type:text;not null" json:"error_message"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type advancedChatConnectorDeviceResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Remark     string     `json:"remark"`
	Hostname   string     `json:"hostname,omitempty"`
	OS         string     `json:"os,omitempty"`
	Arch       string     `json:"arch,omitempty"`
	Version    string     `json:"version,omitempty"`
	Status     string     `json:"status"`
	Online     bool       `json:"online"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type advancedChatConnectorTokenInput struct {
	Name   string `json:"name"`
	Remark string `json:"remark"`
}

type advancedChatConnectorDeviceUpdateInput struct {
	Name   *string `json:"name"`
	Remark *string `json:"remark"`
}

type advancedChatConnectorRegisterInput struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
}

type advancedChatConnectorTaskResponse struct {
	ID                    string                 `json:"id"`
	Action                string                 `json:"action"`
	WorkspacePath         string                 `json:"workspace_path"`
	WorkspaceUnrestricted bool                   `json:"workspace_unrestricted"`
	Payload               map[string]interface{} `json:"payload"`
	CreatedAt             time.Time              `json:"created_at"`
}

type advancedChatConnectorTaskApprovalResponse struct {
	ID                    string                 `json:"id"`
	DeviceID              string                 `json:"device_id"`
	DeviceName            string                 `json:"device_name"`
	RunID                 string                 `json:"run_id"`
	Action                string                 `json:"action"`
	WorkspacePath         string                 `json:"workspace_path"`
	WorkspaceUnrestricted bool                   `json:"workspace_unrestricted"`
	Payload               map[string]interface{} `json:"payload"`
	CreatedAt             time.Time              `json:"created_at"`
}

type advancedChatConnectorTaskResultInput struct {
	Success  bool   `json:"success"`
	Result   string `json:"result"`
	Output   string `json:"output"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error"`
	ExitCode *int   `json:"exit_code"`
}

type advancedChatConnectorTaskDecisionInput struct {
	Approved bool `json:"approved"`
}

type advancedChatWorkspaceSkillsRefreshInput struct {
	ConnectorDeviceID      string `json:"connector_device_id"`
	ConnectorWorkspacePath string `json:"connector_workspace_path"`
}

type advancedChatWorkspaceSkillResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int    `json:"size"`
	Truncated bool   `json:"truncated"`
}

type advancedChatConnectorToolBinding struct {
	DeviceID        string
	DeviceName      string
	WorkspacePath   string
	Action          string
	AutoApprove     bool
	CommandPrefixes []string
}

type advancedChatWorkspaceSkill struct {
	ID        string
	Name      string
	Path      string
	Content   string
	Size      int
	Truncated bool
}

func registerAdvancedChatConnectorRoutes(group *gin.RouterGroup) {
	api := &advancedChatAPI{}
	connectors := group.Group("/advanced-chat/connectors")
	connectors.POST("/register", api.connectorRegister)
	connectors.POST("/heartbeat", api.connectorHeartbeat)
	connectors.GET("/tasks/next", api.connectorNextTask)
	connectors.POST("/tasks/:id/result", api.connectorTaskResult)
}

func (api *advancedChatAPI) listConnectorDevices(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var devices []AdvancedChatConnectorDevice
	if err := model.DB.Where("user_id = ?", user.ID).Order("last_seen_at DESC, created_at DESC").Find(&devices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list devices"})
		return
	}
	responses := make([]advancedChatConnectorDeviceResponse, 0, len(devices))
	for _, device := range devices {
		responses = append(responses, advancedChatConnectorDeviceResponseFromModel(device))
	}
	c.JSON(http.StatusOK, responses)
}

func (api *advancedChatAPI) createConnectorToken(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var input advancedChatConnectorTokenInput
	_ = c.ShouldBindJSON(&input)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Local device"
	}
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	remark := truncateConnectorField(input.Remark, 200)
	token, err := newAdvancedChatConnectorToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create connector token"})
		return
	}
	now := time.Now()
	device := AdvancedChatConnectorDevice{
		ID:         newAdvancedChatID("acd"),
		UserID:     user.ID,
		TokenHash:  hashAdvancedChatConnectorToken(token),
		Name:       name,
		Remark:     remark,
		Status:     advancedChatConnectorDeviceStatusOffline,
		Workspaces: "[]",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := model.DB.Create(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save connector token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "device": advancedChatConnectorDeviceResponseFromModel(device)})
}

func (api *advancedChatAPI) rotateConnectorDeviceToken(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	deviceID := strings.TrimSpace(c.Param("id"))
	token, err := newAdvancedChatConnectorToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create connector token"})
		return
	}
	now := time.Now()
	update := model.DB.Model(&AdvancedChatConnectorDevice{}).
		Where("id = ? AND user_id = ?", deviceID, user.ID).
		Updates(map[string]interface{}{
			"token_hash": hashAdvancedChatConnectorToken(token),
			"updated_at": now,
		})
	if update.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save connector token"})
		return
	}
	if update.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}
	var device AdvancedChatConnectorDevice
	if err := model.DB.Where("id = ? AND user_id = ?", deviceID, user.ID).First(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector device"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "device": advancedChatConnectorDeviceResponseFromModel(device)})
}

func (api *advancedChatAPI) updateConnectorDevice(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	deviceID := strings.TrimSpace(c.Param("id"))
	var input advancedChatConnectorDeviceUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{"updated_at": time.Now()}
	if input.Name != nil {
		name := truncateConnectorField(*input.Name, 120)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Device name is required"})
			return
		}
		updates["name"] = name
	}
	if input.Remark != nil {
		updates["remark"] = truncateConnectorField(*input.Remark, 200)
	}
	update := model.DB.Model(&AdvancedChatConnectorDevice{}).
		Where("id = ? AND user_id = ?", deviceID, user.ID).
		Updates(updates)
	if update.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update connector device"})
		return
	}
	if update.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}
	var device AdvancedChatConnectorDevice
	if err := model.DB.Where("id = ? AND user_id = ?", deviceID, user.ID).First(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector device"})
		return
	}
	c.JSON(http.StatusOK, advancedChatConnectorDeviceResponseFromModel(device))
}

func (api *advancedChatAPI) listPendingConnectorTasks(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	runID := strings.TrimSpace(c.Param("id"))
	var tasks []AdvancedChatConnectorTask
	if err := model.DB.
		Where("user_id = ? AND run_id = ? AND status = ?", user.ID, runID, advancedChatConnectorTaskStatusPendingApproval).
		Order("created_at ASC").
		Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list connector tasks"})
		return
	}
	deviceIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		deviceIDs = append(deviceIDs, task.DeviceID)
	}
	devices := map[string]AdvancedChatConnectorDevice{}
	if len(deviceIDs) > 0 {
		var rows []AdvancedChatConnectorDevice
		if err := model.DB.Where("user_id = ? AND id IN ?", user.ID, deviceIDs).Find(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector devices"})
			return
		}
		for _, device := range rows {
			devices[device.ID] = device
		}
	}
	result := make([]advancedChatConnectorTaskApprovalResponse, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, advancedChatConnectorTaskApprovalResponseFromModel(task, devices[task.DeviceID]))
	}
	c.JSON(http.StatusOK, result)
}

func (api *advancedChatAPI) decideConnectorTask(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var input advancedChatConnectorTaskDecisionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID := strings.TrimSpace(c.Param("id"))
	status, err := decideAdvancedChatConnectorTask(user.ID, taskID, input.Approved, "user", "")
	if err != nil {
		var taskErr advancedChatConnectorTaskDecisionConflict
		if errors.As(err, &taskErr) {
			c.JSON(http.StatusConflict, gin.H{"error": "Connector task already decided", "status": taskErr.Status})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Connector task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update connector task"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": status})
}

type advancedChatConnectorTaskDecisionConflict struct {
	Status string
}

func (e advancedChatConnectorTaskDecisionConflict) Error() string {
	return "connector task already decided"
}

func decideAdvancedChatConnectorTask(userID uint, taskID string, approved bool, actor string, opinion string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "user"
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status":     advancedChatConnectorTaskStatusQueued,
		"updated_at": now,
	}
	status := advancedChatConnectorTaskStatusQueued
	if !approved {
		message := "denied by " + actor
		if trimmed := strings.TrimSpace(opinion); trimmed != "" {
			message += ": " + truncateConnectorTaskText(trimmed)
		}
		status = advancedChatConnectorTaskStatusFailed
		updates = map[string]interface{}{
			"status":        status,
			"error_message": message,
			"finished_at":   &now,
			"updated_at":    now,
		}
	}
	update := model.DB.Model(&AdvancedChatConnectorTask{}).
		Where("id = ? AND user_id = ? AND status = ?", taskID, userID, advancedChatConnectorTaskStatusPendingApproval).
		Updates(updates)
	if update.Error != nil {
		return "", update.Error
	}
	if update.RowsAffected > 0 {
		return status, nil
	}
	var task AdvancedChatConnectorTask
	if err := model.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		return "", err
	}
	return "", advancedChatConnectorTaskDecisionConflict{Status: task.Status}
}

func (api *advancedChatAPI) deleteConnectorDevice(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	deviceID := strings.TrimSpace(c.Param("id"))
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ? AND user_id = ?", deviceID, user.ID).Delete(&AdvancedChatConnectorTask{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", deviceID, user.ID).Delete(&AdvancedChatConnectorDevice{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete connector device"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Device deleted"})
}

func (api *advancedChatAPI) refreshWorkspaceSkills(c *gin.Context) {
	user, ok := currentAdvancedChatUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var input advancedChatWorkspaceSkillsRefreshInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	device, workspacePath, err := loadAdvancedChatConnectorForRun(user.ID, input.ConnectorDeviceID, input.ConnectorWorkspacePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skills, err := loadAdvancedChatWorkspaceSkillsForRun(c.Request.Context(), user.ID, device, workspacePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	responses := make([]advancedChatWorkspaceSkillResponse, 0, len(skills))
	for _, skill := range skills {
		responses = append(responses, advancedChatWorkspaceSkillResponse{
			ID:        skill.ID,
			Name:      skill.Name,
			Path:      skill.Path,
			Content:   skill.Content,
			Size:      skill.Size,
			Truncated: skill.Truncated,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"skills":          responses,
		"max_files":       advancedChatAgentSkillsMaxFiles,
		"max_file_bytes":  advancedChatAgentSkillsMaxFileBytes,
		"max_total_bytes": advancedChatAgentSkillsMaxTotalBytes,
	})
}

func (api *advancedChatAPI) connectorRegister(c *gin.Context) {
	device, ok := authenticateAdvancedChatConnector(c)
	if !ok {
		return
	}
	var input advancedChatConnectorRegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	updates := connectorDeviceUpdates(input, now)
	if err := model.DB.Model(device).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register connector"})
		return
	}
	if err := model.DB.Where("id = ? AND user_id = ?", device.ID, device.UserID).First(device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector"})
		return
	}
	c.JSON(http.StatusOK, advancedChatConnectorDeviceResponseFromModel(*device))
}

func (api *advancedChatAPI) connectorHeartbeat(c *gin.Context) {
	device, ok := authenticateAdvancedChatConnector(c)
	if !ok {
		return
	}
	var input advancedChatConnectorRegisterInput
	_ = c.ShouldBindJSON(&input)
	now := time.Now()
	updates := connectorDeviceUpdates(input, now)
	if err := model.DB.Model(device).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update connector"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "device_id": device.ID})
}

func (api *advancedChatAPI) connectorNextTask(c *gin.Context) {
	device, ok := authenticateAdvancedChatConnector(c)
	if !ok {
		return
	}
	deadline := time.Now().Add(25 * time.Second)
	for {
		task, err := claimAdvancedChatConnectorTask(device.UserID, device.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector task"})
			return
		}
		if task != nil {
			c.JSON(http.StatusOK, gin.H{"task": advancedChatConnectorTaskResponseFromModel(*task)})
			return
		}
		if time.Now().After(deadline) {
			c.JSON(http.StatusOK, gin.H{"task": nil})
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (api *advancedChatAPI) connectorTaskResult(c *gin.Context) {
	device, ok := authenticateAdvancedChatConnector(c)
	if !ok {
		return
	}
	var input advancedChatConnectorTaskResultInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := advancedChatConnectorTaskStatusCompleted
	if !input.Success {
		status = advancedChatConnectorTaskStatusFailed
	}
	now := time.Now()
	result := normalizeConnectorTaskResultText(input)
	errMessage := normalizeConnectorTaskErrorMessage(input)
	update := model.DB.Model(&AdvancedChatConnectorTask{}).
		Where("id = ? AND user_id = ? AND device_id = ?", c.Param("id"), device.UserID, device.ID).
		Updates(map[string]interface{}{
			"status":        status,
			"result":        result,
			"error_message": errMessage,
			"finished_at":   &now,
			"updated_at":    now,
		})
	if update.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save connector task result"})
		return
	}
	if update.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func authenticateAdvancedChatConnector(c *gin.Context) (*AdvancedChatConnectorDevice, bool) {
	token := strings.TrimSpace(c.GetHeader("X-Connector-Token"))
	if token == "" {
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Connector token is required"})
		return nil, false
	}
	var device AdvancedChatConnectorDevice
	if err := model.DB.Where("token_hash = ?", hashAdvancedChatConnectorToken(token)).Limit(1).Find(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to authenticate connector"})
		return nil, false
	}
	if device.ID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid connector token"})
		return nil, false
	}
	return &device, true
}

func connectorDeviceUpdates(input advancedChatConnectorRegisterInput, now time.Time) map[string]interface{} {
	name := strings.TrimSpace(input.Name)
	updates := map[string]interface{}{
		"hostname":     truncateConnectorField(input.Hostname, 120),
		"os":           truncateConnectorField(input.OS, 40),
		"arch":         truncateConnectorField(input.Arch, 40),
		"version":      truncateConnectorField(input.Version, 80),
		"status":       advancedChatConnectorDeviceStatusOnline,
		"last_seen_at": &now,
		"updated_at":   now,
	}
	if name != "" {
		updates["name"] = truncateConnectorField(name, 120)
	}
	return updates
}

func claimAdvancedChatConnectorTask(userID uint, deviceID string) (*AdvancedChatConnectorTask, error) {
	var task AdvancedChatConnectorTask
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND device_id = ? AND status = ?", userID, deviceID, advancedChatConnectorTaskStatusQueued).
			Order("created_at ASC").
			Limit(1).
			Find(&task).Error; err != nil {
			return err
		}
		if task.ID == "" {
			return nil
		}
		now := time.Now()
		update := tx.Model(&AdvancedChatConnectorTask{}).
			Where("id = ? AND status = ?", task.ID, advancedChatConnectorTaskStatusQueued).
			Updates(map[string]interface{}{
				"status":     advancedChatConnectorTaskStatusRunning,
				"started_at": &now,
				"updated_at": now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			task = AdvancedChatConnectorTask{}
			return nil
		}
		task.Status = advancedChatConnectorTaskStatusRunning
		task.StartedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	if task.ID == "" {
		return nil, nil
	}
	return &task, nil
}

func advancedChatConnectorTaskResponseFromModel(task AdvancedChatConnectorTask) advancedChatConnectorTaskResponse {
	payload := map[string]interface{}{}
	if strings.TrimSpace(task.Payload) != "" {
		_ = json.Unmarshal([]byte(task.Payload), &payload)
	}
	return advancedChatConnectorTaskResponse{
		ID:                    task.ID,
		Action:                task.Action,
		WorkspacePath:         task.WorkspacePath,
		WorkspaceUnrestricted: strings.TrimSpace(task.WorkspacePath) == "",
		Payload:               stripAdvancedChatConnectorPreviewFields(payload),
		CreatedAt:             task.CreatedAt,
	}
}

func advancedChatConnectorTaskApprovalResponseFromModel(task AdvancedChatConnectorTask, device AdvancedChatConnectorDevice) advancedChatConnectorTaskApprovalResponse {
	payload := map[string]interface{}{}
	if strings.TrimSpace(task.Payload) != "" {
		_ = json.Unmarshal([]byte(task.Payload), &payload)
	}
	return advancedChatConnectorTaskApprovalResponse{
		ID:                    task.ID,
		DeviceID:              task.DeviceID,
		DeviceName:            device.Name,
		RunID:                 task.RunID,
		Action:                task.Action,
		WorkspacePath:         task.WorkspacePath,
		WorkspaceUnrestricted: strings.TrimSpace(task.WorkspacePath) == "",
		Payload:               payload,
		CreatedAt:             task.CreatedAt,
	}
}

func advancedChatConnectorDeviceResponseFromModel(device AdvancedChatConnectorDevice) advancedChatConnectorDeviceResponse {
	online := advancedChatConnectorDeviceOnline(device)
	status := device.Status
	if !online {
		status = advancedChatConnectorDeviceStatusOffline
	}
	return advancedChatConnectorDeviceResponse{
		ID:         device.ID,
		Name:       device.Name,
		Remark:     device.Remark,
		Hostname:   device.Hostname,
		OS:         device.OS,
		Arch:       device.Arch,
		Version:    device.Version,
		Status:     status,
		Online:     online,
		LastSeenAt: device.LastSeenAt,
		CreatedAt:  device.CreatedAt,
		UpdatedAt:  device.UpdatedAt,
	}
}

func advancedChatConnectorDeviceOnline(device AdvancedChatConnectorDevice) bool {
	return device.LastSeenAt != nil &&
		device.Status == advancedChatConnectorDeviceStatusOnline &&
		time.Since(*device.LastSeenAt) <= advancedChatConnectorOnlineWindow
}

func loadAdvancedChatConnectorForRun(userID uint, deviceID string, workspacePath string) (*AdvancedChatConnectorDevice, string, error) {
	device, workspacePath, err := loadAdvancedChatConnectorForSession(userID, deviceID, workspacePath)
	if err != nil || device == nil {
		return device, workspacePath, err
	}
	if !advancedChatConnectorDeviceOnline(*device) {
		return nil, "", errors.New("connector device is offline")
	}
	return device, workspacePath, nil
}

func loadAdvancedChatConnectorForSession(userID uint, deviceID string, workspacePath string) (*AdvancedChatConnectorDevice, string, error) {
	deviceID = strings.TrimSpace(deviceID)
	workspacePath = strings.TrimSpace(workspacePath)
	if deviceID == "" && workspacePath == "" {
		return nil, "", nil
	}
	if deviceID == "" {
		return nil, "", errors.New("connector device is required")
	}
	var device AdvancedChatConnectorDevice
	if err := model.DB.Where("id = ? AND user_id = ?", deviceID, userID).First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", errors.New("connector device not found")
		}
		return nil, "", err
	}
	if len([]rune(workspacePath)) > 1000 {
		return nil, "", errors.New("workspace path is too long")
	}
	return &device, workspacePath, nil
}

func advancedChatConnectorTools(device *AdvancedChatConnectorDevice, workspacePath string, autoApprove bool, commandPrefixes []string) ([]ChatExecutorTool, map[string]advancedChatConnectorToolBinding) {
	if device == nil {
		return nil, nil
	}
	workspacePath = strings.TrimSpace(workspacePath)
	unrestricted := workspacePath == ""
	bindings := map[string]advancedChatConnectorToolBinding{}
	bind := func(name string, action string) {
		if !advancedChatAssistantConnectorActionEnabled(action) {
			return
		}
		bindings[name] = advancedChatConnectorToolBinding{
			DeviceID:        device.ID,
			DeviceName:      device.Name,
			WorkspacePath:   workspacePath,
			Action:          action,
			AutoApprove:     autoApprove,
			CommandPrefixes: normalizeConnectorCommandPrefixes(commandPrefixes),
		}
	}
	tools := []ChatExecutorTool{}
	add := func(action string, tool ChatExecutorTool) {
		if !advancedChatAssistantConnectorActionEnabled(action) {
			return
		}
		tools = append(tools, tool)
	}

	bind(advancedChatConnectorToolListFiles, "list_files")
	listDescription := "List files under the selected local workspace. Paths must be relative to the workspace root."
	if unrestricted {
		listDescription = "List files from the connected local device. Absolute paths are allowed because this message channel is configured without a workspace limit."
	}
	add("list_files", ChatExecutorTool{
		Name:        advancedChatConnectorToolListFiles,
		Description: listDescription,
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":        map[string]interface{}{"type": "string", "description": "Directory path. Relative to workspace root when workspace-limited; absolute paths are allowed when unrestricted."},
				"max_entries": map[string]interface{}{"type": "integer", "description": "Maximum entries to return.", "minimum": 1, "maximum": 500},
			},
		},
	})
	if unrestricted && strings.EqualFold(device.OS, "windows") {
		bind(advancedChatConnectorToolWindowsDrives, "list_windows_drives")
		add("list_windows_drives", ChatExecutorTool{
			Name:        advancedChatConnectorToolWindowsDrives,
			Description: "List available Windows drive roots on the connected local device. Use this before choosing an absolute path in unrestricted Windows message channels.",
			Schema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		})
	}
	bind(advancedChatConnectorToolReadFile, "read_file")
	readDescription := "Read a UTF-8 or text-like file from the selected local workspace. Paths must be relative to the workspace root."
	if unrestricted {
		readDescription = "Read a UTF-8 or text-like file from the connected local device. Absolute paths are allowed because this message channel is configured without a workspace limit."
	}
	add("read_file", ChatExecutorTool{
		Name:        advancedChatConnectorToolReadFile,
		Description: readDescription,
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]interface{}{
				"path":      map[string]interface{}{"type": "string", "description": "File path. Relative to workspace root when workspace-limited; absolute paths are allowed when unrestricted."},
				"max_bytes": map[string]interface{}{"type": "integer", "description": "Maximum bytes to return.", "minimum": 1, "maximum": 200000},
			},
		},
	})
	bind(advancedChatConnectorToolWriteFile, "write_file")
	writeDescription := "Write a file in the selected local workspace. The web frontend asks the user for approval before the connector receives write tasks."
	if unrestricted {
		writeDescription = "Write a file on the connected local device. Absolute paths are allowed because this message channel is configured without a workspace limit. This requires approval unless auto approval is enabled."
	}
	add("write_file", ChatExecutorTool{
		Name:        advancedChatConnectorToolWriteFile,
		Description: writeDescription,
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"path", "content"},
			"properties": map[string]interface{}{
				"path":        map[string]interface{}{"type": "string", "description": "File path. Relative to workspace root when workspace-limited; absolute paths are allowed when unrestricted."},
				"content":     map[string]interface{}{"type": "string", "description": "Full file content to write."},
				"overwrite":   map[string]interface{}{"type": "boolean", "description": "Whether to overwrite an existing file."},
				"create_dirs": map[string]interface{}{"type": "boolean", "description": "Whether to create parent directories."},
			},
		},
	})
	bind(advancedChatConnectorToolReplaceText, "replace_text")
	replaceDescription := "Replace one or more text blocks inside files in the selected local workspace. Use old_text/new_text for a single replacement, or replacements for multiple replacements in one tool call. The web frontend asks the user for approval before the connector receives edit tasks."
	if unrestricted {
		replaceDescription = "Replace one or more text blocks inside files on the connected local device. Absolute paths are allowed because this message channel is configured without a workspace limit. This requires approval unless auto approval is enabled."
	}
	add("replace_text", ChatExecutorTool{
		Name:        advancedChatConnectorToolReplaceText,
		Description: replaceDescription,
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":     map[string]interface{}{"type": "string", "description": "File path for a single replacement, or default path for replacements entries. Relative to workspace root when workspace-limited; absolute paths are allowed when unrestricted."},
				"old_text": map[string]interface{}{"type": "string", "description": "Exact text to replace for a single replacement."},
				"new_text": map[string]interface{}{"type": "string", "description": "Replacement text for a single replacement."},
				"replacements": map[string]interface{}{
					"type":        "array",
					"description": "Multiple replacements. Each item may include path, old_text, and new_text.",
					"items": map[string]interface{}{
						"type":     "object",
						"required": []string{"old_text", "new_text"},
						"properties": map[string]interface{}{
							"path":     map[string]interface{}{"type": "string", "description": "File path. Falls back to the top-level path."},
							"old_text": map[string]interface{}{"type": "string", "description": "Exact text to replace."},
							"new_text": map[string]interface{}{"type": "string", "description": "Replacement text."},
						},
					},
				},
			},
		},
	})
	bind(advancedChatConnectorToolRunCommand, "run_command")
	runDescription := "Run a shell command in the selected local workspace. This always requires approval unless the full command starts with a command prefix explicitly allowed in the session settings."
	if unrestricted {
		runDescription = "Run a shell command on the connected local device. It runs without a workspace limit. This always requires approval unless the full command starts with a command prefix explicitly allowed in the session settings."
	}
	add("run_command", ChatExecutorTool{
		Name:        advancedChatConnectorToolRunCommand,
		Description: runDescription,
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]interface{}{
				"command":     map[string]interface{}{"type": "string", "description": "Command line to execute in the workspace, or on the connected local device when unrestricted."},
				"timeout_sec": map[string]interface{}{"type": "integer", "description": "Maximum execution time in seconds.", "minimum": 1, "maximum": 120},
			},
		},
	})
	bind(advancedChatConnectorToolWebSearch, "web_search")
	add("web_search", ChatExecutorTool{
		Name:        advancedChatConnectorToolWebSearch,
		Description: "Search the web from the local connector and return concise result titles, URLs, and snippets. Use this when current or external information is needed. Choose a search engine when the task benefits from a specific source; otherwise use auto.",
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]interface{}{
				"query":       map[string]interface{}{"type": "string", "description": "Search query."},
				"engine":      map[string]interface{}{"type": "string", "description": "Search engine to use. Use auto unless the user or task implies a specific engine.", "enum": []string{"auto", "duckduckgo", "bing", "baidu", "google"}},
				"max_results": map[string]interface{}{"type": "integer", "description": "Maximum results to return.", "minimum": 1, "maximum": 10},
				"language":    map[string]interface{}{"type": "string", "description": "Preferred result language, such as en, zh-CN, or ja."},
				"region":      map[string]interface{}{"type": "string", "description": "Preferred result region, such as us, cn, or jp."},
				"time_range":  map[string]interface{}{"type": "string", "description": "Optional freshness filter: day, week, month, or year.", "enum": []string{"day", "week", "month", "year"}},
			},
		},
	})
	bind(advancedChatConnectorToolWebFetch, "web_fetch")
	add("web_fetch", ChatExecutorTool{
		Name:        advancedChatConnectorToolWebFetch,
		Description: "Fetch a specific HTTP or HTTPS webpage from the local connector and return readable page text or response content. Use this when the user provides a URL or after web_search returns a relevant URL.",
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"url"},
			"properties": map[string]interface{}{
				"url":       map[string]interface{}{"type": "string", "description": "HTTP or HTTPS URL to fetch."},
				"max_bytes": map[string]interface{}{"type": "integer", "description": "Maximum response bytes to return after extraction.", "minimum": 1000, "maximum": 200000},
			},
		},
	})

	return tools, bindings
}

func callAdvancedChatConnectorTool(ctx context.Context, userID uint, runID string, binding advancedChatConnectorToolBinding, arguments map[string]interface{}) (string, error) {
	task, err := createAdvancedChatConnectorTask(userID, runID, binding, arguments)
	if err != nil {
		return "", err
	}
	return waitAdvancedChatConnectorTask(ctx, task.ID, userID)
}

func createAdvancedChatConnectorTask(userID uint, runID string, binding advancedChatConnectorToolBinding, arguments map[string]interface{}) (AdvancedChatConnectorTask, error) {
	payload, err := json.Marshal(arguments)
	if err != nil {
		return AdvancedChatConnectorTask{}, err
	}
	status := advancedChatConnectorTaskStatusQueued
	if advancedChatConnectorTaskRequiresApproval(binding, arguments) {
		status = advancedChatConnectorTaskStatusPendingApproval
	}
	task := AdvancedChatConnectorTask{
		ID:            newAdvancedChatID("act"),
		UserID:        userID,
		DeviceID:      binding.DeviceID,
		RunID:         runID,
		Action:        binding.Action,
		WorkspacePath: binding.WorkspacePath,
		Payload:       string(payload),
		Status:        status,
		Result:        "",
		ErrorMessage:  "",
	}
	if err := model.DB.Create(&task).Error; err != nil {
		return AdvancedChatConnectorTask{}, err
	}
	return task, nil
}

func callAdvancedChatConnectorToolExpanded(ctx context.Context, userID uint, runID string, binding advancedChatConnectorToolBinding, arguments map[string]interface{}) (string, error) {
	calls := expandAdvancedChatConnectorToolArguments(binding, arguments)
	results := make([]string, 0, len(calls))
	for _, callArguments := range calls {
		result, err := callAdvancedChatConnectorTool(ctx, userID, runID, binding, callArguments)
		if strings.TrimSpace(result) != "" {
			results = append(results, result)
		}
		if err != nil {
			return strings.Join(results, "\n"), err
		}
	}
	return strings.Join(results, "\n"), nil
}

func loadAdvancedChatWorkspaceSkillsForRun(ctx context.Context, userID uint, device *AdvancedChatConnectorDevice, workspacePath string) ([]advancedChatWorkspaceSkill, error) {
	if device == nil {
		return []advancedChatWorkspaceSkill{}, nil
	}
	binding := advancedChatConnectorToolBinding{
		DeviceID:      device.ID,
		DeviceName:    device.Name,
		WorkspacePath: workspacePath,
		Action:        "list_agent_skills",
	}
	loadCtx, cancel := context.WithTimeout(ctx, advancedChatAgentSkillsLoadWait)
	defer cancel()
	result, err := callAdvancedChatConnectorTool(loadCtx, userID, "", binding, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to load connector agent skills: %w", err)
	}
	return parseAdvancedChatWorkspaceSkills(result)
}

func parseAdvancedChatWorkspaceSkills(raw string) ([]advancedChatWorkspaceSkill, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []advancedChatWorkspaceSkill{}, nil
	}
	var payload struct {
		Skills []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Path      string `json:"path"`
			Content   string `json:"content"`
			Size      int    `json:"size"`
			Truncated bool   `json:"truncated"`
		} `json:"skills"`
		Truncated      bool `json:"truncated"`
		TotalBytesRead int  `json:"total_bytes_read"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("failed to decode connector agent skills: %w", err)
	}
	if len(payload.Skills) > advancedChatAgentSkillsMaxFiles || payload.TotalBytesRead > advancedChatAgentSkillsMaxTotalBytes {
		return nil, errors.New("connector agent skills exceeded server limits")
	}
	result := make([]advancedChatWorkspaceSkill, 0, len(payload.Skills))
	totalBytes := 0
	seenPaths := map[string]bool{}
	for _, skill := range payload.Skills {
		path := sanitizeWorkspaceSkillPath(skill.Path)
		content := strings.TrimSpace(skill.Content)
		if path == "" || content == "" || seenPaths[path] {
			continue
		}
		size := len([]byte(content))
		if size > advancedChatAgentSkillsMaxFileBytes {
			content = truncateBytes(content, advancedChatAgentSkillsMaxFileBytes)
			size = len([]byte(content))
			skill.Truncated = true
		}
		if totalBytes+size > advancedChatAgentSkillsMaxTotalBytes {
			break
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			name = path
		}
		if len([]rune(name)) > 120 {
			name = string([]rune(name)[:120])
		}
		id := strings.TrimSpace(skill.ID)
		if len([]rune(id)) > 120 {
			id = string([]rune(id)[:120])
		}
		seenPaths[path] = true
		totalBytes += size
		result = append(result, advancedChatWorkspaceSkill{
			ID:        id,
			Name:      name,
			Path:      path,
			Content:   content,
			Size:      size,
			Truncated: skill.Truncated,
		})
	}
	return result, nil
}

func sanitizeWorkspaceSkillPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "\x00") {
		return ""
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
	}
	if !strings.HasPrefix(strings.ToLower(path), ".agents/") || !strings.HasSuffix(strings.ToLower(path), ".md") {
		return ""
	}
	if len([]rune(path)) > 500 {
		return ""
	}
	return path
}

func advancedChatConnectorToolPreviewArguments(ctx context.Context, userID uint, runID string, binding advancedChatConnectorToolBinding, arguments map[string]interface{}) map[string]interface{} {
	if binding.Action != "write_file" {
		return arguments
	}
	if !advancedChatAssistantConnectorReadFileEnabled() {
		return arguments
	}
	path, _ := arguments["path"].(string)
	if strings.TrimSpace(path) == "" {
		return arguments
	}
	previewArguments := cloneAdvancedChatConnectorArguments(arguments)
	readBinding := binding
	readBinding.Action = "read_file"
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	content, err := callAdvancedChatConnectorTool(readCtx, userID, runID, readBinding, map[string]interface{}{
		"path":      path,
		"max_bytes": 200000,
	})
	if err != nil {
		previewArguments[advancedChatConnectorPreviewOldContentAvailable] = false
		return previewArguments
	}
	previewArguments[advancedChatConnectorPreviewOldContent] = content
	previewArguments[advancedChatConnectorPreviewOldContentAvailable] = true
	return previewArguments
}

func advancedChatConnectorArgumentsWithToolCallID(arguments map[string]interface{}, toolCallID string) map[string]interface{} {
	if strings.TrimSpace(toolCallID) == "" {
		return arguments
	}
	previewArguments := cloneAdvancedChatConnectorArguments(arguments)
	previewArguments[advancedChatConnectorPreviewToolCallID] = toolCallID
	return previewArguments
}

func advancedChatConnectorArgumentsWithTaskID(arguments map[string]interface{}, taskID string) map[string]interface{} {
	if strings.TrimSpace(taskID) == "" {
		return arguments
	}
	previewArguments := cloneAdvancedChatConnectorArguments(arguments)
	previewArguments[advancedChatConnectorTaskID] = taskID
	return previewArguments
}

func cloneAdvancedChatConnectorArguments(arguments map[string]interface{}) map[string]interface{} {
	clone := make(map[string]interface{}, len(arguments))
	for key, value := range arguments {
		clone[key] = value
	}
	return clone
}

func stripAdvancedChatConnectorPreviewFields(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return payload
	}
	sanitized := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if key == advancedChatConnectorPreviewOldContent || key == advancedChatConnectorPreviewOldContentAvailable || key == advancedChatConnectorPreviewToolCallID || key == advancedChatConnectorTaskID {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

func expandAdvancedChatConnectorToolArguments(binding advancedChatConnectorToolBinding, arguments map[string]interface{}) []map[string]interface{} {
	if binding.Action != "replace_text" {
		return []map[string]interface{}{arguments}
	}
	raw, ok := arguments["replacements"].([]interface{})
	if !ok || len(raw) == 0 {
		return []map[string]interface{}{arguments}
	}
	calls := make([]map[string]interface{}, 0, len(raw))
	defaultPath, _ := arguments["path"].(string)
	for _, item := range raw {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		oldText, _ := row["old_text"].(string)
		newText, _ := row["new_text"].(string)
		path, _ := row["path"].(string)
		if strings.TrimSpace(path) == "" {
			path = defaultPath
		}
		if oldText == "" && newText == "" {
			continue
		}
		calls = append(calls, map[string]interface{}{
			"path":     path,
			"old_text": oldText,
			"new_text": newText,
		})
	}
	if len(calls) == 0 {
		return []map[string]interface{}{arguments}
	}
	return calls
}

func advancedChatConnectorTaskRequiresApproval(binding advancedChatConnectorToolBinding, arguments map[string]interface{}) bool {
	switch binding.Action {
	case "list_files", "read_file", "web_search", "web_fetch", "list_agent_skills", "list_windows_drives":
		return false
	case "run_command":
		command, _ := arguments["command"].(string)
		return !connectorCommandAutoApproved(command, binding.CommandPrefixes)
	case "write_file", "replace_text":
		return !binding.AutoApprove
	default:
		return true
	}
}

func connectorCommandAutoApproved(command string, prefixes []string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	for _, prefix := range normalizeConnectorCommandPrefixes(prefixes) {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func waitAdvancedChatConnectorTask(ctx context.Context, taskID string, userID uint) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, advancedChatConnectorTaskWait)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var task AdvancedChatConnectorTask
		if err := model.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
			return "", err
		}
		switch task.Status {
		case advancedChatConnectorTaskStatusCompleted:
			return task.Result, nil
		case advancedChatConnectorTaskStatusFailed:
			if strings.TrimSpace(task.ErrorMessage) == "" {
				return task.Result, errors.New("connector task failed")
			}
			return task.Result, errors.New(task.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			now := time.Now()
			_ = model.DB.Model(&AdvancedChatConnectorTask{}).
				Where("id = ? AND user_id = ?", taskID, userID).
				Updates(map[string]interface{}{
					"status":        advancedChatConnectorTaskStatusFailed,
					"error_message": "connector task timed out",
					"finished_at":   &now,
					"updated_at":    now,
				}).Error
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func advancedChatConnectorSystemPrompt(device *AdvancedChatConnectorDevice, workspacePath string) string {
	if device == nil {
		return ""
	}
	workspacePath = strings.TrimSpace(workspacePath)
	osName := strings.TrimSpace(device.OS)
	if osName == "" {
		osName = "unknown"
	}
	archName := strings.TrimSpace(device.Arch)
	if archName == "" {
		archName = "unknown"
	}
	if workspacePath == "" {
		windowsPathHint := ""
		if strings.EqualFold(device.OS, "windows") {
			windowsPathHint = "\nThe connected device is Windows. Use workspace_list_windows_drives to discover available drive roots before selecting absolute paths when the drive is not already known."
		}
		return fmt.Sprintf(`A local device connector is available without a workspace limit.
Device: %s
Environment: OS=%s Arch=%s
Use workspace tools when you need to inspect or edit files on this device.
Absolute paths are allowed. Ask for or infer concrete paths before reading or changing files.%s
Read-only workspace tools, web search, web fetch, and Windows drive listing do not require approval. The user will be asked in the message channel to reply yes before file operations that change files are sent to the local connector, unless the message channel enables automatic approval. Commands always require approval unless the command starts with a prefix explicitly allowed in the message channel settings.`, device.Name, osName, archName, windowsPathHint)
	}
	return fmt.Sprintf(`A local workspace connector is available.
Device: %s
Environment: OS=%s Arch=%s
Workspace: %s
Use workspace tools when you need to inspect or edit files in this workspace.
Use only relative paths in workspace tool arguments.
Read-only workspace tools, web search, and web fetch do not require approval. The web frontend will ask the user for approval before file operations that change files are sent to the local connector, unless the session enables automatic approval. Commands always require approval unless the command starts with a prefix explicitly allowed in the session settings.`, device.Name, osName, archName, workspacePath)
}

func normalizeConnectorCommandPrefixes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func newAdvancedChatConnectorToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data))
	return "wpc_" + encoded, nil
}

func hashAdvancedChatConnectorToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func truncateConnectorField(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func truncateConnectorTaskText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 200000 {
		return string(runes[:200000]) + "\n...(truncated)"
	}
	return value
}

func truncateBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	total := 0
	var builder strings.Builder
	for _, r := range value {
		size := len(string(r))
		if total+size > maxBytes {
			break
		}
		builder.WriteRune(r)
		total += size
	}
	return builder.String()
}

func normalizeConnectorTaskResultText(input advancedChatConnectorTaskResultInput) string {
	sections := make([]string, 0, 4)
	if text := strings.TrimSpace(input.Result); text != "" {
		sections = append(sections, text)
	}
	if text := strings.TrimSpace(input.Output); text != "" && !connectorTaskSectionAlreadyIncluded(sections, text) {
		sections = append(sections, text)
	}
	if text := strings.TrimSpace(input.Stdout); text != "" && !connectorTaskSectionAlreadyIncluded(sections, text) {
		sections = append(sections, "stdout:\n"+text)
	}
	if text := strings.TrimSpace(input.Stderr); text != "" && !connectorTaskSectionAlreadyIncluded(sections, text) {
		sections = append(sections, "stderr:\n"+text)
	}
	return truncateConnectorTaskText(strings.Join(sections, "\n\n"))
}

func normalizeConnectorTaskErrorMessage(input advancedChatConnectorTaskResultInput) string {
	message := strings.TrimSpace(input.Error)
	if input.ExitCode != nil {
		exitMessage := fmt.Sprintf("exit code %d", *input.ExitCode)
		if message == "" {
			message = exitMessage
		} else if !strings.Contains(strings.ToLower(message), strings.ToLower(exitMessage)) {
			message += "; " + exitMessage
		}
	}
	return truncateConnectorTaskText(message)
}

func connectorTaskSectionAlreadyIncluded(sections []string, text string) bool {
	for _, section := range sections {
		if strings.TrimSpace(section) == text {
			return true
		}
	}
	return false
}
