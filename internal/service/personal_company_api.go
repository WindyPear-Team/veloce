package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type personalCompanyAPI struct{}

// RegisterPersonalCompanyUserRoutes keeps the personal-company surface outside
// enterprise routing and permissions. Every handler enforces personal mode and
// ownership itself.
func RegisterPersonalCompanyUserRoutes(group *gin.RouterGroup) {
	api := &personalCompanyAPI{}
	group.GET("/personal-company", api.getCompany)
	group.POST("/personal-company/bootstrap", api.bootstrapCompany)
	group.PUT("/personal-company/charter", api.updateCharter)
	group.POST("/personal-company/pause", api.pauseCompany)
	group.POST("/personal-company/resume", api.resumeCompany)
	group.PUT("/personal-company/runtime", api.updateStudioRuntime)
	group.GET("/personal-company/objectives", api.listObjectives)
	group.POST("/personal-company/objectives", api.createObjective)
	group.GET("/personal-company/org-chart", api.getOrgChart)
	group.GET("/personal-company/role-templates", api.listRoleTemplates)
	group.POST("/personal-company/role-templates", api.createRoleTemplate)
	group.GET("/personal-company/staffing/recruitment-plans", api.listRecruitmentPlans)
	group.POST("/personal-company/staffing/recruitment-plans", api.createRecruitmentPlan)
	group.POST("/personal-company/staffing/recruitment-plans/:id/approve", api.approveRecruitmentPlan)
	group.POST("/personal-company/employees/:id/capability-evidence", api.recordCapabilityEvidence)
	group.POST("/personal-company/employees/:id/promote", api.promoteEmployee)
	group.POST("/personal-company/employees/:id/runtime-binding", api.bindEmployeeRuntime)
	group.GET("/personal-company/work-items", api.listWorkItems)
	group.POST("/personal-company/work-items", api.createWorkItem)
	group.GET("/personal-company/work-items/:id/timeline", api.getWorkItemTimeline)
	group.POST("/personal-company/work-items/:id/handoffs", api.createWorkItemHandoff)
	group.POST("/personal-company/work-items/:id/approve", api.approveWorkItem)
	group.POST("/personal-company/work-items/:id/queue", api.queueWorkItem)
	group.POST("/personal-company/work-items/:id/run", api.runWorkItem)
	group.POST("/personal-company/work-items/:id/cancel", api.cancelWorkItem)
	group.POST("/personal-company/handoffs/:id/decide", api.decideHandoff)
	group.GET("/personal-company/approvals", api.listApprovals)
	group.POST("/personal-company/approvals/:id/decide", api.decideApproval)
}

func (api *personalCompanyAPI) personalCompanyContext(c *gin.Context) (*personalCompanyRequestContext, bool) {
	if !PersonalModeEnabled() {
		c.JSON(http.StatusConflict, gin.H{"error": "Personal Company is available only in personal mode"})
		return nil, false
	}
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return nil, false
	}
	agentGroupID := strings.TrimSpace(c.Query("studio_id"))
	if agentGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "studio_id is required"})
		return nil, false
	}
	if _, err := readAdvancedChatAgentGroup(c.Request.Context(), user.ID, nil, agentGroupID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent Studio not found"})
		return nil, false
	}
	return &personalCompanyRequestContext{userID: user.ID, agentGroupID: agentGroupID}, true
}
