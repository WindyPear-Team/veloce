package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type personalCompanyRuntimeBindingInput struct {
	AdvancedChatAgentID      string   `json:"advanced_chat_agent_id"`
	ConnectorDeviceID        string   `json:"connector_device_id"`
	ConnectorWorkspacePath   string   `json:"connector_workspace_path"`
	ConnectorCommandPrefixes []string `json:"connector_command_prefixes"`
}

type personalCompanyRuntimePolicy struct {
	ConnectorDeviceID        string   `json:"connector_device_id,omitempty"`
	ConnectorWorkspacePath   string   `json:"connector_workspace_path,omitempty"`
	ConnectorCommandPrefixes []string `json:"connector_command_prefixes,omitempty"`
}

type personalCompanyStudioRuntimeInput struct {
	ConnectorDeviceID        string   `json:"connector_device_id"`
	ConnectorWorkspacePath   string   `json:"connector_workspace_path"`
	ConnectorCommandPrefixes []string `json:"connector_command_prefixes"`
}

type personalCompanyConnectorApprovalDecisionInput struct {
	Approved bool `json:"approved"`
}

// updateStudioRuntime binds a connector to the Studio itself. Studio members
// keep their own agents, models, skills, and MCP configuration.
func (api *personalCompanyAPI) updateStudioRuntime(c *gin.Context) {
	ctx, ok := api.personalCompanyContext(c)
	if !ok {
		return
	}
	company, err := loadPersonalCompany(ctx.userID, ctx.agentGroupID)
	if writePersonalCompanyLoadError(c, err) {
		return
	}
	var input personalCompanyStudioRuntimeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deviceID := strings.TrimSpace(input.ConnectorDeviceID)
	workspacePath := strings.TrimSpace(input.ConnectorWorkspacePath)
	if deviceID != "" || workspacePath != "" {
		if _, _, err := loadAdvancedChatConnectorForRun(ctx.userID, deviceID, workspacePath); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	prefixes, err := json.Marshal(normalizeConnectorCommandPrefixes(input.ConnectorCommandPrefixes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid connector command prefixes"})
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.PersonalCompany{}).Where("id = ? AND owner_user_id = ?", company.ID, ctx.userID).Updates(map[string]interface{}{
			"connector_device_id":        deviceID,
			"connector_workspace_path":   workspacePath,
			"connector_command_prefixes": string(prefixes),
		}).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, company.ID, nil, "owner", ctx.userID, "studio.runtime_configured", fmt.Sprintf(`{"connector_device_id":%q,"workspace_path":%q}`, deviceID, workspacePath))
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure Studio runtime"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connector_device_id": deviceID, "connector_workspace_path": workspacePath})
}

func (api *personalCompanyAPI) bindEmployeeRuntime(c *gin.Context) {
	ctx, ok := api.personalCompanyContext(c)
	if !ok {
		return
	}
	company, err := loadPersonalCompany(ctx.userID, ctx.agentGroupID)
	if writePersonalCompanyLoadError(c, err) {
		return
	}
	employee, err := loadPersonalCompanyEmployee(company.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Employee not found"})
		return
	}
	var input personalCompanyRuntimeBindingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := loadAdvancedChatAgent(ctx.userID, input.AdvancedChatAgentID); err != nil || strings.TrimSpace(input.AdvancedChatAgentID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "An existing Advanced Chat agent is required"})
		return
	}
	if strings.TrimSpace(input.ConnectorDeviceID) != "" || strings.TrimSpace(input.ConnectorWorkspacePath) != "" {
		if _, _, err := loadAdvancedChatConnectorForRun(ctx.userID, input.ConnectorDeviceID, input.ConnectorWorkspacePath); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	policy, _ := json.Marshal(personalCompanyRuntimePolicy{ConnectorDeviceID: strings.TrimSpace(input.ConnectorDeviceID), ConnectorWorkspacePath: strings.TrimSpace(input.ConnectorWorkspacePath), ConnectorCommandPrefixes: normalizeConnectorCommandPrefixes(input.ConnectorCommandPrefixes)})
	version := employee.Version + 1
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.CompanyEmployeeVersion{PersonalCompanyID: company.ID, EmployeeID: employee.ID, Version: version, PromptProfile: "advanced_chat_agent:" + strings.TrimSpace(input.AdvancedChatAgentID), ModelPolicy: string(policy), ToolGrants: employee.AllowedTools, DataScope: employee.DataScope, SkillScope: "[]", CreatedByUserID: ctx.userID}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PersonalCompanyEmployee{}).Where("id = ? AND version = ?", employee.ID, employee.Version).Updates(map[string]interface{}{"advanced_chat_agent_id": strings.TrimSpace(input.AdvancedChatAgentID), "version": version}).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, company.ID, nil, "owner", ctx.userID, "employee.runtime_bound", fmt.Sprintf(`{"employee_id":%d,"version":%d}`, employee.ID, version))
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to bind employee runtime"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"employee_id": employee.ID, "version": version})
}

