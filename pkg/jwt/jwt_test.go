package jwt

import (
	"encoding/json"
	"testing"
)

func TestDecode(t *testing.T) {
	// A test JWT: header={"alg":"ES256","typ":"JWT"}, payload={"sub":"test","iss":"issuer"}
	raw := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiaXNzIjoiaXNzdWVyIn0.signature"

	token, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if token.Format != "jwt" {
		t.Errorf("expected format jwt, got %s", token.Format)
	}
	if !token.SignaturePresent {
		t.Error("expected signature_present to be true")
	}
	if token.Signature != "signature" {
		t.Errorf("expected signature 'signature', got %s", token.Signature)
	}

	var header map[string]any
	if err := json.Unmarshal(token.Header, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "ES256" {
		t.Errorf("expected alg ES256, got %v", header["alg"])
	}

	var payload map[string]any
	if err := json.Unmarshal(token.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["sub"] != "test" {
		t.Errorf("expected sub test, got %v", payload["sub"])
	}
}

func TestDecodeWithoutSignature(t *testing.T) {
	raw := "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0"

	token, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if token.SignaturePresent {
		t.Error("expected signature_present to be false")
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode("not-a-jwt")
	if err == nil {
		t.Error("expected error for invalid JWT")
	}
}

func TestDecodeTooFewParts(t *testing.T) {
	_, err := Decode("only.one")
	if err == nil {
		t.Error("expected error")
	}
}

func TestLooksLikeJWT(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.sig", true},
		{"eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0", true},
		{"not-a-jwt", false},
		{"", false},
		{"a.b.c.d.e.f", false}, // too many parts
	}

	for _, tt := range tests {
		got := LooksLikeJWT(tt.input)
		if got != tt.expected {
			t.Errorf("LooksLikeJWT(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestDecodePreservesRaw(t *testing.T) {
	raw := "eyJhIjoiMSJ9.eyJiIjoiMiJ9.sig"
	token, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if token.Raw != raw {
		t.Errorf("expected raw %q, got %q", raw, token.Raw)
	}
}
