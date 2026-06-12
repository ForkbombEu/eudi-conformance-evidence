// Package webui serves a browser interface for protocol context extraction.
package webui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/jwt"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

const (
	maxFormBytes     = 64 << 10
	maxResponseBytes = 4 << 20
	defaultTimeout   = 30 * time.Second
)

//go:embed templates/*.html static/*.css static/*.js
var assets embed.FS

type server struct {
	client    *http.Client
	templates *template.Template
}

type pageData struct {
	Title      string
	Kind       string
	Input      string
	Source     string
	Output     string
	Details    string
	Error      string
	StatusText string
}

// Run starts the web interface.
func Run(args []string) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	shutdownTelemetry := telemetry.Setup()
	defer shutdownTelemetry()

	log.Printf("eudi conformance evidence web listening on %s", *addr)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           NewHandler(newSafeHTTPClient(defaultTimeout)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return httpServer.ListenAndServe()
}

// NewHandler returns the complete web application handler.
func NewHandler(client *http.Client) http.Handler {
	tmpl := template.Must(template.ParseFS(assets, "templates/*.html"))
	s := &server{client: client, templates: tmpl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("POST /extract", s.extract)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("GET /static/", http.FileServerFS(assets))
	return securityHeaders(mux)
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.render(w, http.StatusOK, "index.html", pageData{Title: "EUDI Context Extractor"})
}

func (s *server) extract(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "The submitted form is invalid.", "")
		return
	}

	kind := strings.TrimSpace(r.FormValue("kind"))
	input := strings.TrimSpace(r.FormValue("input"))
	if input == "" {
		s.renderError(w, http.StatusBadRequest, "An extraction input is required.", kind)
		return
	}

	data := pageData{Title: "Extraction Result", Kind: kind, Input: input, StatusText: "resolved"}
	var output any
	var details any
	var err error

	switch kind {
	case "credential-offer":
		data.Source = "Credential offer"
		output, details, err = s.extractCredentialOffer(input)
	case "credimi-credential":
		data.Source = "Credimi credential"
		output, details, err = s.extractCredimiCredential(input)
	case "presentation-request":
		data.Source = "Presentation request"
		output, details, err = s.extractPresentationRequest(input)
	case "credimi-verification":
		data.Source = "Credimi use-case verification"
		output, details, err = s.extractCredimiVerification(input)
	default:
		err = fmt.Errorf("unknown extraction type %q", kind)
	}

	if err != nil {
		data.Error = err.Error()
		data.StatusText = "failed"
		s.render(w, http.StatusUnprocessableEntity, "result.html", data)
		return
	}

	data.Output, err = prettyJSON(output)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err.Error(), kind)
		return
	}
	data.Details, _ = prettyJSON(details)
	s.render(w, http.StatusOK, "result.html", data)
}

func (s *server) extractCredentialOffer(input string) (any, any, error) {
	offer, chain, err := resolveCredentialOffer(s.client, input, 0)
	if err != nil {
		return nil, chain, err
	}
	metadata, fetch, err := credoffer.FetchIssuerMetadata(s.client, offer)
	if err != nil {
		return nil, chain, err
	}
	return rawJSONValue(metadata), map[string]any{"credential_offer": rawJSONValue(offer), "resolution_chain": chain, "metadata_fetch": fetch}, nil
}

func (s *server) extractCredimiCredential(input string) (any, any, error) {
	baseURL, id, err := parseCredimiHubURL(input, "credentials")
	if err != nil {
		return nil, nil, err
	}
	result := credoffer.Resolve(s.client, baseURL, id, "auto", 5)
	if result.Status != "ok" {
		return nil, result, extractionError(result.Error)
	}
	metadata, fetch, err := credoffer.FetchIssuerMetadata(s.client, result.CredentialOffer)
	if err != nil {
		return nil, result, err
	}
	result.IssuerMetadata = metadata
	result.IssuerMetadataFetch = fetch
	return rawJSONValue(metadata), result, nil
}

