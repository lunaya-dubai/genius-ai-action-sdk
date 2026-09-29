package action

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// EnvNodeID carries the workflow step (node) ID of the running action; the
// runtime injects it so callbacks into the BFF can be attributed per node.
const EnvNodeID = "GENAI_NODE_ID"

// RunContext is the run identity the runtime injects into every action
// container. Actions pass it back to internal BFF surfaces (ProductOps,
// credential broker) so the BFF can scope the call to the owning org and
// attribute it to the run/node.
type RunContext struct {
	RuntimeRunID string
	NodeID       string
}

// RunContextFromEnv reads the injected run identity. RuntimeRunID is required
// for any internal BFF call; NodeID is best-effort (older runtimes omit it).
func RunContextFromEnv() (RunContext, error) {
	rc := RunContext{
		RuntimeRunID: strings.TrimSpace(os.Getenv(EnvRuntimeRunID)),
		NodeID:       strings.TrimSpace(os.Getenv(EnvNodeID)),
	}
	if rc.RuntimeRunID == "" {
		return rc, fmt.Errorf("%s unset: internal BFF calls need the run identity", EnvRuntimeRunID)
	}
	return rc, nil
}

// InternalRPC returns the base URL, HTTP client, and Connect client options an
// action needs to construct a generated Connect client against an internal,
// token-gated BFF service (e.g. genius.productops.v1.ProductOps). The bearer is
// attached by an interceptor; it is never returned to the caller or logged.
//
//	base, hc, opts, err := action.InternalRPC()
//	client := productopsv1connect.NewProductOpsClient(hc, base, opts...)
func InternalRPC() (baseURL string, httpClient connect.HTTPClient, opts []connect.ClientOption, err error) {
	baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv(EnvBrokerURL)), "/")
	token := strings.TrimSpace(os.Getenv(EnvBrokerToken))
	if baseURL == "" || token == "" {
		return "", nil, nil, fmt.Errorf("internal BFF not configured (%s/%s unset)", EnvBrokerURL, EnvBrokerToken)
	}
	httpClient = &http.Client{Timeout: 60 * time.Second}
	opts = []connect.ClientOption{connect.WithInterceptors(bearerInterceptor{token: token})}
	return baseURL, httpClient, opts, nil
}

type bearerInterceptor struct{ token string }

func (b bearerInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", "Bearer "+b.token)
		return next(ctx, req)
	}
}

func (b bearerInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", "Bearer "+b.token)
		return conn
	}
}

func (b bearerInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
