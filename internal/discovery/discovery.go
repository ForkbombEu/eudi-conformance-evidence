// Package discovery finds credential-offer and presentation-request steps in pipeline JSON.
package discovery

import (
	"encoding/json"
	"fmt"
)

// Step represents a discovered pipeline step.
type Step struct {
	PipelineOrder int    `json:"pipeline_order"`
	StepID        string `json:"step_id"`
	Use           string `json:"use"`
	CredentialID  string `json:"credential_id,omitempty"`
	UseCaseID     string `json:"use_case_id,omitempty"`
}

// Result holds discovered steps from a pipeline input.
type Result struct {
	CredentialOfferSteps     []Step `json:"credential_offer_steps"`
	PresentationRequestSteps []Step `json:"presentation_request_steps"`
}

// Discover parses pipeline input JSON and finds credential-offer and
// use-case-verification-deeplink steps.
func Discover(input []byte) (*Result, error) {
	var root any
	if err := json.Unmarshal(input, &root); err != nil {
		return nil, fmt.Errorf("discovery: invalid JSON: %w", err)
	}

	var steps []rawStep

	// Try known locations first
	locations := []string{
		"workflow_definition.steps[]",
		"workflow_input.workflow_definition.steps[]",
		"payload.workflow_definition.steps[]",
	}

	for _, loc := range locations {
		if found := extractAt(root, loc); len(found) > 0 {
			steps = append(steps, found...)
		}
	}

	// If nothing found in known locations, do recursive scan
	if len(steps) == 0 {
		steps = recursiveScan(root)
	}

	result := &Result{}
	for i, s := range steps {
		credID := extractCredentialID(s.With)
		caseID := extractUseCaseID(s.With)

		switch s.Use {
		case "credential-offer":
			result.CredentialOfferSteps = append(result.CredentialOfferSteps, Step{
				PipelineOrder: i,
				StepID:        s.ID,
				Use:           s.Use,
				CredentialID:  credID,
			})
		case "use-case-verification-deeplink":
			result.PresentationRequestSteps = append(result.PresentationRequestSteps, Step{
				PipelineOrder: i,
				StepID:        s.ID,
				Use:           s.Use,
				UseCaseID:     caseID,
			})
		}
	}

	return result, nil
}

type rawStep struct {
	ID   string `json:"id"`
	Use  string `json:"use"`
	With any    `json:"with"`
}

func extractAt(root any, path string) []rawStep {
	switch path {
	case "workflow_definition.steps[]":
		m, ok := root.(map[string]any)
		if !ok {
			return nil
		}
		wd, ok := m["workflow_definition"].(map[string]any)
		if !ok {
			return nil
		}
		return parseStepsArray(wd["steps"])
	case "workflow_input.workflow_definition.steps[]":
		m, ok := root.(map[string]any)
		if !ok {
			return nil
		}
		wi, ok := m["workflow_input"].(map[string]any)
		if !ok {
			return nil
		}
		wd, ok := wi["workflow_definition"].(map[string]any)
		if !ok {
			return nil
		}
		return parseStepsArray(wd["steps"])
	case "payload.workflow_definition.steps[]":
		m, ok := root.(map[string]any)
		if !ok {
			return nil
		}
		p, ok := m["payload"].(map[string]any)
		if !ok {
			return nil
		}
		wd, ok := p["workflow_definition"].(map[string]any)
		if !ok {
			return nil
		}
		return parseStepsArray(wd["steps"])
	}
	return nil
}

func parseStepsArray(v any) []rawStep {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	return parseRawSteps(arr)
}

func parseRawSteps(arr []any) []rawStep {
	var steps []rawStep
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		use, _ := m["use"].(string)
		if id == "" || use == "" {
			continue
		}
		steps = append(steps, rawStep{
			ID:   id,
			Use:  use,
			With: m["with"],
		})
	}
	return steps
}

func recursiveScan(v any) []rawStep {
	switch val := v.(type) {
	case map[string]any:
		// If this map looks like a step
		if id, _ := val["id"].(string); id != "" {
			if use, _ := val["use"].(string); use != "" {
				return []rawStep{{ID: id, Use: use, With: val["with"]}}
			}
		}
		// Recurse into values
		var steps []rawStep
		for _, child := range val {
			steps = append(steps, recursiveScan(child)...)
		}
		return steps
	case []any:
		// First, try to parse as steps array
		if steps := parseRawSteps(val); len(steps) > 0 {
			return steps
		}
		// Otherwise recurse
		var steps []rawStep
		for _, child := range val {
			steps = append(steps, recursiveScan(child)...)
		}
		return steps
	default:
		return nil
	}
}

func extractCredentialID(with any) string {
	if with == nil {
		return ""
	}
	m, ok := with.(map[string]any)
	if !ok {
		return ""
	}
	// Direct: with.credential_id
	if id, ok := m["credential_id"].(string); ok {
		return id
	}
	// Nested: with.payload.credential_id
	if payload, ok := m["payload"].(map[string]any); ok {
		if id, ok := payload["credential_id"].(string); ok {
			return id
		}
	}
	return ""
}

func extractUseCaseID(with any) string {
	if with == nil {
		return ""
	}
	m, ok := with.(map[string]any)
	if !ok {
		return ""
	}
	// Direct: with.use_case_id
	if id, ok := m["use_case_id"].(string); ok {
		return id
	}
	// Nested: with.payload.use_case_id
	if payload, ok := m["payload"].(map[string]any); ok {
		if id, ok := payload["use_case_id"].(string); ok {
			return id
		}
	}
	return ""
}
