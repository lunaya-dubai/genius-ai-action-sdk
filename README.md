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
}

type Output struct {
	OK bool `json:"ok"`
}

func main() {
	action.Main(action.Meta{
		Name:        "Echo",
		Description: "Echo a message",
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

## License

Apache-2.0 — see [LICENSE](./LICENSE).
