// Package presentation resolves presentation requests from Credimi verification deeplinks.
package presentation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/jwt"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

// RequestURIFetch records how the request_uri was fetched.
type RequestURIFetch struct {
	URL         string `json:"url"`
	HTTPStatus  int    `json:"http_status"`
	ContentType string `json:"content_type"`
	FetchedAt   string `json:"fetched_at"`
	Format      string `json:"format"`
	SHA256      string `json:"sha256"`
}

// ExtractionError represents a recoverable extraction error.
type ExtractionError struct {
	Status string `json:"status"`
	Error  struct {
		Code         string `json:"code"`
		Message      string `json:"message"`
		HumanMessage string `json:"human_message"`
		URL          string `json:"url,omitempty"`
		HTTPStatus   int    `json:"http_status,omitempty"`
		Recoverable  bool   `json:"recoverable"`
	} `json:"error"`
}

// Result holds the full presentation-request extraction result.
type Result struct {
	Status           string `json:"status"`
	StepID           string `json:"step_id"`
	UseCaseID        string `json:"use_case_id"`
	DeeplinkURI      string
	RequestURI       string
	RequestURIMethod string
	PostStrategy     string
	RequestObject    *jwt.Token
	RequestURIRaw    string
	RequestURIFetch  *RequestURIFetch
	Error            *ExtractionError
}

// postStrategies in order for auto mode
var postStrategies = []string{"empty", "wallet_nonce", "wallet_metadata_object", "wallet_metadata_empty_string"}

// Resolve resolves a presentation request from a Credimi verification deeplink.
func Resolve(client *http.Client, credimiBaseURL, useCaseID, idEncoding, postStrategy string, timeout time.Duration) *Result {
	r := &Result{
		Status:       "ok",
		UseCaseID:    useCaseID,
		PostStrategy: postStrategy,
	}

	credimiURL := buildCredimiURL(credimiBaseURL, "verification", useCaseID, idEncoding)

	// Step 1: fetch Credimi verification deeplink
	body, err := httpGet(client, credimiURL)
	if err != nil {
		r.Status = "error"
		r.Error = newExtractionError("verification_deeplink_fetch_failed", "Could not fetch verification deeplink",
			"The verification deeplink could not be fetched from Credimi.", credimiURL, 0, true)
		return r
	}
	r.DeeplinkURI = strings.TrimSpace(body)

	// Parse URI to extract request_uri and request_uri_method
	parsed, err := url.Parse(r.DeeplinkURI)
	if err != nil {
		r.Status = "error"
		r.Error = newExtractionError("deeplink_parse_failed", "Could not parse verification deeplink URI",
			"The verification deeplink URI could not be parsed.", r.DeeplinkURI, 0, true)
		return r
	}

	query := parsed.Query()
	r.RequestURI = query.Get("request_uri")
	if r.RequestURI == "" {
		r.Status = "error"
		r.Error = newExtractionError("request_uri_missing", "No request_uri in verification deeplink",
			"The verification deeplink does not contain a request_uri parameter.", r.DeeplinkURI, 0, true)
		return r
	}

	r.RequestURIMethod = query.Get("request_uri_method")
	if r.RequestURIMethod == "" {
		r.RequestURIMethod = "get"
	}

	// Fetch request_uri
	if r.RequestURIMethod == "post" {
		if err := r.fetchRequestURIPost(client, r.RequestURI, postStrategy); err != nil {
			r.Status = "error"
			r.Error = newExtractionError("request_uri_post_failed", err.Error(),
				"Could not fetch the request_uri via POST.", r.RequestURI, 0, true)
		}
	} else {
		if err := r.fetchRequestURIGet(client, r.RequestURI); err != nil {
			r.Status = "error"
			r.Error = newExtractionError("request_uri_get_failed", err.Error(),
				"Could not fetch the request_uri via GET.", r.RequestURI, 0, true)
		}
	}

	return r
}

func (r *Result) fetchRequestURIGet(client *http.Client, uri string) error {
	body, err := httpGet(client, uri)
	if err != nil {
		return err
	}
	return r.processResponse(uri, body)
}

