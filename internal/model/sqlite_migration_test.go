package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrationTargetDialector(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		dsn    string
		want   string
		valid  bool
	}{
		{name: "PostgreSQL URL", driver: "postgres", dsn: "postgres://user:password@localhost:5432/veloce", want: "postgres", valid: true},
		{name: "PostgreSQL alias", driver: "postgresql", dsn: "host=localhost user=veloce dbname=veloce sslmode=disable", want: "postgres", valid: true},
		{name: "MySQL", driver: "mysql", dsn: "veloce:password@tcp(127.0.0.1:3306)/veloce?parseTime=True", want: "mysql", valid: true},
		{name: "SQLite is unsupported", driver: "sqlite", dsn: "file:veloce.db", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, _, err := migrationTargetDialector(test.driver, test.dsn)
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected unsupported target error")
			}
			if driver != test.want {
				t.Fatalf("driver = %q, want %q", driver, test.want)
			}
		})
	}
}

func TestDiscardDanglingModelConfigs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sqlite-migration-model-configs?mode=memory&cache=shared"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Channel{}, &Model{}, &ModelConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Omit("Models", "Configs").Create(&Channel{ID: 1, Name: "channel"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Omit("Configs").Create(&Model{ID: 1, ModelName: "model"}).Error; err != nil {
		t.Fatal(err)
	}
	configs := []ModelConfig{{ID: 1, ChannelID: 1, ModelID: 1}, {ID: 2, ChannelID: 2, ModelID: 1}, {ID: 3, ChannelID: 1, ModelID: 2}}
	if err := db.Omit("Channel", "Model", "GroupMultipliers").Create(&configs).Error; err != nil {
		t.Fatal(err)
	}
	discarded, err := discardDanglingModelConfigs(db)
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 2 {
		t.Fatalf("discarded = %d, want 2", discarded)
	}
	var count int64
	if err := db.Model(&ModelConfig{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("remaining model configs = %d, want 1", count)
	}
}

func TestRepairDanglingPluginLogReference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sqlite-migration-plugin-logs?mode=memory&cache=shared"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Plugin{}, &PluginLog{}); err != nil {
		t.Fatal(err)
	}
	plugin := Plugin{ID: "valid-plugin", Name: "Plugin", Version: "1", ManifestJSON: "{}", Path: "plugin.wasm"}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatal(err)
	}
	logs := []PluginLog{{ID: 1, PluginID: "valid-plugin", Level: "info", Event: "test"}, {ID: 2, PluginID: "missing-plugin", Level: "info", Event: "test"}}
	if err := db.Omit("Plugin", "User").Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	repaired, err := repairDanglingReferences(db, []interface{}{&Plugin{}, &PluginLog{}})
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}
	var dangling PluginLog
	if err := db.First(&dangling, 2).Error; err != nil {
		t.Fatal(err)
	}
	if dangling.PluginID != "" {
		t.Fatalf("plugin ID = %q, want empty after NULL repair", dangling.PluginID)
	}
}
