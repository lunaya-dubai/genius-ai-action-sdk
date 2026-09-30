// Package action is the SDK for writing GenAI runtime action binaries.
//
// Action process I/O contract:
//
//   - input:  one JSON object whose keys are input param names (matching the
//     action's input schema / Go struct json tags), read from stdin by default
//   - output: one JSON object matching the action's output schema, written to
//     stdout by default
//   - stderr: diagnostics (surfaced by the runtime on failure)
//   - exit 0 on success
//
// Cluster / Argo: set GENAI_INPUT_FILE and/or GENAI_OUTPUT_FILE to use paths
// instead of stdin/stdout (scratch images have no shell to pipe JSON).
//
// Schema discovery: pass --describe to print a [Document] JSON object on stdout
// (no handler invocation). Use [Main] so both modes are wired from the same
// In/Out types.
//
//	func main() {
//		action.Main(action.Meta{Name: "Echo", Description: "echo input"},
//			func(ctx context.Context, in Input) (Output, error) {
//				return Output{OK: true}, nil
//			})
//	}
package action

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

const DescribeFlag = "--describe"

// Env var names for file-based I/O (Argo Workflows / scratch images).
const (
	EnvInputFile  = "GENAI_INPUT_FILE"
	EnvOutputFile = "GENAI_OUTPUT_FILE"
)

var emptyObjectSchema = json.RawMessage(`{"type":"object","additionalProperties":false}`)

// Meta is catalog metadata for an action binary.
type Meta struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

// Document is the machine-readable describe payload printed for --describe.
type Document struct {
	Name         string          `json:"name"`
	Version      string          `json:"version,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// FileRef is a file port on an action signature / wire payload.
// LocalPath is reserved for runtime staging and is omitted from JSON.
type FileRef struct {
	URI       string `json:"uri,omitempty"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	LocalPath string `json:"-"`
}

// SecretRef is an opaque secret handle. Values are never echoed by the runtime.
type SecretRef struct {
	ID string `json:"id"`
}

// DecodeArgs reads a single JSON object from r into T.
func DecodeArgs[T any](r io.Reader) (T, error) {
	var out T
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return out, fmt.Errorf("decode action input: %w", err)
	}
	return out, nil
}

// WriteOutput writes v as a single JSON object to w.
func WriteOutput(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode action output: %w", err)
	}
	return nil
}

