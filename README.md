# Genius AI Action SDK

Public Go SDK for writing Genius AI / Lunaya Flow **action** binaries.

```go
package main

import (
	"context"

	"github.com/lunaya-dubai/genius-ai-action-sdk/action"
)

type Input struct {
	Message string `json:"message"`
	Method  string `json:"method,omitempty" enum:"GET,POST,PUT,PATCH,DELETE,HEAD"`
}

type Output struct {
	OK bool `json:"ok"`
}

func main() {
	action.Main(action.Meta{
		Name:        "Echo",
		Version:     "1",
		Description: "Echo a message",
		Categories:  []string{"Utility"}, // first entry is primary palette group
	}, func(ctx context.Context, in Input) (Output, error) {
		return Output{OK: in.Message != ""}, nil
	})
}
```

## Packages

| Package | Purpose |
|---------|---------|
| [`action`](./action) | `Main` / `--describe`, stdin JSON I/O, credential broker, `InternalRPC` for Connect clients |
| [`s3`](./s3) | Cluster file storage helper (`GENAI_S3_*`, SeaweedFS S3 API) |

## Runtime-injected environment

The workflow runtime sets these in action containers:

| Variable | Used by |
|----------|---------|
| `GENAI_INPUT_FILE` / `GENAI_OUTPUT_FILE` | File I/O instead of stdin/stdout (scratch images) |
| `GENAI_CREDENTIAL_BROKER_URL` | Base URL for credential resolve + internal BFF |
| `GENAI_INTERNAL_TOKEN` | Bearer for broker / internal RPC |
| `GENAI_RUNTIME_RUN_ID` | Org-scoped run identity |
| `GENAI_NODE_ID` | Optional step id for audit |
| `GENAI_S3_URL` / `GENAI_S3_ID` / `GENAI_S3_SECRET` / `GENAI_S3_BUCKET` | Platform object storage (`s3` package) |

Credentials: call `action.ResolveCredential` / `ResolveCredentialHeaders` with a `credentialId` from the workflow input. Never log secrets.

## Version + rebuild skip

Set `Meta.Version` (string). It is emitted on `--describe` as `"version"`. BuildAction skips Build/Push when the catalog already has the same version + image for that action. **Bump the version manually** when you want that package rebuilt (adding a new package does not rebuild others).

## Categories (palette groups)

Set `Meta.Categories` (ordered strings). `--describe` emits `"categories": [...]`.

- **First entry is primary** — the BFF maps it to palette `group`.
- Later entries are optional secondary sections the UI may also show.
- Empty / omitted → consumers fall back to `"Actions"`.
- Values are trimmed; empties dropped; duplicates removed (case-sensitive), order preserved.

```go
Categories: []string{"Apps", "Files"}, // primary Apps; also list under Files
```

## Icon URL

Set `Meta.IconURL` to a public `http` or `https` image. `--describe` emits `"icon_url"`. Empty is fine. Userinfo in the URL is rejected. The catalog stores it and the canvas loads it with the node; it is not written onto the workflow document. Changing only the icon does not require a version bump — a catalog sync still upserts describe metadata when the image build is skipped.

```go
IconURL: "https://s3.geniusai.io/genai/public/svg/gmail.svg",
```

## Enum fields

`google/jsonschema-go` treats `jsonschema` tags as descriptions only. For closed string sets, add a separate tag:

```go
Method string `json:"method" jsonschema:"HTTP method" enum:"GET,POST,PUT,PATCH,DELETE,HEAD"`
```

`SchemaFor` / `--describe` emit JSON Schema `"enum": [...]` (string values only). Invalid values fail input validation before the handler runs.

## Mode-dependent fields (`when`)

When one enum field switches which other inputs apply (Branch `mode`, ProductCall `operation`), tag those fields:

```go
Mode string `json:"mode" enum:"if,switch"`
Op   string `json:"op,omitempty" enum:"eq,ne" when:"mode=if"`
Left any    `json:"left,omitempty" when:"mode=if"`
Value any   `json:"value,omitempty" when:"mode=switch,if"` // visible for any listed value
```

`--describe` emits on that property:

```json
"x-genai": { "when": { "field": "mode", "in": ["if"] } }
```

Untagged fields are always visible. The GUI walks `inputSchema.properties` — no root field-name map. One discriminator per input struct.

## License

Apache-2.0 — see [LICENSE](./LICENSE).
