package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
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
