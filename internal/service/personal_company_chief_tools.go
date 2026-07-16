package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	personalCompanyChiefCreateWorkToolName = "studio_create_work"
	personalCompanyChiefListWorkToolName   = "studio_list_work"
)

func init() {
	RegisterAdvancedChatRuntimeExtensionHook(personalCompanyChiefRuntimeExtension)
	RegisterAdvancedChatToolHandler(personalCompanyChiefCreateWorkToolName, handlePersonalCompanyChiefCreateWork)
	RegisterAdvancedChatToolHandler(personalCompanyChiefListWorkToolName, handlePersonalCompanyChiefListWork)
}

// personalCompanyChiefRuntimeExtension gives the Chief a bounded way to turn a
// user's request into a durable Studio work item. It is never exposed outside
// an enabled Studio operation.
func personalCompanyChiefRuntimeExtension(_ context.Context, input AdvancedChatRuntimeContext) (AdvancedChatRuntimeExtension, error) {
	if input.Mode != advancedChatModeAgentGroup || strings.TrimSpace(input.AgentGroupID) == "" {
		return AdvancedChatRuntimeExtension{}, nil
	}
	if personalCompanyChiefRunIsInternal(input.RunID) {
		return AdvancedChatRuntimeExtension{}, nil
	}
	company, err := loadPersonalCompany(input.UserID, input.AgentGroupID)
	if errors.Is(err, gorm.ErrRecordNotFound) || company.State != model.PersonalCompanyStateOperating {
		return AdvancedChatRuntimeExtension{}, nil
	}
	if err != nil {
		return AdvancedChatRuntimeExtension{}, err
	}
	return AdvancedChatRuntimeExtension{
		SystemPrompt: "This Studio has operations enabled. You are the Chief and receive every user message. A goal is a long-lived outcome or direction; a work item is one concrete, bounded, verifiable delivery. When the user asks the Studio to do concrete work, clarify the deliverable and use studio_create_work after you have enough detail. For questions about progress, active work, or pending approvals, call studio_list_work before answering. Do not create work for mere discussion, status questions, or ambiguous requests.",
		Tools: []ChatExecutorTool{
			{
				Name:        personalCompanyChiefCreateWorkToolName,
				Description: "Create and start a concrete Studio work item. Use after you have enough detail to define its outcome and acceptance criteria.",
				Schema: map[string]interface{}{
					"type":     "object",
					"required": []string{"title", "definition_of_done"},
					"properties": map[string]interface{}{
						"title":              map[string]interface{}{"type": "string", "description": "Short concrete delivery title."},
						"description":        map[string]interface{}{"type": "string", "description": "Context, constraints, and requested outcome."},
						"definition_of_done": map[string]interface{}{"type": "string", "description": "Verifiable acceptance criteria and evidence."},
						"priority":           map[string]interface{}{"type": "integer", "description": "Optional priority; higher is more urgent."},
					},
				},
			},
			{
				Name:        personalCompanyChiefListWorkToolName,
				Description: "Read the Studio's recent work status and pending owner approvals before reporting operational progress.",
				Schema:      map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			},
		},
	}, nil
}