func (api *personalCompanyAPI) runWorkItem(c *gin.Context) {
	ctx, ok := api.personalCompanyContext(c)
	if !ok {
		return
	}
	company, err := loadPersonalCompany(ctx.userID, ctx.agentGroupID)
	if writePersonalCompanyLoadError(c, err) {
		return
	}
	work, err := loadPersonalCompanyWorkItem(company.ID, ctx.userID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Work item not found"})
		return
	}
	if work.Status == model.CompanyWorkStatusPlanned || work.Status == model.CompanyWorkStatusAuthorized {
		if err := QueuePersonalCompanyWorkItem(model.DB, company, work.ID, ctx.userID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Work item is not ready to run"})
			return
		}
	}
	attempt, runID, err := startPersonalCompanyWorkRun(company, ctx.userID, work.ID)
	if err != nil {
		writePersonalCompanyRuntimeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"work_attempt": attempt, "advanced_chat_run_id": runID})
}

// getWorkItemInternalSession exposes the immutable Studio conversation that
// performed a work attempt. Owners can inspect it without editing the run.
func (api *personalCompanyAPI) getWorkItemInternalSession(c *gin.Context) {
	ctx, ok := api.personalCompanyContext(c)
	if !ok {
		return
	}
	company, err := loadPersonalCompany(ctx.userID, ctx.agentGroupID)
	if writePersonalCompanyLoadError(c, err) {
		return
	}
	workItem, err := loadPersonalCompanyWorkItem(company.ID, ctx.userID, c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Work item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load work item"})
		return
	}
	var attempt model.CompanyWorkAttempt
	if err := model.DB.Where("work_item_id = ? AND advanced_chat_run_id <> ''", workItem.ID).Order("attempt_number DESC").First(&attempt).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No internal Studio session exists for this work item"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load work attempt"})
		return
	}
	var run AdvancedChatRun
	if err := model.DB.Where("id = ? AND user_id = ?", attempt.AdvancedChatRunID, ctx.userID).First(&run).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Internal Studio run not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load internal Studio run"})
		return
	}
	session, err := advancedChatSessionResponseFor(ctx.userID, run.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load internal Studio session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"work_item_id": workItem.ID, "attempt": attempt, "run": advancedChatRunResponseFromModel(run), "session": session, "readonly": true})
}

