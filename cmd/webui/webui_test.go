package webui

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIndexAndStaticAssets(t *testing.T) {
	handler := NewHandler(http.DefaultClient)

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d", index.Code)
	}
	if !strings.Contains(index.Body.String(), "Extract .well-known and DCQL") {
		t.Fatal("index did not contain extractor launcher")
	}
	if strings.Count(index.Body.String(), `class="card extractor-card"`) != 2 {
		t.Fatal("index did not contain exactly two extractor cards")
	}

	stylesheet := httptest.NewRecorder()
	handler.ServeHTTP(stylesheet, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	if stylesheet.Code != http.StatusOK {
		t.Fatalf("stylesheet status = %d", stylesheet.Code)
	}
	if !strings.Contains(stylesheet.Body.String(), "--brand-primary") {
		t.Fatal("stylesheet did not contain design tokens")
	}

	script := httptest.NewRecorder()
	handler.ServeHTTP(script, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))
	if script.Code != http.StatusOK {
		t.Fatalf("script status = %d", script.Code)
	}
	if !strings.Contains(script.Body.String(), "EUDI Evidence") {
		t.Fatal("script did not contain console signature")
	}
}

func TestExtractCredentialOffer(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-credential-issuer" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"credential_issuer":%q,"credential_endpoint":%q}`, serverURL(r), serverURL(r)+"/credential")
	}))
	defer issuer.Close()

	offer := fmt.Sprintf(`{"credential_issuer":%q}`, issuer.URL)
	input := "openid-credential-offer://?credential_offer=" + url.QueryEscape(offer)
	response := postExtraction(t, issuer.Client(), "issuer-metadata", input)
	assertResultContains(t, response, "credential_endpoint")
	assertResultContains(t, response, `class="result-disclosure`)
}

func TestExtractCredimiCredential(t *testing.T) {
	var mock *httptest.Server
	mock = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/credential/deeplink":
			if r.URL.Query().Get("id") != "/org/integration/credential" {
				t.Errorf("credential id = %q", r.URL.Query().Get("id"))
			}
			offer := fmt.Sprintf(`{"credential_issuer":%q}`, mock.URL)
			_, _ = fmt.Fprint(w, "openid-credential-offer://?credential_offer="+url.QueryEscape(offer))
		case "/.well-known/openid-credential-issuer":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"credential_issuer":%q,"credential_endpoint":%q}`, mock.URL, mock.URL+"/credential")
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	input := mock.URL + "/hub/credentials/org/integration/credential"
	response := postExtraction(t, mock.Client(), "issuer-metadata", input)
	assertResultContains(t, response, "credential_endpoint")
}

func TestExtractPresentationRequest(t *testing.T) {
	input := `{"client_id":"verifier","dcql_query":{"credentials":[{"id":"pid"}]}}`
	response := postExtraction(t, http.DefaultClient, "presentation-metadata", input)
	assertResultContains(t, response, "credentials")
	assertResultContains(t, response, "pid")
}

func TestExtractPresentationRequestURI(t *testing.T) {
	requestObject := compactJWT(t, `{"alg":"none"}`, `{"dcql_query":{"credentials":[{"id":"pid-from-uri"}]}}`)
	requestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, requestObject)
	}))
	defer requestServer.Close()

	input := "openid4vp://authorize?request_uri=" + url.QueryEscape(requestServer.URL)
	response := postExtraction(t, requestServer.Client(), "presentation-metadata", input)
	assertResultContains(t, response, "pid-from-uri")
}

func TestExtractCredimiVerification(t *testing.T) {
	requestObject := compactJWT(t, `{"alg":"none"}`, `{"client_id":"verifier","dcql_query":{"credentials":[{"id":"pid"}]}}`)
	var mock *httptest.Server
	mock = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/verification/deeplink":
			if r.URL.Query().Get("id") != "/org/integration/use-case" {
				t.Errorf("use case id = %q", r.URL.Query().Get("id"))
			}
			_, _ = fmt.Fprint(w, "openid4vp://authorize?request_uri="+url.QueryEscape(mock.URL+"/request.jwt"))
		case "/request.jwt":
			_, _ = fmt.Fprint(w, requestObject)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	input := mock.URL + "/hub/use_cases_verifications/org/integration/use-case"
	response := postExtraction(t, mock.Client(), "presentation-metadata", input)
	assertResultContains(t, response, "credentials")
	assertResultContains(t, response, "pid")
}

func TestExtractRejectsMissingDCQL(t *testing.T) {
	response := postExtraction(t, http.DefaultClient, "presentation-metadata", `{"client_id":"verifier"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(response.Body.String(), "no dcql_query") {
		t.Fatalf("error result did not explain the missing DCQL query: %s", response.Body.String())
	}
}

func TestParseCredimiHubURL(t *testing.T) {
	base, id, err := parseCredimiHubURL("https://credimi.io/hub/credentials/org/integration/credential", "credentials")
	if err != nil {
		t.Fatal(err)
	}
	if base != "https://credimi.io" || id != "/org/integration/credential" {
		t.Fatalf("base = %q, id = %q", base, id)
	}
}

func TestIsCredimiHubURL(t *testing.T) {
	if !isCredimiHubURL("https://credimi.io/hub/credentials/org/integration/credential", "credentials") {
		t.Fatal("credential URL was not recognized")
	}
	if isCredimiHubURL("https://credimi.io/hub/use_cases_verifications/org/integration/use-case", "credentials") {
		t.Fatal("verification URL was recognized as a credential URL")
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Errorf("%s unexpectedly considered public", raw)
		}
	}
	if !isPublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address unexpectedly rejected")
	}
}

func postExtraction(t *testing.T, client *http.Client, kind, input string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"kind": {kind}, "input": {input}}
	request := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	NewHandler(client).ServeHTTP(response, request)
	return response
}

func assertResultContains(t *testing.T, response *httptest.ResponseRecorder, expected string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), expected) {
		t.Fatalf("result does not contain %q: %s", expected, response.Body.String())
	}
}

func compactJWT(t *testing.T, header, payload string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