func (s *server) extractPresentationRequest(input string) (any, any, error) {
	payload, details, err := resolvePresentationRequest(s.client, input)
	if err != nil {
		return nil, details, err
	}
	dcql, err := findDCQL(payload)
	if err != nil {
		return nil, details, err
	}
	return dcql, details, nil
}

func (s *server) extractCredimiVerification(input string) (any, any, error) {
	baseURL, id, err := parseCredimiHubURL(input, "use_cases_verifications")
	if err != nil {
		return nil, nil, err
	}
	result := presentation.Resolve(s.client, baseURL, id, "auto", "auto", defaultTimeout)
	if result.Status != "ok" {
		return nil, result, presentationError(result.Error)
	}

	var payload json.RawMessage
	if result.RequestObject != nil {
		payload = result.RequestObject.Payload
	} else if json.Valid([]byte(result.RequestURIRaw)) {
		payload = json.RawMessage(result.RequestURIRaw)
	} else {
		return nil, result, errors.New("presentation request did not contain a JSON or JWT request object")
	}
	dcql, err := findDCQL(payload)
	if err != nil {
		return nil, result, err
	}
	return dcql, result, nil
}

func resolveCredentialOffer(client *http.Client, input string, depth int) (json.RawMessage, []map[string]any, error) {
	if depth > 5 {
		return nil, nil, errors.New("credential_offer_uri resolution exceeded 5 steps")
	}
	input = strings.TrimSpace(input)
	if json.Valid([]byte(input)) {
		return json.RawMessage(input), nil, nil
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" {
		return nil, nil, errors.New("input is not a credential offer JSON object or URI")
	}
	query := parsed.Query()
	if offer := query.Get("credential_offer"); offer != "" {
		if !json.Valid([]byte(offer)) {
			return nil, nil, errors.New("credential_offer is not valid JSON")
		}
		return json.RawMessage(offer), nil, nil
	}
	if offerURI := query.Get("credential_offer_uri"); offerURI != "" {
		body, status, contentType, err := fetchLimited(client, http.MethodGet, offerURI, nil)
		step := map[string]any{"depth": depth + 1, "url": offerURI, "http_status": status, "content_type": contentType}
		if err != nil {
			return nil, []map[string]any{step}, err
		}
		offer, chain, err := resolveCredentialOffer(client, string(body), depth+1)
		return offer, append([]map[string]any{step}, chain...), err
	}
	return nil, nil, errors.New("credential offer URI has neither credential_offer nor credential_offer_uri")
}

func resolvePresentationRequest(client *http.Client, input string) (json.RawMessage, any, error) {
	input = strings.TrimSpace(input)
	if json.Valid([]byte(input)) {
		return json.RawMessage(input), map[string]any{"format": "json"}, nil
	}
	if jwt.LooksLikeJWT(input) {
		token, err := jwt.Decode(input)
		if err != nil {
			return nil, nil, err
		}
		return token.Payload, token, nil
	}

	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" {
		return nil, nil, errors.New("input is not a presentation request JSON object, JWT, or URI")
	}
	query := parsed.Query()
	if request := query.Get("request"); request != "" {
		return resolvePresentationRequest(client, request)
	}
	requestURI := query.Get("request_uri")
	if requestURI == "" {
		return nil, nil, errors.New("presentation request URI has neither request nor request_uri")
	}

	method := strings.ToLower(query.Get("request_uri_method"))
	if method == "" {
		method = http.MethodGet
	}
	body, status, contentType, err := fetchPresentationObject(client, method, requestURI)
	details := map[string]any{"request_uri": requestURI, "request_uri_method": method, "http_status": status, "content_type": contentType}
	if err != nil {
		return nil, details, err
	}
	payload, decoded, err := resolvePresentationRequest(client, string(body))
	if err != nil {
		return nil, details, err
	}
	details["request_object"] = decoded
	return payload, details, nil
}

