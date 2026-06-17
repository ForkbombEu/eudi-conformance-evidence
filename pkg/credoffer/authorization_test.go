package credoffer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchAuthorizationServerMetadataUsesOfferSelectionAndOIDCFallback(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server/realms/test":
			http.NotFound(w, r)
		case "/realms/test/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q}`, server.URL+"/realms/test", server.URL+"/authorize")
		default:
			t.Fatalf("unexpected discovery path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	offer := json.RawMessage(fmt.Sprintf(`{
		"credential_issuer":"https://issuer.example",
		"grants":{"authorization_code":{"authorization_server":%q}}
	}`, server.URL+"/realms/test"))
	issuerMetadata := json.RawMessage(`{
		"credential_issuer":"https://issuer.example",
		"authorization_servers":["https://ignored.example"]
	}`)

	results, err := FetchAuthorizationServerMetadata(server.Client(), offer, issuerMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("authorization servers = %d", len(results))
	}
	result := results[0]
	if result.Source != "credential_offer" {
		t.Fatalf("source = %q", result.Source)
	}
	if len(result.Fetches) != 2 {
		t.Fatalf("fetch attempts = %d", len(result.Fetches))
	}
	if !strings.HasSuffix(result.Fetches[0].URL, "/.well-known/oauth-authorization-server/realms/test") {
		t.Fatalf("RFC 8414 URL = %q", result.Fetches[0].URL)
	}
	if !strings.HasSuffix(result.Fetches[1].URL, "/realms/test/.well-known/openid-configuration") {
		t.Fatalf("OIDC URL = %q", result.Fetches[1].URL)
	}
	if !strings.Contains(string(result.Metadata), "authorization_endpoint") {
		t.Fatalf("metadata = %s", result.Metadata)
	}
}

func TestFetchAuthorizationServerMetadataUsesAllIssuerMetadataServers(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server/one" && r.URL.Path != "/.well-known/oauth-authorization-server/two" {
			t.Fatalf("discovery path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer":%q}`, server.URL+strings.TrimPrefix(r.URL.Path, "/.well-known/oauth-authorization-server"))
	}))
	defer server.Close()

	offer := json.RawMessage(`{"credential_issuer":"https://issuer.example"}`)
	issuerMetadata := json.RawMessage(fmt.Sprintf(`{"credential_issuer":"https://issuer.example","authorization_servers":[%q,%q]}`, server.URL+"/one", server.URL+"/two"))
	results, err := FetchAuthorizationServerMetadata(server.Client(), offer, issuerMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Source != "issuer_metadata" || results[1].Source != "issuer_metadata" {
		t.Fatalf("results = %#v", results)
	}
}

func TestFetchAuthorizationServerMetadataUsesCredentialIssuerFallback(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			t.Fatalf("discovery path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q}`, server.URL, server.URL+"/token")
	}))
	defer server.Close()

	offer := json.RawMessage(fmt.Sprintf(`{"credential_issuer":%q}`, server.URL))
	issuerMetadata := json.RawMessage(fmt.Sprintf(`{"credential_issuer":%q}`, server.URL))
	results, err := FetchAuthorizationServerMetadata(server.Client(), offer, issuerMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Source != "credential_issuer" {
		t.Fatalf("results = %#v", results)
	}
	if len(results[0].Fetches) != 1 || results[0].Fetches[0].SHA256 == "" {
		t.Fatalf("fetch = %#v", results[0].Fetches)
	}
}
