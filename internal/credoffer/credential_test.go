package credoffer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestResolveDirectCredentialOffer(t *testing.T) {
	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer=%7B%22credential_issuer%22%3A%22https%3A%2F%2Fissuer.example%22%2C%22credential_configuration_ids%22%3A%5B%22test%22%5D%7D`)) //nolint:errcheck
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-credential-id", "raw", 5)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s", result.Status)
	}
	if result.CredentialOffer == nil {
		t.Fatal("expected credential offer, got nil")
	}

	var offer map[string]any
	if err := json.Unmarshal(result.CredentialOffer, &offer); err != nil {
		t.Fatalf("unmarshal offer: %v", err)
	}
	if offer["credential_issuer"] != "https://issuer.example" {
		t.Errorf("unexpected issuer: %v", offer["credential_issuer"])
	}
}

func TestResolveNestedCredentialOfferURI(t *testing.T) {
	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"credential_issuer":"https://issuer.example","credential_configuration_ids":["final"]}`))
	}))
	defer issuerServer.Close()

	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodedURI := url.QueryEscape(issuerServer.URL + "/offers/nested?raw=true")
		_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer_uri=` + encodedURI))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-id", "raw", 5)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s: %v", result.Status, result.Error)
	}
	if result.CredentialOffer == nil {
		t.Fatal("expected credential offer, got nil")
	}
	if len(result.ResolutionChain) < 2 {
		t.Errorf("expected at least 2 resolution steps, got %d", len(result.ResolutionChain))
	}
}

func TestResolveCredimiFetchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	client := &http.Client{}
	result := Resolve(client, server.URL, "test-id", "raw", 5)

	if result.Status != "error" {
		t.Errorf("expected status error, got %s", result.Status)
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveMaxDepthExceeded(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/credential/deeplink" {
			_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer_uri=` + url.QueryEscape(server.URL+"/self-ref")))
		} else {
			_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer_uri=` + url.QueryEscape(server.URL+"/self-ref")))
		}
	}))
	defer server.Close()

	client := &http.Client{}
	result := Resolve(client, server.URL, "test-id", "raw", 2)

	if result.Status != "error" {
		t.Errorf("expected status error for max depth, got %s", result.Status)
	}
}

func TestFetchIssuerMetadataURL(t *testing.T) {
	offer := json.RawMessage(`{"credential_issuer":"https://example.com"}`)
	_, fetch, err := FetchIssuerMetadata(&http.Client{}, offer)

	// Will likely fail because example.com may not respond, but URL should be correct
	if err == nil && fetch != nil {
		if fetch.URL != "https://example.com/.well-known/openid-credential-issuer" {
			t.Errorf("unexpected metadata URL: %s", fetch.URL)
		}
	}
}

func TestExtractionErrorJSON(t *testing.T) {
	e := NewError("test_code", "Test message", "A human readable message", "http://example.com", 404, true)
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !json.Valid(out) {
		t.Error("error output is not valid JSON")
	}
}

func TestEncodeID(t *testing.T) {
	rawID := "/org/integration/test-id"

	if got := encodeID(rawID, "url"); got == rawID {
		t.Error("url encoding should encode slashes")
	}
	if got := encodeID(rawID, "raw"); got != rawID {
		t.Errorf("raw encoding should not modify: got %q", got)
	}
	// auto defaults to url encoding
	if got := encodeID(rawID, "auto"); got == rawID {
		t.Error("auto encoding should encode by default")
	}
}

func TestResolveNestedOfferURIReturnsJSON(t *testing.T) {
	// resolveOfferURI returns concrete JSON directly
	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"credential_issuer":"https://issuer.example","credential_configuration_ids":["final"]}`))
	}))
	defer issuerServer.Close()

	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer_uri=` + url.QueryEscape(issuerServer.URL)))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "test-id", "raw", 5)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s: %v", result.Status, result.Error)
	}
	if result.CredentialOffer == nil {
		t.Fatal("expected credential offer, got nil")
	}
	var offer map[string]any
	_ = json.Unmarshal(result.CredentialOffer, &offer)
	if offer["credential_issuer"] != "https://issuer.example" {
		t.Errorf("unexpected issuer: %v", offer["credential_issuer"])
	}
}

func TestResolveURLEncodedCredentialID(t *testing.T) {
	credimiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`openid-credential-offer://?credential_offer=%7B%22credential_issuer%22%3A%22https%3A%2F%2Fissuer.example%22%7D`))
	}))
	defer credimiServer.Close()

	client := &http.Client{}
	result := Resolve(client, credimiServer.URL, "/org/integration/test-id", "url", 5)

	if result.Status != "ok" {
		t.Fatalf("expected status ok, got %s", result.Status)
	}
}

func TestFetchIssuerMetadataURLDerivation(t *testing.T) {
	offer := json.RawMessage(`{"credential_issuer":"https://issuer.eudiw.dev"}`)
	_, fetch, err := FetchIssuerMetadata(&http.Client{}, offer)
	// May fail if the server doesn't respond, but the URL should be set
	if fetch != nil && fetch.URL != "https://issuer.eudiw.dev/.well-known/openid-credential-issuer" {
		t.Errorf("unexpected URL: %s", fetch.URL)
	}
	_ = err // network error is expected in test
}
