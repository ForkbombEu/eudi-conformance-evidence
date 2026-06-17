package credoffer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

// AuthorizationServerFetch records one authorization-server discovery attempt.
type AuthorizationServerFetch struct {
	URL         string `json:"url"`
	HTTPStatus  int    `json:"http_status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	FetchedAt   string `json:"fetched_at"`
	Format      string `json:"format,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Error       string `json:"error,omitempty"`
}

// AuthorizationServerMetadata holds metadata and every discovery attempt for one server.
type AuthorizationServerMetadata struct {
	Issuer   string                     `json:"issuer"`
	Source   string                     `json:"source"`
	Metadata json.RawMessage            `json:"metadata,omitempty"`
	Fetches  []AuthorizationServerFetch `json:"fetches"`
	Error    string                     `json:"error,omitempty"`
}

// FetchAuthorizationServerMetadata discovers and fetches the authorization servers
// selected by the credential offer and credential issuer metadata.
func FetchAuthorizationServerMetadata(client *http.Client, credentialOffer, issuerMetadata json.RawMessage) ([]AuthorizationServerMetadata, error) {
	servers, err := authorizationServers(credentialOffer, issuerMetadata)
	if err != nil {
		return nil, err
	}

	results := make([]AuthorizationServerMetadata, 0, len(servers))
	var fetchErrors []error
	for _, server := range servers {
		result := AuthorizationServerMetadata{Issuer: server.Issuer, Source: server.Source}
		for _, discoveryURL := range authorizationDiscoveryURLs(server.Issuer) {
			metadata, fetch, fetchErr := fetchAuthorizationServerURL(client, discoveryURL)
			result.Fetches = append(result.Fetches, fetch)
			if fetchErr == nil {
				result.Metadata = metadata
				result.Error = ""
				break
			}
			result.Error = fetchErr.Error()
		}
		if result.Metadata == nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("authorization server %s: %s", server.Issuer, result.Error))
		}
		results = append(results, result)
	}

	return results, errors.Join(fetchErrors...)
}

type authorizationServer struct {
	Issuer string
	Source string
}

func authorizationServers(credentialOffer, issuerMetadata json.RawMessage) ([]authorizationServer, error) {
	var offer struct {
		CredentialIssuer string                     `json:"credential_issuer"`
		Grants           map[string]json.RawMessage `json:"grants"`
	}
	if err := json.Unmarshal(credentialOffer, &offer); err != nil {
		return nil, fmt.Errorf("parse credential offer for authorization server: %w", err)
	}

	var selected []authorizationServer
	for _, grant := range offer.Grants {
		var value struct {
			AuthorizationServer string `json:"authorization_server"`
		}
		if json.Unmarshal(grant, &value) == nil && value.AuthorizationServer != "" {
			selected = append(selected, authorizationServer{Issuer: value.AuthorizationServer, Source: "credential_offer"})
		}
	}
	if len(selected) > 0 {
		return uniqueAuthorizationServers(selected)
	}

	payload := issuerMetadataPayload(issuerMetadata)
	var metadata struct {
		CredentialIssuer     string   `json:"credential_issuer"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return nil, fmt.Errorf("parse issuer metadata for authorization server: %w", err)
	}
	for _, issuer := range metadata.AuthorizationServers {
		if strings.TrimSpace(issuer) != "" {
			selected = append(selected, authorizationServer{Issuer: issuer, Source: "issuer_metadata"})
		}
	}
	if len(selected) == 0 {
		issuer := metadata.CredentialIssuer
		if issuer == "" {
			issuer = offer.CredentialIssuer
		}
		selected = append(selected, authorizationServer{Issuer: issuer, Source: "credential_issuer"})
	}

	return uniqueAuthorizationServers(selected)
}

func uniqueAuthorizationServers(servers []authorizationServer) ([]authorizationServer, error) {
	seen := make(map[string]bool, len(servers))
	unique := make([]authorizationServer, 0, len(servers))
	for _, server := range servers {
		server.Issuer = strings.TrimSuffix(strings.TrimSpace(server.Issuer), "/")
		parsed, err := url.Parse(server.Issuer)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("invalid authorization server issuer %q", server.Issuer)
		}
		if !seen[server.Issuer] {
			seen[server.Issuer] = true
			unique = append(unique, server)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].Issuer < unique[j].Issuer })
	return unique, nil
}

func issuerMetadataPayload(metadata json.RawMessage) json.RawMessage {
	var wrapped struct {
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(metadata, &wrapped) == nil && json.Valid(wrapped.Payload) {
		return wrapped.Payload
	}
	return metadata
}

func authorizationDiscoveryURLs(issuer string) []string {
	parsed, _ := url.Parse(issuer)
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if path == "/" {
		path = ""
	}
	base := parsed.Scheme + "://" + parsed.Host
	return []string{
		base + "/.well-known/oauth-authorization-server" + path,
		base + path + "/.well-known/openid-configuration",
	}
}

func fetchAuthorizationServerURL(client *http.Client, discoveryURL string) (json.RawMessage, AuthorizationServerFetch, error) {
	fetch := AuthorizationServerFetch{
		URL:       discoveryURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	var resp *http.Response
	var body []byte
	err := telemetry.TraceHTTP(context.Background(), http.MethodGet, discoveryURL, func() (int, error) {
		req, requestErr := http.NewRequest(http.MethodGet, discoveryURL, nil)
		if requestErr != nil {
			return 0, requestErr
		}
		req.Header.Set("Accept", "application/json")
		resp, requestErr = client.Do(req)
		if requestErr != nil {
			return 0, requestErr
		}
		defer resp.Body.Close() //nolint:errcheck
		body, requestErr = io.ReadAll(resp.Body)
		return resp.StatusCode, requestErr
	})
	if err != nil {
		fetch.Error = err.Error()
		return nil, fetch, err
	}

	fetch.HTTPStatus = resp.StatusCode
	fetch.ContentType = resp.Header.Get("Content-Type")
	fetch.SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	if resp.StatusCode >= http.StatusBadRequest {
		fetch.Error = fmt.Sprintf("remote server returned HTTP %d", resp.StatusCode)
		return nil, fetch, errors.New(fetch.Error)
	}
	if !json.Valid(body) {
		fetch.Error = "authorization server metadata is not valid JSON"
		return nil, fetch, errors.New(fetch.Error)
	}

	fetch.Format = "json"
	return json.RawMessage(body), fetch, nil
}
