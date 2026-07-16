package service

import (
	"time"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/shopspring/decimal"
)

type personalCompanyDashboardResponse struct {
	Company    model.PersonalCompany          `json:"company"`
	Charter    *model.CompanyCharterRevision  `json:"charter,omitempty"`
	Objectives []model.CompanyObjective       `json:"objectives"`
	WorkItems  []model.CompanyWorkItem        `json:"work_items"`
	Approvals  []model.CompanyApprovalRequest `json:"approvals"`
	Budget     personalCompanyBudgetSummary   `json:"budget"`
	Health     personalCompanyHealthSummary   `json:"health"`
}

type personalCompanyBudgetSummary struct {
	DailyLimit      decimal.Decimal `json:"daily_limit"`
	MonthlyLimit    decimal.Decimal `json:"monthly_limit"`
	Reserved        decimal.Decimal `json:"reserved"`
	Consumed        decimal.Decimal `json:"consumed"`
	MonthlyReserved decimal.Decimal `json:"monthly_reserved"`
}

type personalCompanyHealthSummary struct {
	ActiveObjectives int `json:"active_objectives"`
	ActiveWorkItems  int `json:"active_work_items"`
	BlockedWorkItems int `json:"blocked_work_items"`
	PendingApprovals int `json:"pending_approvals"`
}

func personalCompanyDashboard(company model.PersonalCompany) (personalCompanyDashboardResponse, error) {
	response := personalCompanyDashboardResponse{
		Company: company, Objectives: []model.CompanyObjective{}, WorkItems: []model.CompanyWorkItem{}, Approvals: []model.CompanyApprovalRequest{},
		Budget: personalCompanyBudgetSummary{DailyLimit: company.DailyBudget, MonthlyLimit: company.MonthlyBudget},
	}
	if company.CharterRevisionID != nil {
		var charter model.CompanyCharterRevision
		if err := model.DB.Where("id = ? AND personal_company_id = ?", *company.CharterRevisionID, company.ID).First(&charter).Error; err == nil {
			response.Charter = &charter
		} else {
			return response, err
		}
	}
	if err := model.DB.Where("personal_company_id = ?", company.ID).Order("priority DESC, created_at DESC").Limit(8).Find(&response.Objectives).Error; err != nil {
		return response, err
	}
	if err := model.DB.Where("personal_company_id = ?", company.ID).Order("priority DESC, created_at DESC").Limit(12).Find(&response.WorkItems).Error; err != nil {
		return response, err
	}
	if err := model.DB.Where("personal_company_id = ? AND status = ?", company.ID, model.CompanyApprovalPending).Order("created_at ASC").Find(&response.Approvals).Error; err != nil {
		return response, err
	}
	if err := model.DB.Model(&model.CompanyWorkItem{}).Select("COALESCE(SUM(reserved_cost), 0)").Where("personal_company_id = ? AND status NOT IN ?", company.ID, []string{model.CompanyWorkStatusCancelled, model.CompanyWorkStatusDelivered}).Scan(&response.Budget.Reserved).Error; err != nil {
		return response, err
	}
	if err := model.DB.Model(&model.CompanyWorkItem{}).Select("COALESCE(SUM(consumed_cost), 0)").Where("personal_company_id = ?", company.ID).Scan(&response.Budget.Consumed).Error; err != nil {
		return response, err
	}
	monthStart := time.Now().UTC().AddDate(0, 0, -time.Now().UTC().Day()+1).Truncate(24 * time.Hour)
	if err := model.DB.Model(&model.CompanyBudgetLedger{}).Select("COALESCE(SUM(amount), 0)").Where("personal_company_id = ? AND entry_type IN ? AND created_at >= ?", company.ID, []string{"reservation", "release"}, monthStart).Scan(&response.Budget.MonthlyReserved).Error; err != nil {
		return response, err
	}
	var count int64
	if err := model.DB.Model(&model.CompanyObjective{}).Where("personal_company_id = ? AND status = ?", company.ID, model.CompanyObjectiveStatusActive).Count(&count).Error; err != nil {
		return response, err
	}
	response.Health.ActiveObjectives = int(count)
	if err := model.DB.Model(&model.CompanyWorkItem{}).Where("personal_company_id = ? AND status IN ?", company.ID, []string{model.CompanyWorkStatusPlanned, model.CompanyWorkStatusAuthorized, model.CompanyWorkStatusQueued, model.CompanyWorkStatusExecuting, model.CompanyWorkStatusAwaitingReview}).Count(&count).Error; err != nil {
		return response, err
	}
	response.Health.ActiveWorkItems = int(count)
	if err := model.DB.Model(&model.CompanyWorkItem{}).Where("personal_company_id = ? AND status = ?", company.ID, model.CompanyWorkStatusBlocked).Count(&count).Error; err != nil {
		return response, err
	}
	response.Health.BlockedWorkItems = int(count)
	if err := model.DB.Model(&model.CompanyApprovalRequest{}).Where("personal_company_id = ? AND status = ?", company.ID, model.CompanyApprovalPending).Count(&count).Error; err != nil {
		return response, err
	}
	response.Health.PendingApprovals = int(count)
	return response, nil
}
