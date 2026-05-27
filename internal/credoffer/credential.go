// Package credential resolves credential offers from Credimi deeplinks.
package credoffer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/internal/jwt"
)

// ResolutionStep records one step in the credential-offer resolution chain.
type ResolutionStep struct {
	Depth                 int    `json:"depth"`
	Kind                  string `json:"kind"`
	URL                   string `json:"url"`
	HTTPStatus            int    `json:"http_status"`
	ReturnedURI           string `json:"returned_uri,omitempty"`
	ReturnedPayloadType   string `json:"returned_payload_type,omitempty"`
	ReturnedPayloadSHA256 string `json:"returned_payload_sha256,omitempty"`
}

// MetadataFetch records how issuer metadata was fetched.
type MetadataFetch struct {
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

// Result holds the full credential-offer extraction result.
type Result struct {
	Status              string           `json:"status"`
	StepID              string           `json:"step_id"`
	CredentialID        string           `json:"credential_id"`
	ResolutionChain     []ResolutionStep `json:"resolution_chain"`
	FinalOfferPath      string           `json:"final_credential_offer_path,omitempty"`
	DeeplinkURI         string
	CredentialOffer     json.RawMessage
	IssuerMetadata      json.RawMessage
	IssuerMetadataFetch *MetadataFetch
	Error               *ExtractionError
}

// Resolve resolves a credential offer from a Credimi deeplink.
func Resolve(client *http.Client, credimiBaseURL, credentialID, idEncoding string, maxDepth int) *Result {
	r := &Result{
		Status:       "ok",
		CredentialID: credentialID,
	}

	credimiURL := buildCredimiURL(credimiBaseURL, "credential", credentialID, idEncoding)

	// Step 0: fetch Credimi deeplink
	step0, body, err := fetchURL(client, credimiURL)
	if err != nil {
		r.Status = "error"
		r.Error = NewError("credimi_deeplink_fetch_failed", "Could not fetch Credimi deeplink",
			"The Credimi deeplink could not be fetched.", credimiURL, 0, true)
		return r
	}
	if step0.HTTPStatus >= 400 {
		r.ResolutionChain = append(r.ResolutionChain, step0)
		r.Status = "error"
		r.Error = NewError("credimi_deeplink_fetch_failed", fmt.Sprintf("Credimi returned HTTP %d", step0.HTTPStatus),
			"The Credimi deeplink endpoint returned an error.", credimiURL, step0.HTTPStatus, true)
		return r
	}
	step0.Depth = 0
	step0.Kind = "credimi_deeplink"
	step0.ReturnedURI = strings.TrimSpace(body)
	r.ResolutionChain = append(r.ResolutionChain, step0)
	r.DeeplinkURI = step0.ReturnedURI

	// Parse the returned URI
	parsed, err := url.Parse(step0.ReturnedURI)
	if err != nil {
		r.Status = "error"
		r.Error = NewError("deeplink_parse_failed", "Could not parse deeplink URI",
			"The deeplink URI returned by Credimi could not be parsed.", step0.ReturnedURI, 0, true)
		return r
	}

	query := parsed.Query()

	if co := query.Get("credential_offer"); co != "" {
		decoded, err := url.QueryUnescape(co)
		if err != nil {
			decoded = co
		}
		r.CredentialOffer = json.RawMessage(decoded)
		r.FinalOfferPath = "credential-offer.json"
	} else if cou := query.Get("credential_offer_uri"); cou != "" {
		decodedURI, _ := url.QueryUnescape(cou)
		if err := r.resolveOfferURI(client, decodedURI, 1, maxDepth); err != nil {
			r.Status = "error"
			r.Error = NewError("credential_offer_uri_resolution_failed", err.Error(),
				"The credential offer URI could not be resolved.", decodedURI, 0, true)
		}
	}

	return r
}

func (r *Result) resolveOfferURI(client *http.Client, uri string, depth, maxDepth int) error {
	if depth > maxDepth {
		return fmt.Errorf("max depth %d exceeded", maxDepth)
	}

	step, body, err := fetchURL(client, uri)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}
	step.Depth = depth
	step.Kind = "credential_offer_uri"
	r.ResolutionChain = append(r.ResolutionChain, step)

	if step.HTTPStatus >= 400 {
		return fmt.Errorf("HTTP %d", step.HTTPStatus)
	}

	// Check if the response is JSON (concrete credential offer)
	if json.Valid([]byte(body)) {
		r.CredentialOffer = json.RawMessage(body)
		r.FinalOfferPath = "credential-offer.json"
		return nil
	}

	// Check if it's another URI (redirect to nested credential_offer_uri)
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "openid-credential-offer://") ||
		strings.HasPrefix(trimmed, "haip-vci://") ||
		strings.HasPrefix(trimmed, "haip-vp://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return fmt.Errorf("parse nested URI: %w", err)
		}
		query := parsed.Query()
		if cou := query.Get("credential_offer_uri"); cou != "" {
			decodedURI, _ := url.QueryUnescape(cou)
			return r.resolveOfferURI(client, decodedURI, depth+1, maxDepth)
		}
		if co := query.Get("credential_offer"); co != "" {
			decoded, _ := url.QueryUnescape(co)
			r.CredentialOffer = json.RawMessage(decoded)
			r.FinalOfferPath = "credential-offer.json"
			return nil
		}
	}

	// Treat as JSON credential offer anyway
	r.CredentialOffer = json.RawMessage(body)
	r.FinalOfferPath = "credential-offer.json"
	return nil
}

