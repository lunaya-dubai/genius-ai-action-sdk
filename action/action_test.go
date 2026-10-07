package action_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lunaya-dubai/genius-ai-action-sdk/action"
)

type sampleIn struct {
	Prompt string           `json:"prompt"`
	Audio  action.FileRef   `json:"audio"`
	Token  action.SecretRef `json:"token"`
}

type sampleOut struct {
	Text string `json:"text"`
	N    int    `json:"n"`
}

func TestDecodeArgs(t *testing.T) {
	raw := `{
		"prompt": "hi",
		"audio": {"uri": "file://a.wav", "name": "a", "mediaType": "audio/wav"},
		"token": {"id": "sk"}
	}`
	in, err := action.DecodeArgs[sampleIn](strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in.Prompt != "hi" || in.Token.ID != "sk" {
		t.Fatalf("got %+v", in)
	}
}

func TestWriteOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := action.WriteOutput(&buf, sampleOut{Text: "ok", N: 2}); err != nil {
		t.Fatal(err)
	}
	var got sampleOut
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Text != "ok" || got.N != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestDescribe(t *testing.T) {
	doc, err := action.Describe[sampleIn, sampleOut](action.Meta{
		Name:        "Sample",
		Version:     "1.2.3",
		Description: "demo",
		Categories:  []string{" Apps ", "Data", "Apps", "", "Flow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Sample" || doc.Description != "demo" || doc.Version != "1.2.3" {
		t.Fatalf("meta: %+v", doc)
	}
	wantCats := []string{"Apps", "Data", "Flow"}
	if len(doc.Categories) != len(wantCats) {
		t.Fatalf("categories: %#v", doc.Categories)
	}
	for i := range wantCats {
		if doc.Categories[i] != wantCats[i] {
			t.Fatalf("categories: %#v", doc.Categories)
		}
	}
	var inDecoded map[string]any
	if err := json.Unmarshal(doc.InputSchema, &inDecoded); err != nil {
		t.Fatal(err)
	}
	props := inDecoded["properties"].(map[string]any)
	audio := props["audio"].(map[string]any)
	if audio["x-genai"].(map[string]any)["kind"] != "file" {
		t.Fatalf("audio kind: %#v", audio["x-genai"])
	}
	token := props["token"].(map[string]any)
	if token["x-genai"].(map[string]any)["kind"] != "secret" {
		t.Fatalf("token kind: %#v", token["x-genai"])
	}
}

func TestSchemaForEnumTag(t *testing.T) {
	type in struct {
		Method string `json:"method" jsonschema:"HTTP method" enum:"GET,POST,PUT"`
		URL    string `json:"url"`
	}
	raw, err := action.SchemaFor(in{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	props := decoded["properties"].(map[string]any)
	method := props["method"].(map[string]any)
	enum, ok := method["enum"].([]any)
	if !ok {
		t.Fatalf("method.enum missing: %#v", method)
	}
	want := []string{"GET", "POST", "PUT"}
	if len(enum) != len(want) {
		t.Fatalf("enum=%v want %v", enum, want)
	}
	for i, v := range want {
		if enum[i] != v {
			t.Fatalf("enum[%d]=%v want %q", i, enum[i], v)
		}
	}
	url := props["url"].(map[string]any)
	if _, has := url["enum"]; has {
		t.Fatalf("url should not have enum: %#v", url)
	}
}

func TestSchemaForEnumTagRejectsEmptyToken(t *testing.T) {
	type bad struct {
		Op string `json:"op" enum:"eq,,ne"`
	}
	if _, err := action.SchemaFor(bad{}); err == nil {
		t.Fatal("expected error for empty enum token")
	}
}

func TestSchemaForWhenTag(t *testing.T) {
	type in struct {
		Mode  string `json:"mode" enum:"if,switch"`
		Op    string `json:"op,omitempty" enum:"eq,ne" when:"mode=if"`
		Left  any    `json:"left,omitempty" when:"mode=if"`
		Value any    `json:"value,omitempty" when:"mode=switch"`
		Always string `json:"always"`
	}
	raw, err := action.SchemaFor(in{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	props := decoded["properties"].(map[string]any)
	op := props["op"].(map[string]any)
	xg := op["x-genai"].(map[string]any)
	when := xg["when"].(map[string]any)
	if when["field"] != "mode" {
		t.Fatalf("when.field=%v", when["field"])
	}
	inVals, ok := when["in"].([]any)
	if !ok || len(inVals) != 1 || inVals[0] != "if" {
		t.Fatalf("when.in=%v", when["in"])
	}
	if _, has := op["enum"]; !has {
		t.Fatal("op should keep enum")
	}
	val := props["value"].(map[string]any)
	vin := val["x-genai"].(map[string]any)["when"].(map[string]any)["in"].([]any)
	if len(vin) != 1 || vin[0] != "switch" {
		t.Fatalf("value when.in=%v", vin)
	}
	always := props["always"].(map[string]any)
	if _, has := always["x-genai"]; has {
		t.Fatalf("always should have no x-genai: %#v", always)
	}
	mode := props["mode"].(map[string]any)
	if _, has := mode["x-genai"]; has {
		t.Fatalf("discriminator should have no when: %#v", mode)
	}
}

func TestSchemaForWhenTagMultipleValues(t *testing.T) {
	type in struct {
		Op    string `json:"operation" enum:"a,b,c"`
		Field string `json:"field,omitempty" when:"operation=a,b"`
	}
	raw, err := action.SchemaFor(in{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	when := decoded["properties"].(map[string]any)["field"].(map[string]any)["x-genai"].(map[string]any)["when"].(map[string]any)
	inVals := when["in"].([]any)
	if when["field"] != "operation" || len(inVals) != 2 || inVals[0] != "a" || inVals[1] != "b" {
		t.Fatalf("when=%v", when)
	}
}

func TestSchemaForWhenTagRejectsTwoDiscriminators(t *testing.T) {
	type bad struct {
		A string `json:"a,omitempty" when:"mode=if"`
		B string `json:"b,omitempty" when:"operation=create"`
	}
	if _, err := action.SchemaFor(bad{}); err == nil {
		t.Fatal("expected error for multiple discriminators")
	}
}

func TestSchemaForWhenTagRejectsEmptyToken(t *testing.T) {
	type bad struct {
		A string `json:"a,omitempty" when:"mode=if,"`
	}
	if _, err := action.SchemaFor(bad{}); err == nil {
		t.Fatal("expected error for empty when token")
	}
}

func TestMainDescribeFlag(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "go.mod")
	main := filepath.Join(dir, "main.go")
	sdkRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mod, []byte("module example.com/tmpaction\n\ngo 1.26\n\nrequire github.com/lunaya-dubai/genius-ai-action-sdk v0.0.0\n\nreplace github.com/lunaya-dubai/genius-ai-action-sdk => "+sdkRoot+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main
import (
  "context"
  "github.com/lunaya-dubai/genius-ai-action-sdk/action"
)
type In struct { N int ` + "`json:\"n\"`" + ` }
type Out struct { OK bool ` + "`json:\"ok\"`" + ` }
func main() {
  action.Main(action.Meta{Name: "Tmp", Description: "tmp action"},
    func(ctx context.Context, in In) (Out, error) {
      return Out{OK: in.N > 0}, nil
    })
}
`
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	cmd := exec.Command("go", "run", ".", "--describe")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run --describe: %v\n%s", err, out)
	}
	var doc action.Document
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("decode document: %v\n%s", err, out)
	}
	if doc.Name != "Tmp" {
		t.Fatalf("name=%q", doc.Name)
	}
	if !json.Valid(doc.InputSchema) || !json.Valid(doc.OutputSchema) {
		t.Fatalf("invalid schemas: %s", out)
	}
}

func TestMainFileIO(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "go.mod")
	main := filepath.Join(dir, "main.go")
	sdkRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mod, []byte("module example.com/tmpaction\n\ngo 1.26\n\nrequire github.com/lunaya-dubai/genius-ai-action-sdk v0.0.0\n\nreplace github.com/lunaya-dubai/genius-ai-action-sdk => "+sdkRoot+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main
import (
  "context"
  "github.com/lunaya-dubai/genius-ai-action-sdk/action"
)
type In struct { N int ` + "`json:\"n\"`" + ` }
type Out struct { OK bool ` + "`json:\"ok\"`" + ` }
func main() {
  action.Main(action.Meta{Name: "Tmp", Description: "tmp action"},
    func(ctx context.Context, in In) (Out, error) {
      return Out{OK: in.N > 0}, nil
    })
}
`
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	inPath := filepath.Join(dir, "in.json")
	outPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(inPath, []byte(`{"n":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		action.EnvInputFile+"="+inPath,
		action.EnvOutputFile+"="+outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run file io: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode out: %v\n%s", err, raw)
	}
	if !got.OK {
		t.Fatalf("expected ok=true, got %s", raw)
	}
}