func (api *personalCompanyAPI) decideStudioConnectorApproval(c *gin.Context) {
	ctx, ok := api.personalCompanyContext(c)
	if !ok {
		return
	}
	company, err := loadPersonalCompany(ctx.userID, ctx.agentGroupID)
	if writePersonalCompanyLoadError(c, err) {
		return
	}
	var input personalCompanyConnectorApprovalDecisionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID := strings.TrimSpace(c.Param("id"))
	var task AdvancedChatConnectorTask
	if err := model.DB.Where("id = ? AND user_id = ?", taskID, ctx.userID).First(&task).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Connector approval not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load connector approval"})
		return
	}
	var attempt model.CompanyWorkAttempt
	if err := model.DB.Joins("JOIN company_work_items ON company_work_items.id = company_work_attempts.work_item_id").Where("company_work_attempts.advanced_chat_run_id = ? AND company_work_items.personal_company_id = ?", task.RunID, company.ID).First(&attempt).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Connector approval is not owned by this Studio"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify connector approval"})
		return
	}
	status, err := decideAdvancedChatConnectorTask(ctx.userID, task.ID, input.Approved, "owner", "Studio owner decision")
	if err != nil {
		var conflict advancedChatConnectorTaskDecisionConflict
		if errors.As(err, &conflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "Connector approval has already been decided", "status": conflict.Status})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decide connector approval"})
		return
	}
	_ = createPersonalCompanyAuditEvent(model.DB, company.ID, &attempt.WorkItemID, "owner", ctx.userID, "connector_approval."+status, fmt.Sprintf(`{"connector_task_id":%q}`, task.ID))
	c.JSON(http.StatusOK, gin.H{"status": status})
}

