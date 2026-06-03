package telemetry

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestSetupNoEndpoint(t *testing.T) {
	// Ensure no endpoint is set
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	shutdown := Setup()
	if shutdown == nil {
		t.Fatal("expected no-op shutdown func, got nil")
	}
	// Must not panic
	shutdown()
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world this is long", 10, "hello w..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
		{"", 10, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expected)
		}
	}
}

func TestTraceHTTPSuccess(t *testing.T) {
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	// Reset to no-op tracer
	Setup()

	err := TraceHTTP(context.Background(), "GET", "http://example.com/api", func() (int, error) {
		return 200, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTraceHTTPError(t *testing.T) {
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	Setup()

	expectedErr := errors.New("connection refused")
	err := TraceHTTP(context.Background(), "POST", "http://example.com/api", func() (int, error) {
		return 0, expectedErr
	})
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestSetupWithInvalidEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "invalid://bad-url-%xx")
	shutdown := Setup()
	if shutdown == nil {
		t.Fatal("expected no-op shutdown func on error, got nil")
	}
	shutdown()
}

func TestTraceHTTPNon200(t *testing.T) {
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	Setup()

	err := TraceHTTP(context.Background(), "GET", "http://example.com/api", func() (int, error) {
		return 500, nil
	})
	if err != nil {
		t.Errorf("unexpected error for non-200: %v", err)
	}
}
