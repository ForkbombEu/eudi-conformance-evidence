// Package jwt provides compact JWT/JWS decoding without signature verification.
package jwt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Token represents a decoded compact JWT/JWS.
type Token struct {
	Raw              string          `json:"raw"`
	Header           json.RawMessage `json:"header"`
	Payload          json.RawMessage `json:"payload"`
	Signature        string          `json:"signature,omitempty"`
	SignaturePresent bool            `json:"signature_present"`
	Format           string          `json:"format"`
}

// Decode decodes a compact JWT/JWS string without verifying the signature.
func Decode(raw string) (*Token, error) {
	raw = strings.TrimSpace(raw)

	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("jwt: expected at least 2 parts, got %d", len(parts))
	}

	headerBytes, err := base64urlDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("jwt: header decode: %w", err)
	}

	if !json.Valid(headerBytes) {
		return nil, fmt.Errorf("jwt: header is not valid JSON")
	}

	payloadBytes, err := base64urlDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("jwt: payload decode: %w", err)
	}

	if !json.Valid(payloadBytes) {
		return nil, fmt.Errorf("jwt: payload is not valid JSON")
	}

	t := &Token{
		Raw:     raw,
		Header:  json.RawMessage(headerBytes),
		Payload: json.RawMessage(payloadBytes),
		Format:  "jwt",
	}

	if len(parts) >= 3 && parts[2] != "" {
		t.Signature = parts[2]
		t.SignaturePresent = true
	}

	return t, nil
}

// LooksLikeJWT returns true if the string looks like a compact JWT/JWS.
func LooksLikeJWT(s string) bool {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return false
	}
	// Only validate header and payload parts (not signature)
	for _, p := range parts[:2] {
		if _, err := base64urlDecode(p); err != nil {
			return false
		}
	}
	return true
}

func base64urlDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
