package presentation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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
				_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.sig"))
				return
			}
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.sig"))
	}))
	defer requestURIServer.Close()

	client := &http.Client{}
	deeplink := `haip-vp://?request_uri=` + url.QueryEscape(requestURIServer.URL) + `&request_uri_method=post`
	result := ResolveDeeplink(client, "test-case", deeplink, "auto")
	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s: %v", result.Status, result.Error)
	}
	if result.PostStrategy != "wallet_nonce" {
		t.Errorf("expected post_strategy wallet_nonce, got %s", result.PostStrategy)
	}
}

func TestPOSTStrategiesAllTry(t *testing.T) {
	// All POST strategies fail
	requestURIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))
	defer requestURIServer.Close()

	client := &http.Client{}
	deeplink := `haip-vp://?request_uri=` + url.QueryEscape(requestURIServer.URL) + `&request_uri_method=post`
	result := ResolveDeeplink(client, "test-case", deeplink, "auto")

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

func TestResolveDeeplink(t *testing.T) {
	requestURIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwt")
		_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiaXNzIjoiaXNzdWVyIn0.c2ln"))
	}))
	defer requestURIServer.Close()

	tests := []struct {
		name          string
		deeplink      string
		wantStatus    string
		wantCode      string
		wantJWT       bool
		wantMethodGET bool
	}{
		{
			name:          "request_uri defaults to GET",
			deeplink:      " haip-vp://?request_uri=" + url.QueryEscape(requestURIServer.URL) + "&client_id=test-client\n",
			wantStatus:    "ok",
			wantJWT:       true,
			wantMethodGET: true,
		},
		{
			name:       "empty deeplink",
			deeplink:   "",
			wantStatus: "error",
			wantCode:   "deeplink_missing",
		},
		{
			name:       "missing request_uri",
			deeplink:   "haip-vp://?client_id=test",
			wantStatus: "error",
			wantCode:   "request_uri_missing",
		},
		{
			name:       "unparsable deeplink",
			deeplink:   "haip-vp://%zz",
			wantStatus: "error",
			wantCode:   "deeplink_parse_failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveDeeplink(&http.Client{}, "org/verifier/use-case", tc.deeplink, "auto")

			if result.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (error: %+v)", result.Status, tc.wantStatus, result.Error)
			}
			if result.UseCaseID != "org/verifier/use-case" {
				t.Errorf("use case id = %q", result.UseCaseID)
			}
			if tc.wantCode != "" && (result.Error == nil || result.Error.Error.Code != tc.wantCode) {
				t.Errorf("error = %+v, want code %q", result.Error, tc.wantCode)
			}
			if tc.wantJWT && (result.RequestObject == nil || !result.RequestObject.SignaturePresent) {
				t.Errorf("expected a signed request object, got %+v", result.RequestObject)
			}
			if tc.wantMethodGET && result.RequestURIMethod != "get" {
				t.Errorf("request_uri_method = %q, want get", result.RequestURIMethod)
			}
			if result.DeeplinkURI != strings.TrimSpace(tc.deeplink) {
				t.Errorf("deeplink = %q, want trimmed input", result.DeeplinkURI)
			}
		})
	}
}
