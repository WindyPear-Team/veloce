package model

import "testing"

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
