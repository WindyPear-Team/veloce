package service

import (
	"context"
	"os"
	"testing"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/shopspring/decimal"
)

// This test is opt-in because building the fixture requires TinyGo. CI can set
// VELOCE_PLUGIN_TEST_WASM to any helper-based plugin with a draw action.
func TestPluginRuntimeHelperWASM(t *testing.T) {
	wasmPath := os.Getenv("VELOCE_PLUGIN_TEST_WASM")
	if wasmPath == "" {
		t.Skip("VELOCE_PLUGIN_TEST_WASM is not set")
	}
	db := walletTestDB(t, "runtime-helper")
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	user := model.User{Username: "runtime-helper", Email: "runtime-helper@example.com", APIKey: "runtime-helper-key", Balance: decimal.NewFromInt(10)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	plugin := model.Plugin{
		ID: "balance-lottery-example", WASMPath: wasmPath,
		PermissionsJSON: mustJSON([]string{"wallet.balance.read", "wallet.balance.debit", "wallet.balance.credit"}),
	}
	result, err := InvokePluginAction(context.Background(), plugin, user.ID, "integration-draw-1", "draw", map[string]interface{}{"values": map[string]interface{}{}})
	if err != nil {
		t.Fatal(err)
	}
	if result["message"] == "" || result["balance"] == "" {
		t.Fatalf("unexpected helper plugin result: %#v", result)
	}
	replay, err := InvokePluginAction(context.Background(), plugin, user.ID, "integration-draw-1", "draw", map[string]interface{}{"values": map[string]interface{}{}})
	if err != nil {
		t.Fatal(err)
	}
	if replay["replay"] != true || replay["prize"] != result["prize"] {
		t.Fatalf("unexpected replay: first=%#v replay=%#v", result, replay)
	}
}
