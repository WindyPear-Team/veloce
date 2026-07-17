package model

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sqliteMigrationBatchSize = 500

// SQLiteMigrationReport describes a completed one-way SQLite export.
type SQLiteMigrationReport struct {
	TargetDriver string
	Tables       int
	Rows         int64
}

// MigrateSQLiteToDSN copies all application tables from sourcePath to an empty
// MySQL or PostgreSQL database. It never modifies the SQLite source.
func MigrateSQLiteToDSN(sourcePath, targetDSN string) (SQLiteMigrationReport, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	targetDSN = strings.TrimSpace(targetDSN)
	if sourcePath == "" {
		return SQLiteMigrationReport{}, errors.New("SQLite source path is required")
	}
	if targetDSN == "" {
		return SQLiteMigrationReport{}, errors.New("target DB_DSN is required")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("inspect SQLite source: %w", err)
	}
	if info.IsDir() {
		return SQLiteMigrationReport{}, fmt.Errorf("SQLite source path %q is a directory", sourcePath)
	}
	targetDriver, targetDialector, err := migrationTargetDialector(targetDSN)
	if err != nil {
		return SQLiteMigrationReport{}, err
	}

	source, err := gorm.Open(sqlite.Open(sqliteDSN(sourcePath)), &gorm.Config{})
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("open SQLite source: %w", err)
	}
	sourceSQL, err := source.DB()
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("access SQLite source: %w", err)
	}
	defer sourceSQL.Close()
	if err := configureDatabaseConnection(sourceSQL, true); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("configure SQLite source: %w", err)
	}
	if err := sourceSQL.Ping(); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("ping SQLite source: %w", err)
	}

	// Create the target without foreign keys first, so records can be copied in
	// batches without requiring a fragile ordering across every relationship.
	target, err := gorm.Open(targetDialector, &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("open %s target: %w", targetDriver, err)
	}
	targetSQL, err := target.DB()
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("access %s target: %w", targetDriver, err)
	}
	defer targetSQL.Close()
	if err := configureDatabaseConnection(targetSQL, false); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("configure %s target: %w", targetDriver, err)
	}
	if err := targetSQL.Ping(); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("ping %s target: %w", targetDriver, err)
	}

	models := sqliteMigrationModels()
	for _, item := range models {
		if target.Migrator().HasTable(item) {
			return SQLiteMigrationReport{}, fmt.Errorf("target database is not empty: table %q already exists", migrationModelTableName(target, item))
		}
	}
	if err := target.AutoMigrate(models...); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("create %s schema: %w", targetDriver, err)
	}

	report := SQLiteMigrationReport{TargetDriver: targetDriver}
	if err := target.Transaction(func(tx *gorm.DB) error {
		for _, item := range models {
			if !source.Migrator().HasTable(item) {
				continue
			}
			rows, err := copySQLiteMigrationTable(source, tx, item)
			if err != nil {
				return err
			}
			report.Tables++
			report.Rows += rows
		}
		return nil
	}); err != nil {
		return SQLiteMigrationReport{}, err
	}

	// The copied data now satisfies the source's relationships. Re-run schema
	// migration with constraints enabled so the target matches normal startup.
	targetWithConstraints, err := gorm.Open(targetDialector, &gorm.Config{})
	if err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("reopen %s target: %w", targetDriver, err)
	}
	if err := targetWithConstraints.AutoMigrate(models...); err != nil {
		return SQLiteMigrationReport{}, fmt.Errorf("finalize %s schema: %w", targetDriver, err)
	}
	return report, nil
}

func migrationModelTableName(db *gorm.DB, item interface{}) string {
	modelType := reflect.TypeOf(item)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	return db.NamingStrategy.TableName(modelType.Name())
}

func migrationTargetDialector(dsn string) (string, gorm.Dialector, error) {
	normalized := strings.ToLower(strings.TrimSpace(dsn))
	switch {
	case strings.HasPrefix(normalized, "postgres://"), strings.HasPrefix(normalized, "postgresql://"), strings.Contains(normalized, "host="), strings.Contains(normalized, "dbname="):
		return "postgres", postgres.Open(dsn), nil
	case strings.Contains(normalized, "@tcp("), strings.Contains(normalized, "@unix("), strings.HasPrefix(normalized, "mysql://"):
		return "mysql", mysql.Open(dsn), nil
	default:
		return "", nil, errors.New("DB_DSN must be a MySQL or PostgreSQL DSN when using --migrate")
	}
}

func copySQLiteMigrationTable(source, target *gorm.DB, item interface{}) (int64, error) {
	modelType := reflect.TypeOf(item)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	batchType := reflect.SliceOf(modelType)
	batch := reflect.New(batchType).Interface()
	var copied int64
	err := source.Model(item).FindInBatches(batch, sqliteMigrationBatchSize, func(tx *gorm.DB, _ int) error {
		value := reflect.ValueOf(batch).Elem()
		if value.Len() == 0 {
			return nil
		}
		if err := target.Omit(clause.Associations).Create(value.Interface()).Error; err != nil {
			return fmt.Errorf("copy table %s: %w", migrationModelTableName(source, item), err)
		}
		copied += int64(value.Len())
		return nil
	}).Error
	return copied, err
}

func sqliteMigrationModels() []interface{} {
	return []interface{}{
		&User{}, &UserAvatar{}, &APIKey{}, &EmailVerificationCode{}, &OIDCBindRequest{}, &WebAuthnChallenge{}, &PasskeyCredential{}, &CheckInRecord{}, &PaymentOrder{},
		&Group{}, &UserGroupMembership{}, &ChannelGroupMultiplier{}, &ModelGroupMultiplier{}, &ReferralCommissionLog{}, &UserChannel{}, &Channel{}, &Model{}, &ModelConfig{},
		&StatusMonitor{}, &StatusCheck{}, &Announcement{}, &SystemSetting{}, &VideoTask{}, &TokenLog{}, &AuditLog{}, &Plugin{}, &UserPluginState{}, &UserPluginConfig{}, &PluginKV{}, &PluginLog{},
		&PersonalCompany{}, &CompanyCharterRevision{}, &PersonalCompanyEmployee{}, &CompanyRoleTemplate{}, &CompanyEmployeeVersion{}, &CompanyCapabilityEvidence{}, &CompanyRecruitmentPlan{}, &CompanyObjective{}, &CompanyWorkItem{}, &CompanyWorkAttempt{}, &CompanyArtifact{}, &CompanyHandoffPackage{}, &CompanyApprovalRequest{}, &CompanyBudgetLedger{}, &CompanyAuditEvent{}, &CompanySignal{}, &CompanyOutboxEvent{},
		&Organization{}, &Department{}, &Workspace{}, &OrganizationMember{}, &WorkspaceMember{}, &Permission{}, &Role{}, &RolePermission{}, &RoleBinding{}, &DepartmentRoleBinding{}, &EnterpriseTask{}, &EnterpriseTaskAssignment{}, &EnterpriseTaskDepartment{}, &DepartmentMember{}, &EnterpriseSharedPool{}, &EnterpriseSharedSession{}, &EnterpriseSharedFile{}, &EnterpriseDevice{}, &EnterpriseDeviceAssignment{}, &QuotaAccount{}, &QuotaLedger{},
	}
}
