package s3_test

import (
	"testing"

	"github.com/lunaya-dubai/genius-ai-action-sdk/s3"
)

func TestEnvFromOS(t *testing.T) {
	t.Setenv("GENAI_S3_URL", "http://localhost:8333")
	t.Setenv("GENAI_S3_ID", "id")
	t.Setenv("GENAI_S3_SECRET", "secret")
	t.Setenv("GENAI_S3_BUCKET", "genai")

	cfg := s3.EnvFromOS()
	if cfg.URL != "http://localhost:8333" || cfg.Bucket != "genai" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if _, err := s3.NewFS(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestNewFSIncomplete(t *testing.T) {
	if _, err := s3.NewFS(s3.EnvConfig{}); err == nil {
		t.Fatal("expected error for incomplete config")
	}
}
