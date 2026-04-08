package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds connection settings for an S3-compatible object store.
type S3Config struct {
	Endpoint  string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
	Region    string
}

// S3Target stores backups in an S3-compatible object store.
type S3Target struct {
	cfg    S3Config
	client *s3.Client
}

func NewS3Target(cfg S3Config) (*S3Target, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	awsCfg := aws.Config{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
	}

	opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	return &S3Target{cfg: cfg, client: client}, nil
}

func (t *S3Target) key(filename string) string {
	if t.cfg.Prefix != "" {
		return t.cfg.Prefix + "/" + filename
	}
	return filename
}

func (t *S3Target) Upload(ctx context.Context, filename string, data io.Reader) (int64, error) {
	cr := &countReader{r: data}
	_, err := t.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(t.key(filename)),
		Body:   cr,
	})
	if err != nil {
		return 0, fmt.Errorf("s3 put: %w", err)
	}
	return cr.n, nil
}

func (t *S3Target) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	out, err := t.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(t.key(filename)),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}
	return out.Body, nil
}

func (t *S3Target) Delete(ctx context.Context, filename string) error {
	_, err := t.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(t.key(filename)),
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

// countReader wraps an io.Reader and counts bytes read.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
