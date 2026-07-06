package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/WindyPear-Team/flai/internal/model"
)

func TestAdvancedChatModelRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 500 * time.Millisecond},
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 6, want: assistantModelRetryMaxDelay},
	}
	for _, tc := range cases {
		if got := advancedChatModelRetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("advancedChatModelRetryDelay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestRetryableAdvancedChatModelRequestError(t *testing.T) {
	if !retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusTooManyRequests, Message: "Upstream request failed"}) {
		t.Fatal("429 upstream errors should be retried")
	}
	if !retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusBadGateway, Message: "Failed to read upstream response"}) {
		t.Fatal("upstream response read failures should be retried")
	}
	if retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusInternalServerError, Message: "Failed to update balance"}) {
		t.Fatal("internal billing errors should not be retried")
	}
	if retryableAdvancedChatModelRequestError(errors.New("plain error")) {
		t.Fatal("plain errors should not be retried")
	}
}

func TestServerChatExecutorPreservesRoutingStateAcrossCalls(t *testing.T) {
	previous := serverChatProxyService
	serverChatProxyService = NewProxyService()
	defer func() { serverChatProxyService = previous }()

	userChannelID := uint(11)
	candidates := []model.ModelConfig{
		{Channel: model.Channel{ID: 1, UserChannelID: &userChannelID, UserChannel: model.UserChannel{RoutingAlgorithm: RoutingRoundRobin}}},
		{Channel: model.Channel{ID: 2, UserChannelID: &userChannelID, UserChannel: model.UserChannel{RoutingAlgorithm: RoutingRoundRobin}}},
	}

	first := serverChatExecutor().selectModelConfig(candidates, "gpt-test")
	second := serverChatExecutor().selectModelConfig(candidates, "gpt-test")
	if first.Channel.ID != 1 || second.Channel.ID != 2 {
		t.Fatalf("server chat executor should preserve routing state across calls, got %d then %d", first.Channel.ID, second.Channel.ID)
	}
}

func TestNormalizeConnectorTaskResultIncludesCommandFailureOutput(t *testing.T) {
	exitCode := 2
	input := advancedChatConnectorTaskResultInput{
		Success:  false,
		Stdout:   "stdout text",
		Stderr:   "stderr text",
		Error:    "command failed",
		ExitCode: &exitCode,
	}
	result := normalizeConnectorTaskResultText(input)
	if !strings.Contains(result, "stdout:\nstdout text") {
		t.Fatalf("result should include stdout, got %q", result)
	}
	if !strings.Contains(result, "stderr:\nstderr text") {
		t.Fatalf("result should include stderr, got %q", result)
	}
	message := normalizeConnectorTaskErrorMessage(input)
	if !strings.Contains(message, "command failed") || !strings.Contains(message, "exit code 2") {
		t.Fatalf("error message should include command error and exit code, got %q", message)
	}
}

func TestAgentStudioRolePermissions(t *testing.T) {
	if advancedChatAgentStudioCanUseExecutionTools("chief") {
		t.Fatal("chief agents must not receive execution tools")
	}
	if advancedChatAgentStudioCanSplit("chief") {
		t.Fatal("chief agents must not split into sub-agents")
	}
	if !advancedChatAgentStudioCanDelegate("chief", true) {
		t.Fatal("chief agents should be able to delegate within an agent group")
	}
	if !advancedChatAgentStudioConnectorActionAllowed("chief", "list_files") || !advancedChatAgentStudioConnectorActionAllowed("chief", "read_file") || !advancedChatAgentStudioConnectorActionAllowed("chief", "run_command") {
		t.Fatal("chief agents should be able to inspect workspaces and run diagnostic commands")
	}
	if advancedChatAgentStudioConnectorActionAllowed("chief", "write_file") || advancedChatAgentStudioConnectorActionAllowed("chief", "replace_text") {
		t.Fatal("chief agents must not receive file mutation tools")
	}
	if !advancedChatAgentStudioCanUseExecutionTools("worker") || !advancedChatAgentStudioCanSplit("worker") {
		t.Fatal("worker agents should be able to execute and split")
	}
	if advancedChatAgentStudioCanUseExecutionTools("checker") || advancedChatAgentStudioCanSplit("checker") || advancedChatAgentStudioCanDelegate("checker", true) {
		t.Fatal("checker agents must not execute, split, or delegate")
	}
	if advancedChatAgentStudioConnectorActionAllowed("checker", "read_file") || advancedChatAgentStudioConnectorActionAllowed("checker", "write_file") {
		t.Fatal("checker agents must not receive connector tools")
	}
}

