package webui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
)

func TestIndexAndStaticAssets(t *testing.T) {
	handler := NewHandler(http.DefaultClient)

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d", index.Code)
	}
	if !strings.Contains(index.Body.String(), "EUDI Issuer/Verifier metatadata extractor") {
		t.Fatal("index did not contain extractor launcher")
	}
	if strings.Count(index.Body.String(), `class="card extractor-card"`) != 2 {
		t.Fatal("index did not contain exactly two extractor cards")
	}
	if !strings.Contains(index.Body.String(), `href="/static/credimi_logo.svg"`) {
		t.Fatal("index did not contain the Credimi favicon")
	}
	if !strings.Contains(index.Body.String(), `href="https://github.com/ForkbombEu/eudi-conformance-evidence/blob/main/README.md"`) ||
		!strings.Contains(index.Body.String(), `target="_blank"`) ||
		!strings.Contains(index.Body.String(), `rel="noopener noreferrer"`) {
		t.Fatal("index did not contain the GitHub README help link")
	}

	favicon := httptest.NewRecorder()
	handler.ServeHTTP(favicon, httptest.NewRequest(http.MethodGet, "/static/credimi_logo.svg", nil))
	if favicon.Code != http.StatusOK || !strings.Contains(favicon.Body.String(), "<svg") {
		t.Fatalf("favicon = %d %q", favicon.Code, favicon.Body.String())
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

	healthz := httptest.NewRecorder()
	handler.ServeHTTP(healthz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthz.Code != http.StatusOK || healthz.Body.String() != "ok\n" {
		t.Fatalf("healthz = %d %q", healthz.Code, healthz.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing route status = %d", missing.Code)
	}
}

func TestExtractCredentialOffer(t *testing.T) {
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-credential-issuer":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"credential_issuer":%q,"credential_endpoint":%q}`, serverURL(r), serverURL(r)+"/credential")
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q}`, serverURL(r), serverURL(r)+"/authorize")
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()

	offer := fmt.Sprintf(`{"credential_issuer":%q}`, issuer.URL)
	input := "openid-credential-offer://?credential_offer=" + url.QueryEscape(offer)
	response := postExtraction(t, issuer.Client(), "issuer-metadata", input)
	assertResultContains(t, response, "credential_endpoint")
	assertResultContains(t, response, `class="result-disclosure`)
	body := response.Body.String()
	if strings.Count(body, `class="metadata-result-box"`) != 3 {
		t.Fatalf("expected three metadata result boxes: %s", body)
	}
	issuerIndex := strings.Index(body, "Credential issuer metadata")
	authorizationIndex := strings.Index(body, "Authorization server metadata")
	detailsIndex := strings.Index(body, "Resolution details")
	if issuerIndex < 0 || authorizationIndex < 0 || detailsIndex < 0 || issuerIndex > authorizationIndex || authorizationIndex > detailsIndex {
		t.Fatalf("metadata result order was not issuer, authorization server, resolution details: %s", body)
	}
	assertResultContains(t, response, `data-copy-target="issuer-metadata-output"`)
	assertResultContains(t, response, `data-copy-target="authorization-server-output"`)
	assertResultContains(t, response, `data-copy-target="issuer-resolution-details-output"`)
	assertResultContains(t, response, "authorization_endpoint")
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
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q}`, mock.URL, mock.URL+"/token")
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	input := mock.URL + "/hub/credentials/org/integration/credential"
	response := postExtraction(t, mock.Client(), "issuer-metadata", input)
	assertResultContains(t, response, "credential_endpoint")
	assertResultContains(t, response, "token_endpoint")
}

func TestExtractPresentationRequest(t *testing.T) {
	input := `{"client_id":"verifier","dcql_query":{"credentials":[{"id":"pid"}]}}`
	response := postExtraction(t, http.DefaultClient, "presentation-metadata", input)
	assertResultContains(t, response, "credentials")
	assertResultContains(t, response, "pid")
	assertResultContains(t, response, `data-copy-target="presentation-output"`)
	assertResultContains(t, response, `data-copy-target="presentation-resolution-details-output"`)
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

func TestExtractRejectsInvalidFormInput(t *testing.T) {
	handler := NewHandler(http.DefaultClient)

	invalidForm := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader("%"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(invalidForm, request)
	if invalidForm.Code != http.StatusBadRequest {
		t.Fatalf("invalid form status = %d", invalidForm.Code)
	}

	missingInput := postExtraction(t, http.DefaultClient, "issuer-metadata", "")
	if missingInput.Code != http.StatusBadRequest {
		t.Fatalf("missing input status = %d", missingInput.Code)
	}

	unknownKind := postExtraction(t, http.DefaultClient, "other", "{}")
	if unknownKind.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown kind status = %d", unknownKind.Code)
	}
}

func TestResolveCredentialOfferNestedURI(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			next := "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(server.URL+"/second")
			_, _ = fmt.Fprint(w, next)
		case "/second":
			_, _ = fmt.Fprint(w, `{"credential_issuer":"https://issuer.example"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	input := "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(server.URL+"/first")
	offer, chain, err := resolveCredentialOffer(server.Client(), input, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(offer) != `{"credential_issuer":"https://issuer.example"}` {
		t.Fatalf("offer = %s", offer)
	}
	if len(chain) != 2 {
		t.Fatalf("chain length = %d", len(chain))
	}
}

func TestResolveCredentialOfferRejectsMalformedInputs(t *testing.T) {
	cases := []string{
		"not a uri",
		"openid-credential-offer://?credential_offer=%7B",
		"openid-credential-offer://?issuer=missing-offer",
	}
	for _, input := range cases {
		if _, _, err := resolveCredentialOffer(http.DefaultClient, input, 0); err == nil {
			t.Fatalf("resolveCredentialOffer(%q) succeeded", input)
		}
	}
	if _, _, err := resolveCredentialOffer(http.DefaultClient, "openid-credential-offer://?credential_offer_uri=https://example.test", 6); err == nil {
		t.Fatal("resolveCredentialOffer over max depth succeeded")
	}
}

func TestResolvePresentationRequestVariants(t *testing.T) {
	requestJWT := compactJWT(t, `{"alg":"none"}`, `{"dcql_query":{"credentials":[{"id":"nested"}]}}`)
	direct := "openid4vp://authorize?request=" + url.QueryEscape(requestJWT)
	payload, details, err := resolvePresentationRequest(http.DefaultClient, direct)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "nested") {
		t.Fatalf("payload = %s", payload)
	}
	if details == nil {
		t.Fatal("expected decoded JWT details")
	}

	if _, _, err := resolvePresentationRequest(http.DefaultClient, "openid4vp://authorize"); err == nil {
		t.Fatal("presentation request without request parameters succeeded")
	}
}

func TestFetchPresentationObjectPOSTRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if attempts < 3 {
			http.Error(w, "try again", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/jwt")
		_, _ = fmt.Fprint(w, "header.payload.signature")
	}))
	defer server.Close()

	body, status, contentType, err := fetchPresentationObject(server.Client(), http.MethodPost, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || status != http.StatusOK || contentType != "application/jwt" || string(body) != "header.payload.signature" {
		t.Fatalf("attempts=%d status=%d contentType=%q body=%q", attempts, status, contentType, body)
	}
}

