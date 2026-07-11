package service

import (
	"testing"

	"github.com/WindyPear-Team/veloce/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPersonalModeEnabledInTxUsesTransactionConnection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:system-mode-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("access database pool: %v", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := db.AutoMigrate(&model.SystemSetting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := db.Create(&model.SystemSetting{Key: "system_mode", Value: SystemModePersonal}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if !PersonalModeEnabledInTx(tx) {
			t.Fatal("expected personal mode from the transaction connection")
		}
		return nil
	}); err != nil {
		t.Fatalf("read setting in transaction: %v", err)
	}
}