func startPersonalCompanyWorkRun(company model.PersonalCompany, userID, workItemID uint) (model.CompanyWorkAttempt, string, error) {
	var currentCompany model.PersonalCompany
	if err := model.DB.Select("state", "balance_floor").Where("id = ? AND owner_user_id = ?", company.ID, userID).First(&currentCompany).Error; err != nil {
		return model.CompanyWorkAttempt{}, "", err
	}
	if currentCompany.State != model.PersonalCompanyStateOperating {
		return model.CompanyWorkAttempt{}, "", errors.New("studio operations are paused")
	}
	company.BalanceFloor = currentCompany.BalanceFloor
	var owner model.User
	if err := model.DB.Select("id", "balance").Where("id = ?", userID).First(&owner).Error; err != nil {
		return model.CompanyWorkAttempt{}, "", err
	}
	if owner.Balance.LessThanOrEqual(company.BalanceFloor) {
		_ = pausePersonalCompanyForBalance(company, workItemID, owner.Balance)
		return model.CompanyWorkAttempt{}, "", errors.New("studio balance is below its operating floor")
	}
	var work model.CompanyWorkItem
	if err := model.DB.First(&work, workItemID).Error; err != nil {
		return model.CompanyWorkAttempt{}, "", err
	}
	useStudio := strings.TrimSpace(company.AgentGroupID) != ""
	if !useStudio && work.AssignedEmployeeID == nil {
		var fallback model.PersonalCompanyEmployee
		if err := model.DB.Where("personal_company_id = ? AND status IN ? AND advanced_chat_agent_id <> ''", company.ID, []string{model.PersonalCompanyEmployeeProbation, model.PersonalCompanyEmployeeActive}).Order("created_at ASC").First(&fallback).Error; err != nil {
			return model.CompanyWorkAttempt{}, "", errors.New("work item needs an assigned employee with an Advanced Chat agent")
		}
		if err := model.DB.Model(&model.CompanyWorkItem{}).Where("id = ? AND assigned_employee_id IS NULL", work.ID).Update("assigned_employee_id", fallback.ID).Error; err != nil {
			return model.CompanyWorkAttempt{}, "", err
		}
		work.AssignedEmployeeID = &fallback.ID
	}
	policy := personalCompanyRuntimePolicy{}
	modelName, agentID, mode, agentGroupID := "", "", advancedChatModeAssistant, ""
	if useStudio {
		if _, err := readAdvancedChatAgentGroup(context.Background(), userID, nil, company.AgentGroupID); err != nil {
			return model.CompanyWorkAttempt{}, "", errors.New("bound Agent Studio is unavailable")
		}
		_ = json.Unmarshal([]byte(company.ConnectorCommandPrefixes), &policy.ConnectorCommandPrefixes)
		policy.ConnectorDeviceID, policy.ConnectorWorkspacePath = company.ConnectorDeviceID, company.ConnectorWorkspacePath
		mode, agentGroupID = advancedChatModeAgentGroup, company.AgentGroupID
	} else {
		var employee model.PersonalCompanyEmployee
		if err := model.DB.Where("id = ? AND personal_company_id = ? AND status IN ?", *work.AssignedEmployeeID, company.ID, []string{model.PersonalCompanyEmployeeProbation, model.PersonalCompanyEmployeeActive}).First(&employee).Error; err != nil {
			return model.CompanyWorkAttempt{}, "", errors.New("assigned employee is unavailable")
		}
		if strings.TrimSpace(employee.AdvancedChatAgentID) == "" {
			return model.CompanyWorkAttempt{}, "", errors.New("assigned employee has no Advanced Chat agent")
		}
		var version model.CompanyEmployeeVersion
		if err := model.DB.Where("employee_id = ? AND personal_company_id = ? AND version = ?", employee.ID, company.ID, employee.Version).First(&version).Error; err != nil {
			return model.CompanyWorkAttempt{}, "", errors.New("assigned employee version is unavailable")
		}
		_ = json.Unmarshal([]byte(version.ModelPolicy), &policy)
		agent, err := loadAdvancedChatAgent(userID, employee.AdvancedChatAgentID)
		if err != nil || agent == nil || strings.TrimSpace(agent.DefaultModel) == "" {
			return model.CompanyWorkAttempt{}, "", errors.New("assigned Advanced Chat agent has no model")
		}
		modelName, agentID = agent.DefaultModel, employee.AdvancedChatAgentID
	}
	prompt := fmt.Sprintf("You are completing a governed Personal Company work item.\nTitle: %s\nDescription: %s\nDefinition of done: %s\nInput snapshot: %s\n\nUse only the provided tools. Never perform external side effects without the connector's manual approval. Return a concise result with evidence, unresolved risks, and next steps.", work.Title, work.Description, work.DefinitionOfDone, work.InputSnapshot)
	input := advancedChatCompletionInput{SessionID: newAdvancedChatID("pcw"), Title: "Personal Company: " + work.Title, ModelName: modelName, Messages: []advancedChatCompletionMessage{{Role: "user", Content: prompt}}, Mode: mode, AgentID: agentID, AgentGroupID: agentGroupID, ConnectorDeviceID: policy.ConnectorDeviceID, ConnectorWorkspacePath: policy.ConnectorWorkspacePath, ConnectorApprovalMode: advancedChatConnectorApprovalManual, ConnectorCommandPrefixes: policy.ConnectorCommandPrefixes, ChargeBalance: true}
	prepared, _, message, err := prepareAdvancedChatAssistantRun(context.Background(), userID, input, input.Messages, modelName)
	if err != nil {
		return model.CompanyWorkAttempt{}, "", errors.New(message)
	}
	attempt, err := leasePersonalCompanyWorkItem(model.DB, company.ID, workItemID, time.Now().UTC(), 10*time.Minute)
	if err != nil {
		return model.CompanyWorkAttempt{}, "", err
	}
	_, run, _, message, err := createAdvancedChatAssistantRun(userID, prepared)
	if err != nil {
		_ = releasePersonalCompanyWorkLease(model.DB, company.ID, workItemID, attempt.ID, "Advanced Chat run could not be created")
		return attempt, "", errors.New(message)
	}
	if err := model.DB.Model(&model.CompanyWorkAttempt{}).Where("id = ?", attempt.ID).Update("advanced_chat_run_id", run.ID).Error; err != nil {
		_ = releasePersonalCompanyWorkLease(model.DB, company.ID, workItemID, attempt.ID, "Advanced Chat run link could not be saved")
		return attempt, "", err
	}
	attempt.AdvancedChatRunID = run.ID
	go runPersonalCompanyAdvancedChatWork(company.ID, work.ID, attempt.ID, userID, run.ID, prepared)
	return attempt, run.ID, nil
}

