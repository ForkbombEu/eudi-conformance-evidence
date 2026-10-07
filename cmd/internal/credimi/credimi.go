// Package credimi fetches record deeplinks from a Credimi instance for the
// standalone tools. They run outside Credimi, so its public deeplink routes are
// the only way for them to turn a credential or verification ID into a
// deeplink; resolving the deeplink itself is left to pkg/credoffer and
// pkg/presentation.
package credimi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/forkbombeu/eudi-conformance-evidence/pkg/credoffer"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/presentation"
	"github.com/forkbombeu/eudi-conformance-evidence/pkg/telemetry"
)

// DeeplinkURL returns the Credimi route that answers the deeplink of a record.
// kind is "credential" or "verification". idEncoding "raw" sends the ID as is;
// any other value query-escapes it.
func DeeplinkURL(baseURL, kind, id, idEncoding string) string {
	if idEncoding != "raw" {
		id = url.QueryEscape(id)
	}
	return fmt.Sprintf("%s/api/%s/deeplink?id=%s", strings.TrimSuffix(baseURL, "/"), kind, id)
}

// ResolveCredential fetches the credential deeplink of credentialID from Credimi
// and resolves the credential offer it carries. Step 0 of the resolution chain
// records the Credimi request.
func ResolveCredential(client *http.Client, baseURL, credentialID, idEncoding string, maxDepth int) *credoffer.Result {
	deeplinkURL := DeeplinkURL(baseURL, "credential", credentialID, idEncoding)
	resp, err := fetch(client, deeplinkURL)
	if err != nil {
		return &credoffer.Result{
			Status:       "error",
			CredentialID: credentialID,
			Error: credoffer.NewError("credimi_deeplink_fetch_failed", "Could not fetch Credimi deeplink",
				"The Credimi deeplink could not be fetched.", deeplinkURL, 0, true),
		}
	}
	step0 := credoffer.ResolutionStep{
		Kind:                  "deeplink",
		URL:                   deeplinkURL,
		HTTPStatus:            resp.status,
		ReturnedPayloadType:   resp.contentType,
		ReturnedPayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(resp.body)),
	}
	if resp.status >= 400 {
		return &credoffer.Result{
			Status:          "error",
			CredentialID:    credentialID,
			ResolutionChain: []credoffer.ResolutionStep{step0},
			Error: credoffer.NewError("credimi_deeplink_fetch_failed", fmt.Sprintf("Credimi returned HTTP %d", resp.status),
				"The Credimi deeplink endpoint returned an error.", deeplinkURL, resp.status, true),
		}
	}

	r := credoffer.ResolveDeeplink(client, credentialID, string(resp.body), maxDepth)
	if len(r.ResolutionChain) > 0 {
		step0.ReturnedURI = r.ResolutionChain[0].ReturnedURI
		r.ResolutionChain[0] = step0
	}
	return r
}

// ResolveVerification fetches the verification deeplink of useCaseID from
// Credimi and resolves the presentation request it names.
func ResolveVerification(client *http.Client, baseURL, useCaseID, idEncoding, postStrategy string) *presentation.Result {
	deeplinkURL := DeeplinkURL(baseURL, "verification", useCaseID, idEncoding)
	resp, err := fetch(client, deeplinkURL)
	if err == nil && resp.status >= 400 {
		err = fmt.Errorf("HTTP %d", resp.status)
	}
	if err != nil {
		e := &presentation.ExtractionError{Status: "error"}
		e.Error.Code = "verification_deeplink_fetch_failed"
		e.Error.Message = "Could not fetch verification deeplink"
		e.Error.HumanMessage = "The verification deeplink could not be fetched from Credimi."
		e.Error.URL = deeplinkURL
		e.Error.HTTPStatus = resp.status
		e.Error.Recoverable = true
		return &presentation.Result{Status: "error", UseCaseID: useCaseID, PostStrategy: postStrategy, Error: e}
	}
	return presentation.ResolveDeeplink(client, useCaseID, string(resp.body), postStrategy)
}

type response struct {
	status      int
	contentType string
	body        []byte
}

func fetch(client *http.Client, rawURL string) (response, error) {
	var out response
	err := telemetry.TraceHTTP(context.Background(), http.MethodGet, rawURL, func() (int, error) {
		resp, err := client.Get(rawURL)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close() //nolint:errcheck
		out.status = resp.StatusCode
		out.contentType = resp.Header.Get("Content-Type")
		out.body, err = io.ReadAll(resp.Body)
		return resp.StatusCode, err
	})
	return out, err
}
