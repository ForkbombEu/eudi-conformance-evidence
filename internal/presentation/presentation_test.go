package presentation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResolvePresentationRequest(t *testing.T) {
	// Mock Credimi that returns a verification deeplink
	// Mock request_uri server that returns a JWT
	requestURIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwt")
		w.Write([]byte("eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiaXNzIjoiaXNzdWVyIn0.c2ln"))
	}))
	defer requestURIServer.Close()

	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`haip-vp://?request_uri=` + url.QueryEscape(requestURIServer.URL) + `&request_uri_method=get&client_id=test-client`))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-use-case", "raw", "auto", 30*time.Second)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s: %v", result.Status, result.Error)
	}
	if result.RequestURI != requestURIServer.URL {
		t.Errorf("unexpected request_uri: got %q want %q", result.RequestURI, requestURIServer.URL)
	}
	if result.RequestObject == nil {
		t.Fatal("expected request object")
	}
	if !result.RequestObject.SignaturePresent {
		t.Error("expected signature present")
	}
}

func TestResolvePresentationRequestPOSTAuto(t *testing.T) {
	// Server that rejects empty POST but accepts wallet_nonce
	attempts := 0
	requestURIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method == "POST" {
			body := make([]byte, 256)
			n, _ := r.Body.Read(body)
			if strings.Contains(string(body[:n]), "wallet_nonce") {
				w.WriteHeader(200)
				w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.sig"))
				return
			}
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.sig"))
	}))
	defer requestURIServer.Close()

	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`haip-vp://?request_uri=` + url.QueryEscape(requestURIServer.URL) + `&request_uri_method=post`))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-case", "raw", "auto", 30*time.Second)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s: %v", result.Status, result.Error)
	}
	if result.PostStrategy != "wallet_nonce" {
		t.Errorf("expected post_strategy wallet_nonce, got %s", result.PostStrategy)
	}
}

func TestResolvePresentationRequestMissingRequestURI(t *testing.T) {
	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`haip-vp://?client_id=test`))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-case", "raw", "auto", 30*time.Second)

	if result.Status != "error" {
		t.Errorf("expected status error, got %s", result.Status)
	}
}

func TestResolveCredimiFetchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	client := &http.Client{}
	result := Resolve(client, server.URL, "test-case", "raw", "auto", 30*time.Second)

	if result.Status != "error" {
		t.Errorf("expected status error, got %s", result.Status)
	}
}

func TestPOSTStrategiesAllTry(t *testing.T) {
	// All POST strategies fail
	requestURIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))
	defer requestURIServer.Close()

	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`haip-vp://?request_uri=` + url.QueryEscape(requestURIServer.URL) + `&request_uri_method=post`))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-case", "raw", "auto", 30*time.Second)

	if result.Status != "error" {
		t.Errorf("expected status error, got %s", result.Status)
	}
}

func TestExtractionErrorJSON(t *testing.T) {
	e := newExtractionError("test_code", "Test message", "A human readable message", "http://example.com", 410, true)
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !json.Valid(out) {
		t.Error("error output is not valid JSON")
	}
}

func TestStrategiesToTry(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"auto", 4},
		{"", 4}, // empty string falls through to default (auto) strategies
		{"empty", 1},
		{"wallet_nonce", 1},
		{"wallet_metadata_object", 1},
		{"wallet_metadata_empty_string", 1},
	}

	for _, tt := range tests {
		got := strategiesToTry(tt.input)
		if len(got) != tt.expected {
			t.Errorf("strategiesToTry(%q) returned %d strategies, want %d", tt.input, len(got), tt.expected)
		}
	}
}
