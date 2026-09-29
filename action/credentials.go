package action

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Credential broker env (GAB-86): injected into action containers by the
// runtime. The broker resolves credentialId references at execution time so
// secrets never appear in workflow defs, Argo params, logs, or artifacts.
const (
	EnvBrokerURL    = "GENAI_CREDENTIAL_BROKER_URL"
	EnvBrokerToken  = "GENAI_INTERNAL_TOKEN"
	EnvRuntimeRunID = "GENAI_RUNTIME_RUN_ID"
)

// Credential is the material returned by the BFF credential broker.
// Secret keys depend on Type (e.g. token, access_token, private_key).
// Never log Secret.
type Credential struct {
	Type   string            `json:"type"`
	Secret map[string]string `json:"secret"`
}

// ResolveCredential fetches credential material for a credential reference
// from the BFF broker. The response is never logged by this package; callers
// must not log Secret either.
func ResolveCredential(ctx context.Context, credentialID string) (Credential, error) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return Credential{}, fmt.Errorf("credentialId is required")
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvBrokerURL)), "/")
	token := strings.TrimSpace(os.Getenv(EnvBrokerToken))
	if base == "" || token == "" {
		return Credential{}, fmt.Errorf("credential broker not configured (%s/%s unset)", EnvBrokerURL, EnvBrokerToken)
	}
	body, err := json.Marshal(map[string]string{
		"credentialId": credentialID,
		"runtimeRunId": strings.TrimSpace(os.Getenv(EnvRuntimeRunID)),
		"nodeId":       strings.TrimSpace(os.Getenv(EnvNodeID)),
	})
	if err != nil {
		return Credential{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/credentials/resolve", bytes.NewReader(body))
	if err != nil {
		return Credential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("credential broker: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Credential{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Credential{}, fmt.Errorf("credential broker: status %d", resp.StatusCode)
	}
	var out Credential
	if err := json.Unmarshal(raw, &out); err != nil {
		return Credential{}, fmt.Errorf("credential broker: decode: %w", err)
	}
	if out.Secret == nil {
		out.Secret = map[string]string{}
	}
	return out, nil
}

// ResolveCredentialHeaders resolves a credential and maps it to HTTP headers
// suitable for outbound API calls. Returns nil when credentialID is empty.
func ResolveCredentialHeaders(ctx context.Context, credentialID string) (map[string]string, error) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return nil, nil
	}
	cred, err := ResolveCredential(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	return cred.HTTPHeaders()
}

// HTTPHeaders maps this credential to Authorization (or custom) headers.
func (c Credential) HTTPHeaders() (map[string]string, error) {
	switch strings.ToUpper(strings.TrimSpace(c.Type)) {
	case "BEARER":
		tok := strings.TrimSpace(c.Secret["token"])
		if tok == "" {
			return nil, fmt.Errorf("BEARER credential missing token")
		}
		return map[string]string{"Authorization": "Bearer " + tok}, nil
	case "OAUTH2":
		tok := strings.TrimSpace(c.Secret["access_token"])
		if tok == "" {
			tok = strings.TrimSpace(c.Secret["token"])
		}
		if tok == "" {
			return nil, fmt.Errorf("OAUTH2 credential missing access_token")
		}
		return map[string]string{"Authorization": "Bearer " + tok}, nil
	case "BASIC":
		user := c.Secret["username"]
		pass := c.Secret["password"]
		raw := user + ":" + pass
		return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))}, nil
	case "HTTP_HEADER":
		h := strings.TrimSpace(c.Secret["header"])
		if h == "" {
			return nil, fmt.Errorf("HTTP_HEADER credential missing header")
		}
		return map[string]string{h: c.Secret["value"]}, nil
	default:
		return nil, fmt.Errorf("credential type %q cannot be mapped to HTTP headers", c.Type)
	}
}

// BearerToken returns an access token for BEARER or OAUTH2 credentials.
func (c Credential) BearerToken() (string, error) {
	switch strings.ToUpper(strings.TrimSpace(c.Type)) {
	case "BEARER":
		tok := strings.TrimSpace(c.Secret["token"])
		if tok == "" {
			return "", fmt.Errorf("BEARER credential missing token")
		}
		return tok, nil
	case "OAUTH2":
		tok := strings.TrimSpace(c.Secret["access_token"])
		if tok == "" {
			tok = strings.TrimSpace(c.Secret["token"])
		}
		if tok == "" {
			return "", fmt.Errorf("OAUTH2 credential missing access_token")
		}
		return tok, nil
	default:
		return "", fmt.Errorf("credential type %q has no bearer token", c.Type)
	}
}
