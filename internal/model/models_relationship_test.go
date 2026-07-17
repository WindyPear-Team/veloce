package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestReferralCommissionLogReferrerBelongsToUser(t *testing.T) {
	parsed, err := schema.Parse(&ReferralCommissionLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	relationship := parsed.Relationships.Relations["Referrer"]
	if relationship == nil || relationship.Type != schema.BelongsTo {
		t.Fatalf("Referrer relationship = %#v, want belongs_to", relationship)
	}
	if relationship.FieldSchema == nil || relationship.FieldSchema.Table != "users" {
		t.Fatalf("Referrer target table = %v, want users", relationship.FieldSchema)
	}
}