func TestNormalizeAdvancedChatAgentGroupRequiresOneChecker(t *testing.T) {
	base := advancedChatAgentGroupInput{
		ID:   "studio",
		Name: "Studio",
		Agents: []advancedChatGroupAgent{
			{ID: "chief", Name: "Chief", Type: "chief", ChatAgentID: "1"},
			{ID: "checker", Name: "Checker", Type: "checker", ChatAgentID: "2"},
			{ID: "worker", Name: "Worker", Type: "worker", ChatAgentID: "3"},
		},
	}
	if _, err := normalizeAdvancedChatAgentGroup(base); err != nil {
		t.Fatalf("expected one chief and one checker to be valid, got %v", err)
	}
	missingChecker := base
	missingChecker.Agents = []advancedChatGroupAgent{
		{ID: "chief", Name: "Chief", Type: "chief", ChatAgentID: "1"},
		{ID: "worker", Name: "Worker", Type: "worker", ChatAgentID: "3"},
	}
	if _, err := normalizeAdvancedChatAgentGroup(missingChecker); err == nil || !strings.Contains(err.Error(), "exactly one checker") {
		t.Fatalf("expected missing checker to fail, got %v", err)
	}
	twoCheckers := base
	twoCheckers.Agents = append(twoCheckers.Agents, advancedChatGroupAgent{ID: "checker2", Name: "Checker 2", Type: "checker", ChatAgentID: "4"})
	if _, err := normalizeAdvancedChatAgentGroup(twoCheckers); err == nil || !strings.Contains(err.Error(), "exactly one checker") {
		t.Fatalf("expected duplicate checker to fail, got %v", err)
	}
}

func TestAgentStudioCheckerDecisionRequiresApprovalTool(t *testing.T) {
	yes, opinion, err := advancedChatAgentStudioCheckerDecision(&ChatExecutorResult{ToolCalls: []ChatExecutorToolCall{{
		Name:      advancedChatAgentStudioApprovalToolName,
		Arguments: `{"decision":"yes","opinion":"scoped change"}`,
	}}})
	if err != nil || !yes || opinion != "scoped change" {
		t.Fatalf("expected yes decision with opinion, got yes=%v opinion=%q err=%v", yes, opinion, err)
	}
	no, _, err := advancedChatAgentStudioCheckerDecision(&ChatExecutorResult{ToolCalls: []ChatExecutorToolCall{{
		Name:      advancedChatAgentStudioApprovalToolName,
		Arguments: `{"decision":"no"}`,
	}}})
	if err != nil || no {
		t.Fatalf("expected no decision, got no=%v err=%v", no, err)
	}
	if _, _, err := advancedChatAgentStudioCheckerDecision(&ChatExecutorResult{Content: "yes"}); err == nil {
		t.Fatal("checker must call approval decision tool")
	}
}

func TestCommitDeltaRequiresApprovalUnlessAutoApproved(t *testing.T) {
	binding := advancedChatConnectorToolBinding{Action: "commit_delta"}
	if !advancedChatConnectorTaskRequiresApproval(binding, map[string]interface{}{"mutations": []interface{}{}}) {
		t.Fatal("commit_delta should require approval by default")
	}
	binding.AutoApprove = true
	if advancedChatConnectorTaskRequiresApproval(binding, map[string]interface{}{"mutations": []interface{}{}}) {
		t.Fatal("commit_delta should respect connector auto approval")
	}
	if advancedChatConnectorTaskRequiresApproval(advancedChatConnectorToolBinding{Action: "file_sha256"}, map[string]interface{}{"path": "main.go"}) {
		t.Fatal("file_sha256 should be treated as read-only and not require approval")
	}
}