func handlePersonalCompanyChiefCreateWork(_ context.Context, input AdvancedChatToolCallInput) (string, error) {
	if personalCompanyChiefRunIsInternal(input.RunID) {
		return "", errors.New("internal Studio work sessions cannot create additional work items")
	}
	if input.Mode != advancedChatModeAgentGroup || strings.TrimSpace(input.AgentGroupID) == "" {
		return "", errors.New("Studio Chief context is required")
	}
	company, err := loadPersonalCompany(input.UserID, input.AgentGroupID)
	if err != nil {
		return "", err
	}
	if company.State != model.PersonalCompanyStateOperating {
		return "", errors.New("Studio operations are paused")
	}
	title := truncatePersonalCompanyText(stringFromMap(input.Arguments, "title"), 200)
	description := strings.TrimSpace(stringFromMap(input.Arguments, "description"))
	definition := strings.TrimSpace(stringFromMap(input.Arguments, "definition_of_done"))
	if title == "" || definition == "" {
		return "", errors.New("title and definition_of_done are required")
	}
	priority := intFromStatusPayload(gin.H(input.Arguments), "priority")
	workItem := model.CompanyWorkItem{
		PersonalCompanyID: company.ID,
		OwnerUserID:       input.UserID,
		Title:             title,
		Description:       description,
		DefinitionOfDone:  truncatePersonalCompanyText(definition, 4000),
		Status:            model.CompanyWorkStatusQueued,
		Priority:          priority,
		RiskLevel:         "r0",
		IdempotencyKey:    newPersonalCompanyID("chief-work"),
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&workItem).Error; err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]uint{"work_item_id": workItem.ID})
		if err := tx.Create(&model.CompanySignal{PersonalCompanyID: company.ID, OwnerUserID: input.UserID, Source: "chief_chat", DeduplicationKey: "chief:" + workItem.IdempotencyKey, Payload: string(payload), Status: model.CompanySignalStatusTriaged, WorkItemID: &workItem.ID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.CompanyOutboxEvent{PersonalCompanyID: company.ID, EventKey: "chief:" + workItem.IdempotencyKey, EventType: "work_item.queued", Payload: string(payload), Status: model.CompanyOutboxStatusPending}).Error; err != nil {
			return err
		}
		return createPersonalCompanyAuditEvent(tx, company.ID, &workItem.ID, "chief", 0, "work_item.created_from_chat", string(payload))
	}); err != nil {
		return "", err
	}
	go func() {
		if _, _, startErr := startPersonalCompanyWorkRun(company, input.UserID, workItem.ID); startErr != nil {
			_ = recordPersonalCompanyWorkStartFailure(company.ID, workItem.ID, startErr)
		}
	}()
	return fmt.Sprintf("Created Studio work item #%d and queued its internal Chief-led session.", workItem.ID), nil
}

// handlePersonalCompanyChiefListWork returns a bounded operational snapshot for
// the external Chief chat. Internal work conversations stay focused on delivery.
func handlePersonalCompanyChiefListWork(_ context.Context, input AdvancedChatToolCallInput) (string, error) {
	if personalCompanyChiefRunIsInternal(input.RunID) {
		return "", errors.New("internal Studio work sessions cannot inspect the Studio work queue")
	}
	if input.Mode != advancedChatModeAgentGroup || strings.TrimSpace(input.AgentGroupID) == "" {
		return "", errors.New("Studio Chief context is required")
	}
	company, err := loadPersonalCompany(input.UserID, input.AgentGroupID)
	if err != nil {
		return "", err
	}
	type workSummary struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Priority int    `json:"priority"`
	}
	workItems := []workSummary{}
	if err := model.DB.Model(&model.CompanyWorkItem{}).
		Select("id", "title", "status", "priority").
		Where("personal_company_id = ?", company.ID).
		Order("updated_at DESC").
		Limit(12).
		Find(&workItems).Error; err != nil {
		return "", err
	}
	var workApprovals, connectorApprovals int64
	if err := model.DB.Model(&model.CompanyApprovalRequest{}).
		Where("personal_company_id = ? AND status = ?", company.ID, model.CompanyApprovalPending).
		Count(&workApprovals).Error; err != nil {
		return "", err
	}
	if err := model.DB.Table("advanced_chat_connector_tasks AS tasks").
		Joins("JOIN company_work_attempts AS attempts ON attempts.advanced_chat_run_id = tasks.run_id").
		Joins("JOIN company_work_items AS work ON work.id = attempts.work_item_id").
		Where("work.personal_company_id = ? AND tasks.status = ?", company.ID, advancedChatConnectorTaskStatusPendingApproval).
		Count(&connectorApprovals).Error; err != nil {
		return "", err
	}
	result, err := json.Marshal(gin.H{
		"studio":     gin.H{"name": company.Name, "state": company.State},
		"work_items": workItems,
		"pending_approvals": gin.H{
			"work":      workApprovals,
			"connector": connectorApprovals,
		},
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func personalCompanyChiefRunIsInternal(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	var count int64
	return model.DB.Model(&model.CompanyWorkAttempt{}).Where("advanced_chat_run_id = ?", runID).Count(&count).Error == nil && count > 0
}