// SchemaFor infers a JSON Schema from a Go value's type.
// Nil yields an empty object schema. FileRef and SecretRef get x-genai.kind metadata.
//
// Closed string sets: add enum:"a,b,c" on a string field (comma-separated, trimmed).
// Values are injected as JSON Schema "enum" after inference; validation rejects other
// strings on describe/run.
func SchemaFor(v any) (json.RawMessage, error) {
	if v == nil {
		return append(json.RawMessage(nil), emptyObjectSchema...), nil
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	schema, err := jsonschema.ForType(t, &jsonschema.ForOptions{
		TypeSchemas: typeSchemas(),
	})
	if err != nil {
		return nil, err
	}
	if err := applyEnumTags(t, schema); err != nil {
		return nil, err
	}
	return json.Marshal(schema)
}

// applyEnumTags walks exported struct fields and sets Enum on matching schema
// properties from the enum:"a,b,c" tag. Anonymous embeddings share the parent
// property map; named nested structs recurse into their property schema.
func applyEnumTags(t reflect.Type, schema *jsonschema.Schema) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || schema == nil {
		return nil
	}
	if schema.Properties == nil {
		schema.Properties = map[string]*jsonschema.Schema{}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			if err := applyEnumTags(f.Type, schema); err != nil {
				return err
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		name := jsonFieldName(f)
		if name == "" || name == "-" {
			continue
		}
		enumTag := strings.TrimSpace(f.Tag.Get("enum"))
		if enumTag != "" {
			vals, err := parseEnumTag(enumTag)
			if err != nil {
				return fmt.Errorf("field %s: %w", f.Name, err)
			}
			prop := schema.Properties[name]
			if prop == nil {
				prop = &jsonschema.Schema{Type: "string"}
				schema.Properties[name] = prop
			}
			prop.Enum = vals
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && schema.Properties[name] != nil {
			if err := applyEnumTags(ft, schema.Properties[name]); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	return name
}

func parseEnumTag(tag string) ([]any, error) {
	parts := strings.Split(tag, ",")
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			return nil, fmt.Errorf("enum tag has empty value")
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("enum tag is empty")
	}
	return out, nil
}

// Describe builds a [Document] from meta and the In/Out type parameters.
func Describe[In, Out any](meta Meta) (Document, error) {
	var in In
	var out Out
	inSchema, err := SchemaFor(in)
	if err != nil {
		return Document{}, fmt.Errorf("input schema: %w", err)
	}
	outSchema, err := SchemaFor(out)
	if err != nil {
		return Document{}, fmt.Errorf("output schema: %w", err)
	}
	return Document{
		Name:         meta.Name,
		Version:      meta.Version,
		Description:  meta.Description,
		InputSchema:  inSchema,
		OutputSchema: outSchema,
	}, nil
}

// Main handles --describe or runs the action handler against stdin/stdout
// (or GENAI_INPUT_FILE / GENAI_OUTPUT_FILE when set).
func Main[In, Out any](meta Meta, fn func(context.Context, In) (Out, error)) {
	if wantsDescribe(os.Args[1:]) {
		doc, err := Describe[In, Out](meta)
		if err != nil {
			fail(err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			fail(err)
		}
		return
	}
	run(fn)
}

// run reads JSON input, validates it against the inferred input schema, invokes
// fn, validates the output against the inferred output schema, and writes JSON.
// By default it uses stdin/stdout. When GENAI_INPUT_FILE / GENAI_OUTPUT_FILE
// are set, those paths are used instead (for Argo / scratch images).
// On error it prints the message to stderr and exits with status 1.
func run[In, Out any](fn func(context.Context, In) (Out, error)) {
	ctx := context.Background()
	inR, inCloser, err := openInput()
	if err != nil {
		fail(err)
	}
	if inCloser != nil {
		defer inCloser.Close()
	}
	rawIn, err := io.ReadAll(inR)
	if err != nil {
		fail(fmt.Errorf("read input: %w", err))
	}
	inSchema, err := SchemaFor(*new(In))
	if err != nil {
		fail(fmt.Errorf("input schema: %w", err))
	}
	if err := validateRawAgainstSchema(inSchema, rawIn); err != nil {
		fail(fmt.Errorf("input schema validation: %w", err))
	}
	var in In
	if err := json.Unmarshal(rawIn, &in); err != nil {
		fail(fmt.Errorf("decode action input: %w", err))
	}
	out, err := fn(ctx, in)
	if err != nil {
		fail(err)
	}
	rawOut, err := json.Marshal(out)
	if err != nil {
		fail(fmt.Errorf("encode action output: %w", err))
	}
	outSchema, err := SchemaFor(*new(Out))
	if err != nil {
		fail(fmt.Errorf("output schema: %w", err))
	}
	if err := validateRawAgainstSchema(outSchema, rawOut); err != nil {
		fail(fmt.Errorf("output schema validation: %w", err))
	}
	outW, outCloser, err := openOutput()
	if err != nil {
		fail(err)
	}
	if outCloser != nil {
		defer outCloser.Close()
	}
	if _, err := outW.Write(append(rawOut, '\n')); err != nil {
		fail(fmt.Errorf("write action output: %w", err))
	}
}

func validateRawAgainstSchema(schemaJSON, instanceJSON json.RawMessage) error {
	if len(schemaJSON) == 0 {
		return nil
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema: %w", err)
	}
	var instance any
	if err := json.Unmarshal(instanceJSON, &instance); err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	return resolved.Validate(instance)
}

func openInput() (io.Reader, io.Closer, error) {
	path := os.Getenv(EnvInputFile)
	if path == "" {
		return os.Stdin, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s (%s): %w", EnvInputFile, path, err)
	}
	return f, f, nil
}

func openOutput() (io.Writer, io.Closer, error) {
	path := os.Getenv(EnvOutputFile)
	if path == "" {
		return os.Stdout, nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s (%s): %w", EnvOutputFile, path, err)
	}
	return f, f, nil
}

func wantsDescribe(args []string) bool {
	for _, a := range args {
		if a == DescribeFlag {
			return true
		}
	}
	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}

func typeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[FileRef]():   fileRefSchema(),
		reflect.TypeFor[SecretRef](): secretRefSchema(),
	}
}

func falseSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Not: &jsonschema.Schema{}}
}

func fileRefSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"uri":       {Type: "string", Description: "URI or staged path of the file"},
			"name":      {Type: "string", Description: "Display or base file name"},
			"mediaType": {Type: "string", Description: "IANA media type"},
		},
		AdditionalProperties: falseSchema(),
		Extra: map[string]any{
			"x-genai": map[string]any{"kind": "file"},
		},
	}
}

func secretRefSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"id": {Type: "string", Description: "Secret identifier"},
		},
		Required:             []string{"id"},
		AdditionalProperties: falseSchema(),
		Extra: map[string]any{
			"x-genai": map[string]any{"kind": "secret"},
		},
	}
}
