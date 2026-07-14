package config

import "testing"

func TestGetEnvBoolDefaultsToFallback(t *testing.T) {
	t.Setenv("TEST_ENTERPRISE_BOOL", "")
	if getEnvBool("TEST_ENTERPRISE_BOOL", false) {
		t.Fatal("expected empty environment value to use false fallback")
	}
	if !getEnvBool("TEST_ENTERPRISE_BOOL", true) {
		t.Fatal("expected empty environment value to use true fallback")
	}
}

func TestGetEnvBoolParsesSupportedValues(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "true", want: true},
		{value: "TRUE", want: true},
		{value: "1", want: true},
		{value: "false", want: false},
		{value: "0", want: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("TEST_ENTERPRISE_BOOL", test.value)
			if got := getEnvBool("TEST_ENTERPRISE_BOOL", !test.want); got != test.want {
				t.Fatalf("getEnvBool(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestGetEnvBoolUsesFallbackForInvalidValue(t *testing.T) {
	t.Setenv("TEST_ENTERPRISE_BOOL", "not-a-bool")
	if !getEnvBool("TEST_ENTERPRISE_BOOL", true) {
		t.Fatal("expected invalid value to use fallback")
	}
}