func TestFetchLimitedErrors(t *testing.T) {
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusTeapot)
	}))
	defer errorServer.Close()
	if _, status, _, err := fetchLimited(errorServer.Client(), http.MethodGet, errorServer.URL, nil); err == nil || status != http.StatusTeapot {
		t.Fatalf("HTTP error status=%d err=%v", status, err)
	}

	largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	defer largeServer.Close()
	if _, _, _, err := fetchLimited(largeServer.Client(), http.MethodGet, largeServer.URL, nil); err == nil {
		t.Fatal("large response succeeded")
	}

	if _, _, _, err := fetchLimited(http.DefaultClient, http.MethodGet, "://bad-url", nil); err == nil {
		t.Fatal("bad URL succeeded")
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

	for _, input := range []string{
		"not-url",
		"https://credimi.io/hub/use_cases_verifications/org/integration/use-case",
		"https://credimi.io/hub/credentials/%zz",
		"https://credimi.io/hub/credentials/",
	} {
		if _, _, err := parseCredimiHubURL(input, "credentials"); err == nil {
			t.Fatalf("parseCredimiHubURL(%q) succeeded", input)
		}
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

func TestJSONHelpers(t *testing.T) {
	dcql, err := findDCQL(json.RawMessage(`{"outer":[{"dcql-query":{"credentials":[{"id":"pid"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(dcql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "pid") {
		t.Fatalf("dcql = %s", encoded)
	}

	if value := rawJSONValue(json.RawMessage(`not-json`)); value != "not-json" {
		t.Fatalf("rawJSONValue = %#v", value)
	}

	rendered, err := prettyJSON(map[string]string{"html": "<tag>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "<tag>") {
		t.Fatalf("prettyJSON escaped HTML: %s", rendered)
	}
}

func TestExtractionErrorHelpers(t *testing.T) {
	if err := extractionError(nil); err == nil || err.Error() != "credential extraction failed" {
		t.Fatalf("nil extraction error = %v", err)
	}
	credErr := extractionError(credofferError("credential failed"))
	if credErr == nil || credErr.Error() != "credential failed" {
		t.Fatalf("credential extraction error = %v", credErr)
	}

	if err := presentationError(nil); err == nil || err.Error() != "presentation extraction failed" {
		t.Fatalf("nil presentation error = %v", err)
	}
	presErr := presentationError(presentationExtractionError("presentation failed"))
	if presErr == nil || presErr.Error() != "presentation failed" {
		t.Fatalf("presentation extraction error = %v", presErr)
	}
}

func TestSafeHTTPClientValidation(t *testing.T) {
	client := newSafeHTTPClient(time.Second)
	redirect, err := http.NewRequest(http.MethodGet, "ftp://example.com/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(redirect, nil); err == nil {
		t.Fatal("unsupported redirect scheme succeeded")
	}

	if err := validateRemoteURL(&url.URL{Scheme: "https"}); err == nil {
		t.Fatal("remote URL without host succeeded")
	}
	if _, err := safeDialContext(t.Context(), "tcp", "missing-port"); err == nil {
		t.Fatal("safeDialContext without port succeeded")
	}
	if _, err := safeDialContext(t.Context(), "tcp4", "localhost:1"); err == nil {
		t.Fatal("safeDialContext accepted localhost")
	}
}

func TestRunRejectsInvalidFlag(t *testing.T) {
	if err := Run([]string{"-bad"}); err == nil {
		t.Fatal("Run accepted an invalid flag")
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

func credofferError(message string) *credoffer.ExtractionError {
	err := &credoffer.ExtractionError{}
	err.Error.Message = message
	return err
}

func presentationExtractionError(message string) *presentation.ExtractionError {
	err := &presentation.ExtractionError{}
	err.Error.Message = message
	return err
}