func TestAgentStudioReplaceMutationsUseBaseSHA(t *testing.T) {
	mutations := advancedChatAgentStudioMutationsFromReplaceArguments(map[string]interface{}{
		"path":     "main.go",
		"old_text": "old",
		"new_text": "new",
	}, func(path string) string {
		if path != "main.go" {
			t.Fatalf("unexpected path %q", path)
		}
		return sha256Hex("base")
	})
	if len(mutations) != 1 {
		t.Fatalf("expected one mutation, got %d", len(mutations))
	}
	if mutations[0].BaseSHA256 != sha256Hex("base") {
		t.Fatalf("base sha = %q, want %q", mutations[0].BaseSHA256, sha256Hex("base"))
	}
}

func TestValidateAgentStudioMutationsDetectsConflicts(t *testing.T) {
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{{
		Action:  "replace_text",
		Path:    "main.go",
		OldText: "old",
		NewText: "new",
	}}); err != nil {
		t.Fatalf("expected valid mutation, got %v", err)
	}
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{
		{Action: "replace_text", Path: "main.go", OldText: "old", NewText: "new"},
		{Action: "replace_text", Path: "main.go", OldText: "old", NewText: "other"},
	}); err == nil || !strings.Contains(err.Error(), "conflicting replace_text") {
		t.Fatalf("expected replace conflict, got %v", err)
	}
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{
		{Action: "write_file", Path: "main.go", Content: "one"},
		{Action: "write_file", Path: "main.go", Content: "two"},
	}); err == nil || !strings.Contains(err.Error(), "conflicting write_file") {
		t.Fatalf("expected write conflict, got %v", err)
	}
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{{
		Action: "replace_text",
		Path:   "main.go",
	}}); err == nil || !strings.Contains(err.Error(), "old_text is required") {
		t.Fatalf("expected old_text validation error, got %v", err)
	}
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{{
		Action: "delete_file",
		Path:   "main.go",
	}}); err != nil {
		t.Fatalf("expected delete_file mutation to validate, got %v", err)
	}
	if err := validateAdvancedChatAgentStudioMutations([]advancedChatAgentStudioMutation{
		{Action: "write_file", Path: "main.go", Content: "next"},
		{Action: "delete_file", Path: "main.go"},
	}); err == nil || !strings.Contains(err.Error(), "conflicting delete_file") {
		t.Fatalf("expected delete conflict, got %v", err)
	}
}

func TestAgentStudioSandboxArguments(t *testing.T) {
	sandboxID := advancedChatAgentStudioSandboxID("run/1", "group A", "worker")
	if sandboxID != "run-1-group-A-worker" {
		t.Fatalf("unexpected sandbox id %q", sandboxID)
	}
	arguments := map[string]interface{}{"command": "go test ./..."}
	next := advancedChatAgentStudioSandboxArguments(arguments, sandboxID)
	if next[advancedChatAgentStudioSandboxIDArg] != sandboxID {
		t.Fatalf("sandbox id was not injected: %+v", next)
	}
	if next[advancedChatAgentStudioSandboxBackendArg] != "appcontainer" {
		t.Fatalf("sandbox backend was not injected: %+v", next)
	}
	if _, exists := arguments[advancedChatAgentStudioSandboxIDArg]; exists {
		t.Fatal("sandbox argument injection must not mutate the original argument map")
	}
}

func TestAgentStudioSubAgentStatusHelpers(t *testing.T) {
	payload := map[string]interface{}{
		"task_id":    "task-1",
		"agent_id":   "split-a",
		"agent_name": "Split A",
	}
	if !advancedChatAgentStudioSubAgentMatches(payload, "task-1") {
		t.Fatal("status query should match task id")
	}
	if !advancedChatAgentStudioSubAgentMatches(payload, "split-a") {
		t.Fatal("status query should match agent id")
	}
	if !advancedChatAgentStudioSubAgentMatches(payload, "Split A") {
		t.Fatal("status query should match agent name")
	}
	if advancedChatAgentStudioSubAgentMatches(payload, "other") {
		t.Fatal("status query should not match unrelated ids")
	}
	if key := advancedChatAgentStudioSubAgentKey(payload); key != "task-1" {
		t.Fatalf("expected task id to be the stable key, got %q", key)
	}
}

