package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLogDatabaseIsIndependentAndSupportsDailyFiles(t *testing.T) {
	previousDB, previousRoot := DB, logRoot
	logDatabasesMu.Lock()
	previousDatabases := logDatabases
	logDatabases = map[string]*gorm.DB{}
	logDatabasesMu.Unlock()
	DB = nil
	logRoot = filepath.Join(t.TempDir(), "log")
	t.Cleanup(func() {
		logDatabasesMu.Lock()
		for _, database := range logDatabases {
			if sqlDB, err := database.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		logDatabases = previousDatabases
		logDatabasesMu.Unlock()
		DB = previousDB
		logRoot = previousRoot
	})

	mainDB, err := gorm.Open(sqlite.Open("file:log-db-main?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mainDB.AutoMigrate(&SystemSetting{}); err != nil {
		t.Fatal(err)
	}
	DB = mainDB
	if err := SetSystemSetting("log_storage_mode", LogStorageDaily); err != nil {
		t.Fatal(err)
	}

	database, err := LogDB()
	if err != nil {
		t.Fatal(err)
	}
	entry := TokenLog{ID: NextLogID(), UserID: 42, ModelName: "test", CreatedAt: time.Now()}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if mainDB.Migrator().HasTable(&TokenLog{}) {
		t.Fatal("token_logs must not be created in the primary database")
	}
	if database.Migrator().HasTable(&StatusMonitor{}) {
		t.Fatal("status_monitors must not be created in the log database")
	}
	logs, total, err := ListTokenLogs(TokenLogFilter{}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 || logs[0].ID != entry.ID {
		t.Fatalf("unexpected log query result: total=%d logs=%#v", total, logs)
	}
	if _, err := DeleteLogs(); err != nil {
		t.Fatal(err)
	}
}

func TestClearLegacyLogsRemovesExistingPrimaryDatabaseRecords(t *testing.T) {
	previousDB := DB
	t.Cleanup(func() { DB = previousDB })

	mainDB, err := gorm.Open(sqlite.Open("file:legacy-log-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mainDB.AutoMigrate(&AuditLog{}, &TokenLog{}, &PluginLog{}, &StatusCheck{}); err != nil {
		t.Fatal(err)
	}
	DB = mainDB
	if err := mainDB.Create(&AuditLog{ID: 1, LogType: "system", Action: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := mainDB.Create(&TokenLog{ID: 2, UserID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := mainDB.Create(&PluginLog{ID: 3, Level: "info", Event: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := mainDB.Create(&StatusCheck{ID: 4, MonitorID: 1, Status: "ok"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := ClearLegacyLogs(); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []interface{}{&AuditLog{}, &TokenLog{}, &PluginLog{}, &StatusCheck{}} {
		var count int64
		if err := mainDB.Model(entry).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("legacy table %T still has %d records", entry, count)
		}
	}
}
