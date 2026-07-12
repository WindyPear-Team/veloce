package service

import "testing"

func TestPluginActionAllowed(t *testing.T) {
	allowed := []PluginHookResult{{PluginID: "audit", Output: map[string]interface{}{"allow": true}}}
	if err := pluginActionAllowed(allowed); err != nil {
		t.Fatalf("expected allowed action, got %v", err)
	}

	denied := []PluginHookResult{{PluginID: "policy", Output: map[string]interface{}{"deny": true, "message": "policy denied this action"}}}
	if err := pluginActionAllowed(denied); err == nil || err.Error() != "policy denied this action" {
		t.Fatalf("expected hook denial message, got %v", err)
	}
}

func TestPluginHookMatchesAction(t *testing.T) {
	if !pluginHookMatches(PluginHook{Point: PluginHookPointPluginActionBefore, Action: "*"}, PluginHookPointPluginActionBefore, "publish") {
		t.Fatal("expected wildcard hook to match action")
	}
	if pluginHookMatches(PluginHook{Point: PluginHookPointPluginActionBefore, Action: "publish"}, PluginHookPointPluginActionBefore, "delete") {
		t.Fatal("unexpected action match")
	}
}
