package externaltests

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schemaLocalID is the internal location the embedded schema is registered
// under. The schema's own $id (a public credimi.io URL) is never fetched.
const schemaLocalID = "embedded:///credimi-conformance-result.schema.v1.1.json"

const (
	maxDiagnostics      = 20
	maxDiagnosticLength = 200
)

//go:embed schemas/credimi-conformance-result.schema.v1.1.json
var resultSchemaJSON []byte

var compiledSchema = sync.OnceValues(compileResultSchema)

// noRemoteLoader refuses every external schema reference.
type noRemoteLoader struct{}

func (noRemoteLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("schema loading is disabled for %s", url)
}

func compileResultSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(resultSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("externaltests: embedded schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(noRemoteLoader{})
	compiler.AssertFormat()
	if err := compiler.AddResource(schemaLocalID, doc); err != nil {
		return nil, fmt.Errorf("externaltests: embedded schema: %w", err)
	}
	schema, err := compiler.Compile(schemaLocalID)
	if err != nil {
		return nil, fmt.Errorf("externaltests: embedded schema: %w", err)
	}
	return schema, nil
}

// SchemaError lists bounded schema violations by instance location.
type SchemaError struct {
	Diagnostics []Diagnostic
}

// Diagnostic is one schema violation.
type Diagnostic struct {
	Path    string
	Message string
}

func (e *SchemaError) Error() string {
	parts := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		parts = append(parts, d.Path+": "+d.Message)
	}
	return "externaltests: document violates the v1.1 schema: " + strings.Join(parts, "; ")
}

// ValidateNormalized checks a normalized document against the embedded
// v1.1 schema. Validation proves shape only, not truth or certification.
func ValidateNormalized(doc *ConformanceResult) error {
	if doc == nil {
		return errors.New("externaltests: no document to validate")
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("externaltests: encode document: %w", err)
	}
	return ValidateDocument(data)
}

// ValidateDocument checks raw JSON bytes against the embedded v1.1 schema.
func ValidateDocument(data []byte) error {
	schema, err := compiledSchema()
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return &SchemaError{Diagnostics: []Diagnostic{{Path: "/", Message: "not a single valid JSON document"}}}
	}
	err = schema.Validate(instance)
	var validation *jsonschema.ValidationError
	if errors.As(err, &validation) {
		return &SchemaError{Diagnostics: diagnostics(validation)}
	}
	return err
}

func diagnostics(validation *jsonschema.ValidationError) []Diagnostic {
	var out []Diagnostic
	for _, unit := range validation.BasicOutput().Errors {
		if unit.Error == nil {
			continue
		}
		if len(out) == maxDiagnostics {
			out = append(out, Diagnostic{Path: "...", Message: "further violations omitted"})
			break
		}
		path := unit.InstanceLocation
		if path == "" {
			path = "/"
		}
		message := unit.Error.String()
		if len(message) > maxDiagnosticLength {
			message = message[:maxDiagnosticLength] + "..."
		}
		out = append(out, Diagnostic{Path: path, Message: message})
	}
	return out
}
