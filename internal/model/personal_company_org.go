package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	PersonalCompanyEmployeeProbation = "probation"
	PersonalCompanyEmployeeActive    = "active"
	PersonalCompanyEmployeeSuspended = "suspended"
	PersonalCompanyEmployeeRetired   = "retired"

	CompanyRecruitmentPlanProposed = "proposed"
	CompanyRecruitmentPlanApproved = "approved"
	CompanyRecruitmentPlanRejected = "rejected"
	CompanyRecruitmentPlanHired    = "hired"
)

// PersonalCompanyEmployee is a governed service identity. It is never a User
// account and may reference an AdvancedChatAgent only after a controlled hire.
type PersonalCompanyEmployee struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID   uint           `gorm:"uniqueIndex:idx_company_employee_key;index;not null" json:"personal_company_id"`
	EmployeeKey         string         `gorm:"uniqueIndex:idx_company_employee_key;size:80;not null" json:"employee_key"`
	AdvancedChatAgentID string         `gorm:"index;size:80;not null;default:''" json:"advanced_chat_agent_id,omitempty"`
	Name                string         `gorm:"size:160;not null" json:"name"`
	Role                string         `gorm:"size:100;not null" json:"role"`
	Status              string         `gorm:"size:20;not null;default:'probation';index" json:"status"`
	Version             int            `gorm:"not null;default:1" json:"version"`
	AllowedTools        string         `gorm:"type:text;not null;default:'[]'" json:"allowed_tools"`
	DataScope           string         `gorm:"type:text;not null;default:'[]'" json:"data_scope"`
	MaxRiskLevel        string         `gorm:"size:4;not null;default:'r0'" json:"max_risk_level"`
	MaxConcurrency      int            `gorm:"not null;default:1" json:"max_concurrency"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyRoleTemplate is a reusable position contract. It expresses the job's
// outputs and limits without granting permissions to a running agent.
type CompanyRoleTemplate struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"uniqueIndex:idx_company_role_template_key;index;not null" json:"personal_company_id"`
	TemplateKey       string         `gorm:"uniqueIndex:idx_company_role_template_key;size:80;not null" json:"template_key"`
	Name              string         `gorm:"size:160;not null" json:"name"`
	Responsibilities  string         `gorm:"type:text;not null;default:''" json:"responsibilities"`
	DefinitionOfDone  string         `gorm:"type:text;not null;default:''" json:"definition_of_done"`
	RequiredSkills    string         `gorm:"type:text;not null;default:'[]'" json:"required_skills"`
	AllowedTools      string         `gorm:"type:text;not null;default:'[]'" json:"allowed_tools"`
	MaxRiskLevel      string         `gorm:"size:4;not null;default:'r0'" json:"max_risk_level"`
	MaxConcurrency    int            `gorm:"not null;default:1" json:"max_concurrency"`
	Active            bool           `gorm:"not null;default:true;index" json:"active"`
	CreatedByUserID   uint           `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyEmployeeVersion freezes the configuration that was evaluated for an
// employee. A later change must produce a new version rather than mutate this.
type CompanyEmployeeVersion struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint      `gorm:"index;not null" json:"personal_company_id"`
	EmployeeID        uint      `gorm:"uniqueIndex:idx_company_employee_version;index;not null" json:"employee_id"`
	Version           int       `gorm:"uniqueIndex:idx_company_employee_version;not null" json:"version"`
	RoleTemplateID    *uint     `gorm:"index" json:"role_template_id,omitempty"`
	PromptProfile     string    `gorm:"type:text;not null;default:''" json:"prompt_profile"`
	ModelPolicy       string    `gorm:"type:text;not null;default:'{}'" json:"model_policy"`
	SkillScope        string    `gorm:"type:text;not null;default:'[]'" json:"skill_scope"`
	ToolGrants        string    `gorm:"type:text;not null;default:'[]'" json:"tool_grants"`
	DataScope         string    `gorm:"type:text;not null;default:'[]'" json:"data_scope"`
	EvalSummary       string    `gorm:"type:text;not null;default:''" json:"eval_summary"`
	CreatedByUserID   uint      `gorm:"index;not null" json:"created_by_user_id"`
	CreatedAt         time.Time `json:"created_at"`
}

type CompanyCapabilityEvidence struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"index;not null" json:"personal_company_id"`
	EmployeeID        uint           `gorm:"index;not null" json:"employee_id"`
	EmployeeVersionID *uint          `gorm:"index" json:"employee_version_id,omitempty"`
	Capability        string         `gorm:"size:160;not null" json:"capability"`
	EvidenceURI       string         `gorm:"type:text;not null" json:"evidence_uri"`
	EvaluationSummary string         `gorm:"type:text;not null;default:''" json:"evaluation_summary"`
	Score             float64        `gorm:"not null;default:0" json:"score"`
	Confidence        float64        `gorm:"not null;default:0" json:"confidence"`
	ValidUntil        *time.Time     `gorm:"index" json:"valid_until,omitempty"`
	VerifiedByUserID  uint           `gorm:"index;not null" json:"verified_by_user_id"`
	CreatedAt         time.Time      `json:"created_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// CompanyRecruitmentPlan records a capability gap and remains a proposal until
// the owner explicitly approves it. It never provisions external resources.
type CompanyRecruitmentPlan struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	PersonalCompanyID uint           `gorm:"index;not null" json:"personal_company_id"`
	RoleTemplateID    *uint          `gorm:"index" json:"role_template_id,omitempty"`
	Title             string         `gorm:"size:200;not null" json:"title"`
	CapabilityGap     string         `gorm:"type:text;not null" json:"capability_gap"`
	ExpectedBenefit   string         `gorm:"type:text;not null;default:''" json:"expected_benefit"`
	MaxRiskLevel      string         `gorm:"size:4;not null;default:'r0'" json:"max_risk_level"`
	ProposedTools     string         `gorm:"type:text;not null;default:'[]'" json:"proposed_tools"`
	Status            string         `gorm:"size:20;not null;default:'proposed';index" json:"status"`
	EmployeeID        *uint          `gorm:"index" json:"employee_id,omitempty"`
	CreatedByUserID   uint           `gorm:"index;not null" json:"created_by_user_id"`
	DecidedByUserID   *uint          `gorm:"index" json:"decided_by_user_id,omitempty"`
	DecidedAt         *time.Time     `json:"decided_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func NormalizePersonalCompanyEmployeeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PersonalCompanyEmployeeActive, PersonalCompanyEmployeeSuspended, PersonalCompanyEmployeeRetired:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return PersonalCompanyEmployeeProbation
	}
}

func NormalizeCompanyRecruitmentPlanStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CompanyRecruitmentPlanApproved, CompanyRecruitmentPlanRejected, CompanyRecruitmentPlanHired:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return CompanyRecruitmentPlanProposed
	}
}