func pausePersonalCompanyForBalance(company model.PersonalCompany, workItemID uint, balance decimal.Decimal) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&model.PersonalCompany{}).Where("id = ?", company.ID).Updates(map[string]interface{}{"state": model.PersonalCompanyStateAttentionRequired, "paused_at": now}).Error; err != nil {
			return err
		}
		payload := fmt.Sprintf(`{"balance":%q,"floor":%q}`, balance.String(), company.BalanceFloor.String())
		outbox := model.CompanyOutboxEvent{PersonalCompanyID: company.ID, EventKey: "balance:floor_reached", EventType: "balance.floor_reached", Payload: payload, Status: model.CompanyOutboxStatusPending}
		if err := tx.Where("personal_company_id = ? AND event_key = ?", company.ID, outbox.EventKey).FirstOrCreate(&outbox).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, company.ID, &workItemID, "system", 0, "balance.floor_reached", payload)
	})
}

func releasePersonalCompanyWorkLease(db *gorm.DB, companyID, workItemID, attemptID uint, reason string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&model.CompanyWorkAttempt{}).Where("id = ? AND status = ?", attemptID, model.CompanyWorkStatusExecuting).Updates(map[string]interface{}{"status": model.CompanyWorkStatusRetryableFailure, "finished_at": now, "result_summary": truncatePersonalCompanyText(reason, 1000)}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CompanyWorkItem{}).Where("id = ? AND status = ?", workItemID, model.CompanyWorkStatusExecuting).Update("status", model.CompanyWorkStatusQueued).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, companyID, &workItemID, "worker", 0, "work_attempt.start_failed", fmt.Sprintf(`{"attempt_id":%d}`, attemptID))
	})
}

func leasePersonalCompanyWorkItem(db *gorm.DB, companyID, workItemID uint, now time.Time, leaseDuration time.Duration) (model.CompanyWorkAttempt, error) {
	var attempt model.CompanyWorkAttempt
	err := db.Transaction(func(tx *gorm.DB) error {
		var work model.CompanyWorkItem
		if err := tx.Where("id = ? AND personal_company_id = ? AND status = ?", workItemID, companyID, model.CompanyWorkStatusQueued).First(&work).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPersonalCompanyNoQueuedWork
			}
			return err
		}
		if result := tx.Model(&model.CompanyWorkItem{}).Where("id = ? AND status = ?", work.ID, model.CompanyWorkStatusQueued).Update("status", model.CompanyWorkStatusExecuting); result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				return result.Error
			}
			return ErrPersonalCompanyNoQueuedWork
		}
		var count int64
		if err := tx.Model(&model.CompanyWorkAttempt{}).Where("work_item_id = ?", work.ID).Count(&count).Error; err != nil {
			return err
		}
		expiresAt := now.Add(leaseDuration)
		attempt = model.CompanyWorkAttempt{WorkItemID: work.ID, AttemptNumber: int(count) + 1, Status: model.CompanyWorkStatusExecuting, LeaseToken: newPersonalCompanyID("lease"), LeaseExpiresAt: &expiresAt, StartedAt: &now, InputSnapshot: work.InputSnapshot}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, companyID, &work.ID, "worker", 0, "work_attempt.leased", fmt.Sprintf(`{"attempt_id":%d}`, attempt.ID))
	})
	return attempt, err
}

