package channel

import (
	"strings"
	"testing"

	communityservice "github.com/WindyPear-Team/veloce/internal/service"
)

func TestParseMessageChannelApprovalDecision(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
		ok   bool
	}{
		{name: "yes", text: "yes", want: true, ok: true},
		{name: "quoted yes", text: `"yes"`, want: true, ok: true},
		{name: "curly quoted yes", text: `“yes”`, want: true, ok: true},
		{name: "yes punctuation", text: "yes。", want: true, ok: true},
		{name: "reply yes", text: "回复 yes", want: true, ok: true},
		{name: "chinese approve", text: "同意", want: true, ok: true},
		{name: "no", text: "no", want: false, ok: true},
		{name: "quoted no", text: `"no"`, want: false, ok: true},
		{name: "chinese deny", text: "拒绝。", want: false, ok: true},
		{name: "unknown", text: "稍等", want: false, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMessageChannelApprovalDecision(tt.text)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseMessageChannelApprovalDecision(%q) = (%v, %v), want (%v, %v)", tt.text, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestFormatMessageChannelApprovalPromptChinese(t *testing.T) {
	prompt := formatMessageChannelApprovalPrompt(communityservice.MessageChannelConnectorApproval{
		TaskID:        "task-1",
		DeviceName:    "dev",
		Action:        "run_command",
		WorkspacePath: "D:/dev/project",
		Arguments:     map[string]interface{}{"command": "go test ./..."},
	})
	for _, want := range []string{"连接器操作需要审批", "任务：task-1", "回复 yes 或 同意以批准", "回复 no 或 拒绝以拒绝"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("approval prompt missing %q:\n%s", want, prompt)
		}
	}
}
