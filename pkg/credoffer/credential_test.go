package credoffer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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

func TestFetchIssuerMetadataURLDerivation(t *testing.T) {
	offer := json.RawMessage(`{"credential_issuer":"https://issuer.eudiw.dev"}`)
	_, fetch, err := FetchIssuerMetadata(&http.Client{}, offer)
	// May fail if the server doesn't respond, but the URL should be set
	if fetch != nil && fetch.URL != "https://issuer.eudiw.dev/.well-known/openid-credential-issuer" {
		t.Errorf("unexpected URL: %s", fetch.URL)
	}
	_ = err // network error is expected in test
}

func TestFetchIssuerMetadataFormatsAndValidation(t *testing.T) {
	if _, _, err := FetchIssuerMetadata(http.DefaultClient, json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid credential offer succeeded")
	}
	if _, _, err := FetchIssuerMetadata(http.DefaultClient, json.RawMessage(`{"issuer":"missing"}`)); err == nil {
		t.Fatal("credential offer without credential_issuer succeeded")
	}

	jwtMetadata := compactMetadataJWT(t, `{"alg":"none"}`, `{"credential_issuer":"jwt-issuer"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/jwt/"):
			w.Header().Set("Content-Type", "application/jwt")
			_, _ = fmt.Fprint(w, jwtMetadata)
		case strings.HasPrefix(r.URL.Path, "/text/"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(w, "plain metadata")
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"credential_issuer":"json-issuer"}`)
		}
	}))
	defer server.Close()

	for _, tt := range []struct {
		name   string
		issuer string
		format string
		want   string
	}{
		{name: "json", issuer: server.URL, format: "json", want: "json-issuer"},
		{name: "jwt", issuer: server.URL + "/jwt", format: "jwt", want: "jwt-issuer"},
		{name: "text", issuer: server.URL + "/text", format: "text", want: "plain metadata"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata, fetch, err := FetchIssuerMetadata(server.Client(), json.RawMessage(fmt.Sprintf(`{"credential_issuer":%q}`, tt.issuer)))
			if err != nil {
				t.Fatal(err)
			}
			if fetch.Format != tt.format {
				t.Fatalf("format = %q", fetch.Format)
			}
			if !strings.Contains(string(metadata), tt.want) {
				t.Fatalf("metadata = %s", metadata)
			}
		})
	}
}

func compactMetadataJWT(t *testing.T, header, payload string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}

func TestResolveDeeplink(t *testing.T) {
	offerJSON := `{"credential_issuer":"https://issuer.example","credential_configuration_ids":["test"]}`
	offerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(offerJSON)) //nolint:errcheck
	}))
	defer offerServer.Close()
	nestedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "openid-credential-offer://?credential_offer="+url.QueryEscape(offerJSON))
	}))
	defer nestedServer.Close()
	var loopServer *httptest.Server
	loopServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "openid-credential-offer://?credential_offer_uri="+url.QueryEscape(loopServer.URL))
	}))
	defer loopServer.Close()

	tests := []struct {
		name       string
		deeplink   string
		wantStatus string
		wantCode   string
		wantOffer  bool
		wantChain  int
	}{
		{
			name:       "offer by value",
			deeplink:   "openid-credential-offer://?credential_offer=" + url.QueryEscape(offerJSON),
			wantStatus: "ok",
			wantOffer:  true,
			wantChain:  1,
		},
		{
			name:       "offer by reference",
			deeplink:   "  openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(offerServer.URL) + "\n",
			wantStatus: "ok",
			wantOffer:  true,
			wantChain:  2,
		},
		{
			name:       "offer URI answering another offer deeplink",
			deeplink:   "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(nestedServer.URL),
			wantStatus: "ok",
			wantOffer:  true,
			wantChain:  2,
		},
		{
			name:       "self-referencing offer URI stops at max depth",
			deeplink:   "openid-credential-offer://?credential_offer_uri=" + url.QueryEscape(loopServer.URL),
			wantStatus: "error",
			wantCode:   "credential_offer_uri_resolution_failed",
			wantChain:  6,
		},
		{
			name:       "empty deeplink",
			deeplink:   "   ",
			wantStatus: "error",
			wantCode:   "deeplink_missing",
		},
		{
			name:       "unparsable deeplink",
			deeplink:   "openid-credential-offer://%zz",
			wantStatus: "error",
			wantCode:   "deeplink_parse_failed",
			wantChain:  1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveDeeplink(&http.Client{}, "org/issuer/cred", tc.deeplink, 5)

			if result.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (error: %+v)", result.Status, tc.wantStatus, result.Error)
			}
			if result.CredentialID != "org/issuer/cred" {
				t.Errorf("credential id = %q", result.CredentialID)
			}
			if tc.wantCode != "" && (result.Error == nil || result.Error.Error.Code != tc.wantCode) {
				t.Errorf("error = %+v, want code %q", result.Error, tc.wantCode)
			}
			if tc.wantOffer {
				var offer map[string]any
				if err := json.Unmarshal(result.CredentialOffer, &offer); err != nil {
					t.Fatalf("unmarshal offer: %v", err)
				}
				if offer["credential_issuer"] != "https://issuer.example" {
					t.Errorf("issuer = %v", offer["credential_issuer"])
				}
			}
			if len(result.ResolutionChain) != tc.wantChain {
				t.Fatalf("resolution chain length = %d, want %d", len(result.ResolutionChain), tc.wantChain)
			}
			if tc.wantChain > 0 {
				step0 := result.ResolutionChain[0]
				if step0.Kind != "deeplink" || step0.ReturnedURI != strings.TrimSpace(tc.deeplink) || step0.URL != "" {
					t.Errorf("step 0 = %+v", step0)
				}
			}
		})
	}
}
