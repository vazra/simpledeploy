package backup

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
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

func (t *S3Target) Type() string { return "s3" }

func (t *S3Target) Test(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(t.cfg.Bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 head bucket %q: %w", t.cfg.Bucket, err)
	}
	return nil
}

func (t *S3Target) key(filename string) string {
	if t.cfg.Prefix != "" {
		return t.cfg.Prefix + "/" + filename
	}
	return filename
}

func (t *S3Target) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	// Use the manager Uploader so we can stream a non-seekable reader
	// (pg_dump/tar stdout piped through a gzip writer is not seekable,
	// which PutObject's SHA-256 computation requires).
	cr := &countReader{r: data}
	key := t.key(filename)
	uploader := manager.NewUploader(t.client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(key),
		Body:   cr,
	})
	if err != nil {
		return "", 0, fmt.Errorf("s3 put: %w", err)
	}
	return key, cr.n, nil
}

func (t *S3Target) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	out, err := t.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}
	return out.Body, nil
}

func (t *S3Target) Delete(ctx context.Context, path string) error {
	_, err := t.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

// PresignedURL returns a pre-signed GET URL valid for the given duration.
func (t *S3Target) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presigner := s3.NewPresignClient(t.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("s3 presign: %w", err)
	}
	return req.URL, nil
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
