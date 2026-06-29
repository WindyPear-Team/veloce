package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdvancedChatModelRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 500 * time.Millisecond},
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 6, want: assistantModelRetryMaxDelay},
	}
	for _, tc := range cases {
		if got := advancedChatModelRetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("advancedChatModelRetryDelay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestRetryableAdvancedChatModelRequestError(t *testing.T) {
	if !retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusTooManyRequests, Message: "Upstream request failed"}) {
		t.Fatal("429 upstream errors should be retried")
	}
	if !retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusBadGateway, Message: "Failed to read upstream response"}) {
		t.Fatal("upstream response read failures should be retried")
	}
	if retryableAdvancedChatModelRequestError(&ChatExecutorError{Status: http.StatusInternalServerError, Message: "Failed to update balance"}) {
		t.Fatal("internal billing errors should not be retried")
	}
	if retryableAdvancedChatModelRequestError(errors.New("plain error")) {
		t.Fatal("plain errors should not be retried")
	}
}

func TestNormalizeConnectorTaskResultIncludesCommandFailureOutput(t *testing.T) {
	exitCode := 2
	input := advancedChatConnectorTaskResultInput{
		Success:  false,
		Stdout:   "stdout text",
		Stderr:   "stderr text",
		Error:    "command failed",
		ExitCode: &exitCode,
	}
	result := normalizeConnectorTaskResultText(input)
	if !strings.Contains(result, "stdout:\nstdout text") {
		t.Fatalf("result should include stdout, got %q", result)
	}
	if !strings.Contains(result, "stderr:\nstderr text") {
		t.Fatalf("result should include stderr, got %q", result)
	}
	message := normalizeConnectorTaskErrorMessage(input)
	if !strings.Contains(message, "command failed") || !strings.Contains(message, "exit code 2") {
		t.Fatalf("error message should include command error and exit code, got %q", message)
	}
}