func TestMergeAdvancedChatToolCallDetailListPreservesStreamedWorkerCalls(t *testing.T) {
	current := []advancedChatCompletionToolCall{
		{ID: "delegate-1", Name: "agent_delegate", Server: "Agent Studio", Tool: "agent_delegate", Status: "running"},
		{ID: "worker-read-1", Name: "workspace_read_file", Server: "local", Tool: "read_file", Status: "ok", Result: "read result"},
	}
	final := []advancedChatCompletionToolCall{
		{ID: "delegate-1", Name: "agent_delegate", Server: "Agent Studio", Tool: "agent_delegate", Status: "ok", Result: "[worker] done"},
	}
	merged := mergeAdvancedChatToolCallDetailList(current, final)
	if len(merged) != 2 {
		t.Fatalf("expected merged list to preserve streamed worker calls, got %d items", len(merged))
	}
	if merged[0].Status != "ok" || merged[0].Result != "[worker] done" {
		t.Fatalf("expected final delegate status to override running, got status=%q result=%q", merged[0].Status, merged[0].Result)
	}
	if merged[1].ID != "worker-read-1" || merged[1].Status != "ok" {
		t.Fatalf("expected worker tool call to be preserved, got %+v", merged[1])
	}
}

func TestCancelActiveAdvancedChatToolCalls(t *testing.T) {
	details := []advancedChatCompletionToolCall{
		{ID: "running", Name: "agent_delegate", Status: "running"},
		{ID: "approval", Name: "workspace_write_file", Status: "approval_required"},
		{ID: "done", Name: "workspace_read_file", Status: "ok", Result: "kept"},
	}
	updated, changed := cancelActiveAdvancedChatToolCalls(details, "Cancelled by user.")
	if !changed {
		t.Fatal("expected active tool calls to be marked changed")
	}
	if updated[0].Status != "error" || updated[1].Status != "error" {
		t.Fatalf("expected active calls to become error, got %q and %q", updated[0].Status, updated[1].Status)
	}
	if updated[2].Status != "ok" || updated[2].Result != "kept" {
		t.Fatalf("completed calls should not be changed, got %+v", updated[2])
	}
}

func TestAgentStudioDeltaLogVirtualRead(t *testing.T) {
	delta := &advancedChatAgentStudioDeltaLog{}
	delta.append(advancedChatAgentStudioMutation{Action: "replace_text", Path: "main.go", OldText: "old", NewText: "new"})
	if got := delta.applyToPath("main.go", "old value"); got != "new value" {
		t.Fatalf("virtual read should apply deferred replacement, got %q", got)
	}
	delta.append(advancedChatAgentStudioMutation{Action: "write_file", Path: "main.go", Content: "final"})
	if got := delta.applyToPath("main.go", "old value"); got != "final" {
		t.Fatalf("virtual read should apply deferred write last, got %q", got)
	}
}

func TestNormalizeAdvancedChatGroupAgentsPreservesMemberTools(t *testing.T) {
	agents := normalizeAdvancedChatGroupAgents([]advancedChatGroupAgent{{
		ID:           "worker",
		Name:         "Worker",
		Type:         "worker",
		ChatAgentID:  "1",
		SkillIDs:     []string{"skill-a", "skill-a", "skill-b"},
		MCPServerIDs: []string{"mcp-a", "", "mcp-b", "mcp-a"},
	}})
	if len(agents) != 1 {
		t.Fatalf("expected one normalized agent, got %d", len(agents))
	}
	if got := strings.Join(agents[0].SkillIDs, ","); got != "skill-a,skill-b" {
		t.Fatalf("skill ids were not preserved and deduplicated, got %q", got)
	}
	if got := strings.Join(agents[0].MCPServerIDs, ","); got != "mcp-a,mcp-b" {
		t.Fatalf("mcp server ids were not preserved and deduplicated, got %q", got)
	}
}
