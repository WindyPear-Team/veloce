package service

import "testing"

func TestParseAdvancedChatGeneratedAgentConfig(t *testing.T) {
	config, err := parseAdvancedChatGeneratedAgentConfig("```json\n{\"name\":\"Research assistant\",\"prompt\":\"Research topics and cite sources.\"}\n```")
	if err != nil {
		t.Fatalf("parse generated config: %v", err)
	}
	if config.Name != "Research assistant" || config.Prompt != "Research topics and cite sources." {
		t.Fatalf("unexpected generated config: %#v", config)
	}
}

func TestParseAdvancedChatGeneratedAgentConfigRejectsIncompleteConfig(t *testing.T) {
	if _, err := parseAdvancedChatGeneratedAgentConfig(`{"name":"Missing prompt"}`); err == nil {
		t.Fatal("expected incomplete configuration to be rejected")
	}
}