func (r *Result) fetchRequestURIPost(client *http.Client, uri, strategy string) error {
	strategies := strategiesToTry(strategy)
	var lastErr error

	for _, s := range strategies {
		body, err := r.tryPostStrategy(client, uri, s)
		if err == nil {
			r.PostStrategy = s
			return r.processResponse(uri, body)
		}
		lastErr = err
	}
	return fmt.Errorf("all POST strategies failed: %w", lastErr)
}

func (r *Result) tryPostStrategy(client *http.Client, uri, strategy string) (string, error) {
	var reqBody io.Reader
	switch strategy {
	case "empty":
		// No body
	case "wallet_nonce":
		nonce := fmt.Sprintf("%x", sha256.Sum256([]byte(time.Now().String())))[:16]
		reqBody = strings.NewReader(fmt.Sprintf(`{"wallet_nonce":"%s"}`, nonce))
	case "wallet_metadata_object":
		reqBody = strings.NewReader(`{"wallet_metadata":{}}`)
	case "wallet_metadata_empty_string":
		reqBody = strings.NewReader(`{"wallet_metadata":""}`)
	default:
		return "", fmt.Errorf("unknown POST strategy: %s", strategy)
	}

	httpReq, err := http.NewRequest("POST", uri, reqBody)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp *http.Response
	var body []byte

	err = telemetry.TraceHTTP(context.Background(), "POST", uri, func() (int, error) {
		var doErr error
		resp, doErr = client.Do(httpReq)
		if doErr != nil {
			return 0, doErr
		}
		defer resp.Body.Close() //nolint:errcheck

		body, doErr = io.ReadAll(resp.Body)
		if doErr != nil {
			return resp.StatusCode, doErr
		}
		return resp.StatusCode, nil
	})
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("POST returned %d", resp.StatusCode)
	}

	return string(body), nil
}

func (r *Result) processResponse(uri, body string) error {
	r.RequestURIRaw = body
	r.RequestURIFetch = &RequestURIFetch{
		URL:        uri,
		HTTPStatus: 200,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		SHA256:     fmt.Sprintf("%x", sha256.Sum256([]byte(body))),
	}

	if jwt.LooksLikeJWT(body) {
		r.RequestURIFetch.Format = "jwt"
		token, err := jwt.Decode(body)
		if err != nil {
			return fmt.Errorf("decode JWT request object: %w", err)
		}
		r.RequestObject = token
	} else if json.Valid([]byte(body)) {
		r.RequestURIFetch.Format = "json"
	} else {
		r.RequestURIFetch.Format = "text"
	}

	return nil
}

func strategiesToTry(strategy string) []string {
	if strategy != "" && strategy != "auto" {
		return []string{strategy}
	}
	return postStrategies
}

func httpGet(client *http.Client, rawURL string) (string, error) {
	var resp *http.Response
	var body []byte

	err := telemetry.TraceHTTP(context.Background(), "GET", rawURL, func() (int, error) {
		var fetchErr error
		resp, fetchErr = client.Get(rawURL)
		if fetchErr != nil {
			return 0, fetchErr
		}
		defer resp.Body.Close() //nolint:errcheck

		body, fetchErr = io.ReadAll(resp.Body)
		if fetchErr != nil {
			return resp.StatusCode, fetchErr
		}
		return resp.StatusCode, nil
	})
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

func buildCredimiURL(baseURL, kind, id, encoding string) string {
	base := strings.TrimSuffix(baseURL, "/")
	encodedID := encodeID(id, encoding)
	return fmt.Sprintf("%s/api/%s/deeplink?id=%s", base, kind, encodedID)
}

func encodeID(id, encoding string) string {
	switch encoding {
	case "url":
		return url.QueryEscape(id)
	case "raw":
		return id
	default:
		return url.QueryEscape(id)
	}
}

func newExtractionError(code, message, humanMessage, url string, httpStatus int, recoverable bool) *ExtractionError {
	e := &ExtractionError{Status: "error"}
	e.Error.Code = code
	e.Error.Message = message
	e.Error.HumanMessage = humanMessage
	e.Error.URL = url
	e.Error.HTTPStatus = httpStatus
	e.Error.Recoverable = recoverable
	return e
}
