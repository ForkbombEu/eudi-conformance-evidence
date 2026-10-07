package credimi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const offerJSON = `{"credential_issuer":"https://issuer.example"}`

func TestDeeplinkURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		encoding string
		want     string
	}{
		{"url escapes the id", "https://credimi.io/", "url", "https://credimi.io/api/credential/deeplink?id=%2Forg%2Fissuer%2Fcred"},
		{"auto escapes the id", "https://credimi.io", "auto", "https://credimi.io/api/credential/deeplink?id=%2Forg%2Fissuer%2Fcred"},
		{"raw keeps the id", "https://credimi.io", "raw", "https://credimi.io/api/credential/deeplink?id=/org/issuer/cred"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeeplinkURL(tc.baseURL, "credential", "/org/issuer/cred", tc.encoding); got != tc.want {
				t.Errorf("DeeplinkURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// newCredimi serves the Credimi deeplink routes. It answers status with body
// and records the path and id of the last request.
func newCredimi(t *testing.T, status int, body string) (*httptest.Server, *url.URL) {
	t.Helper()
	var last url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = *r.URL
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &last
}

func TestResolveCredential(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus string
		wantCode   string
		wantHTTP   int
		wantChain  int
	}{
		{
			name:       "deeplink resolved",
			status:     http.StatusOK,
			body:       "openid-credential-offer://?credential_offer=" + url.QueryEscape(offerJSON) + "\n",
			wantStatus: "ok",
			wantChain:  1,
		},
		{
			name:       "Credimi answers an error",
			status:     http.StatusNotFound,
			body:       "not found",
			wantStatus: "error",
			wantCode:   "credimi_deeplink_fetch_failed",
			wantHTTP:   http.StatusNotFound,
			wantChain:  1,
		},
		{
			name:       "unparsable deeplink",
			status:     http.StatusOK,
			body:       "openid-credential-offer://%zz",
			wantStatus: "error",
			wantCode:   "deeplink_parse_failed",
			wantChain:  1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, last := newCredimi(t, tc.status, tc.body)

			r := ResolveCredential(server.Client(), server.URL, "org/issuer/cred", "url", 5)

			if r.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (error: %+v)", r.Status, tc.wantStatus, r.Error)
			}
			if r.CredentialID != "org/issuer/cred" {
				t.Errorf("credential id = %q", r.CredentialID)
			}
			if last.Path != "/api/credential/deeplink" || last.Query().Get("id") != "org/issuer/cred" {
				t.Errorf("request = %s", last.String())
			}
			if tc.wantCode != "" && (r.Error == nil || r.Error.Error.Code != tc.wantCode || r.Error.Error.HTTPStatus != tc.wantHTTP) {
				t.Errorf("error = %+v, want code %q status %d", r.Error, tc.wantCode, tc.wantHTTP)
			}
			if tc.wantStatus == "ok" && string(r.CredentialOffer) != offerJSON {
				t.Errorf("offer = %s", r.CredentialOffer)
			}
			if len(r.ResolutionChain) != tc.wantChain {
				t.Fatalf("chain length = %d, want %d", len(r.ResolutionChain), tc.wantChain)
			}
			step0 := r.ResolutionChain[0]
			wantURL := server.URL + "/api/credential/deeplink?id=org%2Fissuer%2Fcred"
			if step0.URL != wantURL || step0.HTTPStatus != tc.status || step0.ReturnedPayloadType != "text/plain" || step0.ReturnedPayloadSHA256 == "" {
				t.Errorf("step 0 = %+v", step0)
			}
		})
	}
}

func TestResolveCredentialUnreachable(t *testing.T) {
	server, _ := newCredimi(t, http.StatusOK, "")
	server.Close()

	r := ResolveCredential(&http.Client{}, server.URL, "org/issuer/cred", "url", 5)

	if r.Status != "error" || r.Error == nil || r.Error.Error.Code != "credimi_deeplink_fetch_failed" || r.Error.Error.HTTPStatus != 0 {
		t.Fatalf("result = %+v error = %+v", r, r.Error)
	}
	if len(r.ResolutionChain) != 0 {
		t.Errorf("chain = %+v", r.ResolutionChain)
	}
}

func TestResolveVerification(t *testing.T) {
	requestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwt")
		_, _ = fmt.Fprint(w, "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.c2ln")
	}))
	defer requestServer.Close()

	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus string
		wantCode   string
		wantHTTP   int
	}{
		{
			name:       "deeplink resolved",
			status:     http.StatusOK,
			body:       "haip-vp://?request_uri=" + url.QueryEscape(requestServer.URL),
			wantStatus: "ok",
		},
		{
			name:       "Credimi answers an error",
			status:     http.StatusInternalServerError,
			body:       "boom",
			wantStatus: "error",
			wantCode:   "verification_deeplink_fetch_failed",
			wantHTTP:   http.StatusInternalServerError,
		},
		{
			name:       "deeplink without request_uri",
			status:     http.StatusOK,
			body:       "haip-vp://?client_id=test",
			wantStatus: "error",
			wantCode:   "request_uri_missing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, last := newCredimi(t, tc.status, tc.body)

			r := ResolveVerification(server.Client(), server.URL, "org/verifier/use-case", "raw", "auto")

			if r.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (error: %+v)", r.Status, tc.wantStatus, r.Error)
			}
			if r.UseCaseID != "org/verifier/use-case" || r.PostStrategy != "auto" {
				t.Errorf("use case = %q, strategy = %q", r.UseCaseID, r.PostStrategy)
			}
			if last.Path != "/api/verification/deeplink" || last.Query().Get("id") != "org/verifier/use-case" {
				t.Errorf("request = %s", last.String())
			}
			if tc.wantCode != "" && (r.Error == nil || r.Error.Error.Code != tc.wantCode || r.Error.Error.HTTPStatus != tc.wantHTTP) {
				t.Errorf("error = %+v, want code %q status %d", r.Error, tc.wantCode, tc.wantHTTP)
			}
			if tc.wantStatus == "ok" && (r.RequestObject == nil || r.RequestURI != requestServer.URL) {
				t.Errorf("request object = %+v, request_uri = %q", r.RequestObject, r.RequestURI)
			}
		})
	}
}