func fetchPresentationObject(client *http.Client, method, rawURL string) ([]byte, int, string, error) {
	if method != http.MethodPost {
		return fetchLimited(client, http.MethodGet, rawURL, nil)
	}
	strategies := []string{"", `wallet_nonce=web-extractor`, `wallet_metadata={}`, `wallet_metadata=`}
	var lastErr error
	for _, body := range strategies {
		response, status, contentType, err := fetchLimited(client, http.MethodPost, rawURL, strings.NewReader(body))
		if err == nil {
			return response, status, contentType, nil
		}
		lastErr = err
	}
	return nil, 0, "", fmt.Errorf("all request_uri POST strategies failed: %w", lastErr)
}

func fetchLimited(client *http.Client, method, rawURL string, body io.Reader) ([]byte, int, string, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, 0, "", err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	response, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp.StatusCode, resp.Header.Get("Content-Type"), err
	}
	if len(response) > maxResponseBytes {
		return nil, resp.StatusCode, resp.Header.Get("Content-Type"), errors.New("remote response exceeds 4 MiB")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, resp.StatusCode, resp.Header.Get("Content-Type"), fmt.Errorf("remote server returned HTTP %d", resp.StatusCode)
	}
	return response, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

func parseCredimiHubURL(input, collection string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(input))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", errors.New("input is not a valid Credimi Hub URL")
	}
	prefix := "/hub/" + collection + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", "", fmt.Errorf("expected a URL under %s", strings.TrimSuffix(prefix, "/"))
	}
	id := strings.TrimPrefix(parsed.EscapedPath(), prefix)
	decodedID, err := url.PathUnescape(id)
	if err != nil || decodedID == "" {
		return "", "", errors.New("Credimi Hub URL does not contain an identifier")
	}
	return parsed.Scheme + "://" + parsed.Host, "/" + decodedID, nil
}

func findDCQL(payload json.RawMessage) (any, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, fmt.Errorf("decode presentation request: %w", err)
	}
	if dcql, ok := walkForDCQL(value); ok {
		return dcql, nil
	}
	return nil, errors.New("no dcql_query was found in the presentation request")
}

func walkForDCQL(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			if normalized == "dcql_query" || normalized == "dcql" {
				return child, true
			}
		}
		for _, child := range typed {
			if found, ok := walkForDCQL(child); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range typed {
			if found, ok := walkForDCQL(child); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func rawJSONValue(raw json.RawMessage) any {
	var value any
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return string(raw)
}

func prettyJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("encode result: %w", err)
	}
	return strings.TrimSpace(buffer.String()), nil
}

func extractionError(err *credoffer.ExtractionError) error {
	if err == nil {
		return errors.New("credential extraction failed")
	}
	return errors.New(err.Error.Message)
}

func presentationError(err *presentation.ExtractionError) error {
	if err == nil {
		return errors.New("presentation extraction failed")
	}
	return errors.New(err.Error.Message)
}

func (s *server) renderError(w http.ResponseWriter, status int, message, kind string) {
	s.render(w, status, "result.html", pageData{Title: "Extraction Error", Kind: kind, Error: message, StatusText: "failed"})
}

func (s *server) render(w http.ResponseWriter, status int, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func newSafeHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = safeDialContext
	client := &http.Client{Timeout: timeout, Transport: transport}
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		return validateRemoteURL(req.URL)
	}
	return client
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, fmt.Errorf("outbound address %s is not public", address.IP)
		}
	}
	var selected net.IP
	for _, address := range addresses {
		if network == "tcp4" && address.IP.To4() == nil {
			continue
		}
		if network == "tcp6" && address.IP.To4() != nil {
			continue
		}
		selected = address.IP
		break
	}
	if selected == nil {
		return nil, fmt.Errorf("no public address available for network %s", network)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(selected.String(), port))
}

func validateRemoteURL(remote *url.URL) error {
	if remote.Scheme != "https" && remote.Scheme != "http" {
		return fmt.Errorf("unsupported remote URL scheme %q", remote.Scheme)
	}
	if remote.Hostname() == "" {
		return errors.New("remote URL has no hostname")
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast())
}
