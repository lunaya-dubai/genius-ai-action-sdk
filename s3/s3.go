// Package s3 exposes cluster file storage for action binaries (SeaweedFS S3 API).
package s3

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	aws3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3fs "github.com/fclairamb/afero-s3"
	"github.com/spf13/afero"
)

var ErrEnvConfig = errors.New("incomplete S3 environment configuration")

// EnvConfig is S3 connection settings (GENAI_S3_* or explicit values).
type EnvConfig struct {
	URL    string
	ID     string
	Secret string
	Bucket string
}

// EnvFromOS reads S3 settings from GENAI_S3_URL, GENAI_S3_ID, GENAI_S3_SECRET, GENAI_S3_BUCKET.
func EnvFromOS() EnvConfig {
	return EnvConfig{
		URL:    strings.TrimSpace(os.Getenv("GENAI_S3_URL")),
		ID:     strings.TrimSpace(os.Getenv("GENAI_S3_ID")),
		Secret: strings.TrimSpace(os.Getenv("GENAI_S3_SECRET")),
		Bucket: strings.TrimSpace(os.Getenv("GENAI_S3_BUCKET")),
	}
}

func (c EnvConfig) ok() bool {
	return c.URL != "" && c.ID != "" && c.Secret != "" && c.Bucket != ""
}

// NewFSFromEnv builds an afero filesystem for the configured bucket.
func NewFSFromEnv() (afero.Fs, error) {
	return NewFS(EnvFromOS())
}

// Client builds an AWS S3 API client for the configured endpoint.
func Client(c EnvConfig) (*aws3.Client, error) {
	if !c.ok() {
		return nil, ErrEnvConfig
	}
	return aws3.NewFromConfig(aws.Config{
		Region:      "genai",
		Credentials: credentials.NewStaticCredentialsProvider(c.ID, c.Secret, ""),
	}, func(o *aws3.Options) {
		o.BaseEndpoint = aws.String(c.URL)
		o.UsePathStyle = true
	}), nil
}

// PutBytes uploads an object (single PutObject; avoids afero-s3 Create empty placeholder).
func PutBytes(ctx context.Context, c EnvConfig, key string, data []byte) error {
	client, err := Client(c)
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &aws3.PutObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(strings.TrimPrefix(key, "/")),
		Body:   bytes.NewReader(data),
	})
	return err
}

// NewFS builds an afero filesystem from explicit settings.
func NewFS(c EnvConfig) (afero.Fs, error) {
	if !c.ok() {
		return nil, ErrEnvConfig
	}
	client, err := Client(c)
	if err != nil {
		return nil, err
	}
	return s3fs.NewFsFromClient(c.Bucket, client), nil
}