// FetchIssuerMetadata fetches the issuer metadata from .well-known/openid-credential-issuer.
func FetchIssuerMetadata(client *http.Client, credentialOffer json.RawMessage) (json.RawMessage, *MetadataFetch, error) {
	var offer struct {
		CredentialIssuer string `json:"credential_issuer"`
	}
	if err := json.Unmarshal(credentialOffer, &offer); err != nil {
		return nil, nil, fmt.Errorf("parse credential offer: %w", err)
	}
	if offer.CredentialIssuer == "" {
		return nil, nil, fmt.Errorf("credential_issuer not found in offer")
	}

	issuerURL := strings.TrimSuffix(offer.CredentialIssuer, "/")
	metaURL := issuerURL + "/.well-known/openid-credential-issuer"

	fetch := &MetadataFetch{
		URL:       metaURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	resp, err := client.Get(metaURL)
	if err != nil {
		return nil, fetch, fmt.Errorf("fetch issuer metadata: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fetch, fmt.Errorf("read issuer metadata: %w", err)
	}

	fetch.HTTPStatus = resp.StatusCode
	fetch.ContentType = resp.Header.Get("Content-Type")
	fetch.SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))

	// Determine format: JSON, JWT, or text
	contentType := resp.Header.Get("Content-Type")
	if jwt.LooksLikeJWT(string(body)) {
		fetch.Format = "jwt"
		token, err := jwt.Decode(string(body))
		if err != nil {
			return nil, fetch, fmt.Errorf("decode JWT metadata: %w", err)
		}
		// Wrap in metadata structure
		wrapped, _ := json.Marshal(map[string]any{
			"source_url":        metaURL,
			"content_type":      contentType,
			"format":            "jwt",
			"raw":               string(body),
			"header":            json.RawMessage(token.Header),
			"payload":           json.RawMessage(token.Payload),
			"signature_present": token.SignaturePresent,
		})
		return wrapped, fetch, nil
	}

	if strings.HasPrefix(contentType, "application/json") || json.Valid(body) {
		fetch.Format = "json"
		return body, fetch, nil
	}

	fetch.Format = "text"
	return body, fetch, nil
}

func fetchURL(client *http.Client, rawURL string) (ResolutionStep, string, error) {
	step := ResolutionStep{URL: rawURL}

	resp, err := client.Get(rawURL)
	if err != nil {
		step.HTTPStatus = 0
		return step, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		step.HTTPStatus = resp.StatusCode
		return step, "", err
	}

	step.HTTPStatus = resp.StatusCode
	step.ReturnedPayloadType = resp.Header.Get("Content-Type")
	step.ReturnedPayloadSHA256 = fmt.Sprintf("%x", sha256.Sum256(body))

	return step, string(body), nil
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
	default: // auto - default to raw, URL-encoding fallback handled at call site
		return url.QueryEscape(id)
	}
}

// NewError creates a new ExtractionError.
func NewError(code, message, humanMessage, url string, httpStatus int, recoverable bool) *ExtractionError {
	e := &ExtractionError{Status: "error"}
	e.Error.Code = code
	e.Error.Message = message
	e.Error.HumanMessage = humanMessage
	e.Error.URL = url
	e.Error.HTTPStatus = httpStatus
	e.Error.Recoverable = recoverable
	return e
}

func (e *ExtractionError) MarshalJSON() ([]byte, error) {
	type alias ExtractionError
	return json.Marshal((*alias)(e))
}
