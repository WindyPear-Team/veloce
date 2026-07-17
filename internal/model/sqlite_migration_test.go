package model

import "testing"

func TestMigrationTargetDialector(t *testing.T) {
	tests := []struct {
		name   string
		dsn    string
		driver string
		valid  bool
	}{
		{name: "PostgreSQL URL", dsn: "postgres://user:password@localhost:5432/veloce", driver: "postgres", valid: true},
		{name: "PostgreSQL key value", dsn: "host=localhost user=veloce dbname=veloce sslmode=disable", driver: "postgres", valid: true},
		{name: "MySQL", dsn: "veloce:password@tcp(127.0.0.1:3306)/veloce?parseTime=True", driver: "mysql", valid: true},
		{name: "SQLite is unsupported", dsn: "file:veloce.db", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, _, err := migrationTargetDialector(test.dsn)
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected unsupported target error")
			}
			if driver != test.driver {
				t.Fatalf("driver = %q, want %q", driver, test.driver)
			}
		})
	}
}