func runPersonalCompanyAdvancedChatWork(companyID, workItemID, attemptID, userID uint, runID string, prepared preparedAdvancedChatAssistantRun) {
	runAdvancedChatAssistantCompletion(runID, userID, prepared)
	var run AdvancedChatRun
	if err := model.DB.Where("id = ? AND user_id = ?", runID, userID).First(&run).Error; err != nil {
		return
	}
	status := model.CompanyWorkStatusAwaitingReview
	if run.Status != advancedChatRunStatusCompleted {
		status = model.CompanyWorkStatusBlocked
	}
	_ = model.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{"status": status, "finished_at": time.Now().UTC(), "result_summary": run.ErrorMessage, "cost": run.Cost}
		var result AdvancedChatMessage
		if run.Status == advancedChatRunStatusCompleted {
			if err := tx.Where("id = ? AND user_id = ?", run.AssistantMessageID, userID).First(&result).Error; err != nil {
				return err
			}
			updates["result_summary"] = truncatePersonalCompanyText(result.Content, 4000)
		}
		if err := tx.Model(&model.CompanyWorkAttempt{}).Where("id = ?", attemptID).Updates(updates).Error; err != nil {
			return err
		}
		var workItem model.CompanyWorkItem
		if err := tx.Where("id = ?", workItemID).First(&workItem).Error; err != nil {
			return err
		}
		workUpdates := map[string]interface{}{"status": status, "consumed_cost": run.Cost}
		if workItem.ReservedCost.GreaterThan(decimal.Zero) {
			workUpdates["reserved_cost"] = decimal.Zero
		}
		if err := tx.Model(&model.CompanyWorkItem{}).Where("id = ? AND status = ?", workItemID, model.CompanyWorkStatusExecuting).Updates(workUpdates).Error; err != nil {
			return err
		}
		if run.Status == advancedChatRunStatusCompleted && strings.TrimSpace(result.Content) != "" {
			if err := tx.Create(&model.CompanyArtifact{WorkItemID: workItemID, WorkAttemptID: &attemptID, Kind: "advanced_chat_result", URI: "advanced-chat://runs/" + runID, ContentHash: personalCompanyParametersHash(model.CompanyWorkItem{InputSnapshot: result.Content}), Source: "Advanced Chat run output", AcceptanceState: "pending"}).Error; err != nil {
				return err
			}
		}
		if run.Cost.GreaterThan(decimal.Zero) {
			if err := tx.Create(&model.CompanyBudgetLedger{PersonalCompanyID: companyID, WorkItemID: &workItemID, WorkAttemptID: &attemptID, EntryType: "consumption", Amount: run.Cost, ReferenceType: "advanced_chat_run", ReferenceID: runID, CreatedByUserID: userID}).Error; err != nil {
				return err
			}
		}
		var owner model.User
		if err := tx.Select("balance").Where("id = ?", userID).First(&owner).Error; err != nil {
			return err
		}
		var company model.PersonalCompany
		if err := tx.Select("balance_floor").Where("id = ?", companyID).First(&company).Error; err != nil {
			return err
		}
		if owner.Balance.LessThanOrEqual(company.BalanceFloor) {
			if err := tx.Model(&model.PersonalCompany{}).Where("id = ?", companyID).Updates(map[string]interface{}{"state": model.PersonalCompanyStateAttentionRequired, "paused_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
			payload := fmt.Sprintf(`{"balance":%q,"floor":%q}`, owner.Balance.String(), company.BalanceFloor.String())
			outbox := model.CompanyOutboxEvent{PersonalCompanyID: companyID, EventKey: "balance:floor_reached", EventType: "balance.floor_reached", Payload: payload, Status: model.CompanyOutboxStatusPending}
			if err := tx.Where("personal_company_id = ? AND event_key = ?", companyID, outbox.EventKey).FirstOrCreate(&outbox).Error; err != nil {
				return err
			}
			if err := createPersonalCompanyAuditEvent(tx, companyID, &workItemID, "system", 0, "balance.floor_reached", payload); err != nil {
				return err
			}
		}
		if workItem.ReservedCost.GreaterThan(decimal.Zero) {
			if err := tx.Create(&model.CompanyBudgetLedger{PersonalCompanyID: companyID, WorkItemID: &workItemID, WorkAttemptID: &attemptID, EntryType: "release", Amount: workItem.ReservedCost.Neg(), ReferenceType: "work_completion", ReferenceID: runID, CreatedByUserID: userID}).Error; err != nil {
				return err
			}
		}
		return createPersonalCompanyAuditEvent(tx, companyID, &workItemID, "worker", 0, "work_attempt.completed", fmt.Sprintf(`{"attempt_id":%d,"run_id":%q,"status":%q}`, attemptID, runID, status))
	})
}

func writePersonalCompanyRuntimeError(c *gin.Context, err error) {
	if errors.Is(err, ErrPersonalCompanyNoQueuedWork) {
		c.JSON(http.StatusConflict, gin.H{"error": "No queued work is available"})
		return
	}
	c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
}
