package action

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestResolveCredentialAndHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/credentials/resolve", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":   "OAUTH2",
			"secret": map[string]string{"access_token": "tok-1"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv(EnvBrokerURL, srv.URL)
	t.Setenv(EnvBrokerToken, "t")
	t.Setenv(EnvRuntimeRunID, "run-1")

	cred, err := ResolveCredential(context.Background(), "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Type != "OAUTH2" || cred.Secret["access_token"] != "tok-1" {
		t.Fatalf("%+v", cred)
	}
	tok, err := cred.BearerToken()
	if err != nil || tok != "tok-1" {
		t.Fatalf("token=%q err=%v", tok, err)
	}
	hdrs, err := ResolveCredentialHeaders(context.Background(), "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	if hdrs["Authorization"] != "Bearer tok-1" {
		t.Fatalf("%v", hdrs)
	}
}

func TestResolveCredentialHeadersEmptyID(t *testing.T) {
	hdrs, err := ResolveCredentialHeaders(context.Background(), "")
	if err != nil || hdrs != nil {
		t.Fatalf("hdrs=%v err=%v", hdrs, err)
	}
}

func TestHTTPHeadersBearer(t *testing.T) {
	c := Credential{Type: "BEARER", Secret: map[string]string{"token": "x"}}
	hdrs, err := c.HTTPHeaders()
	if err != nil || hdrs["Authorization"] != "Bearer x" {
		t.Fatalf("%v %v", hdrs, err)
	}
}

func TestResolveCredentialRequiresBrokerEnv(t *testing.T) {
	os.Unsetenv(EnvBrokerURL)
	os.Unsetenv(EnvBrokerToken)
	_, err := ResolveCredential(context.Background(), "c")
	if err == nil {
		t.Fatal("expected error")
	}
}
