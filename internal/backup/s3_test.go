package backup

import (
	"testing"
)

func TestS3TargetImplementsInterface(t *testing.T) {
	var _ Target = (*S3Target)(nil)
}

func TestS3TargetNewClient(t *testing.T) {
	cfg := S3Config{
		Endpoint:  "http://localhost:9000",
		Bucket:    "backups",
		Prefix:    "simpledeploy",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Region:    "us-east-1",
	}

	target, err := NewS3Target(cfg)
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if target.client == nil {
		t.Error("expected non-nil s3 client")
	}
	if target.cfg.Bucket != "backups" {
		t.Errorf("bucket mismatch: %s", target.cfg.Bucket)
	}
}

func TestS3TargetDefaultRegion(t *testing.T) {
	cfg := S3Config{
		Bucket:    "backups",
		AccessKey: "key",
		SecretKey: "secret",
	}

	target, err := NewS3Target(cfg)
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if target.cfg.Region != "us-east-1" {
		t.Errorf("expected default region us-east-1, got %s", target.cfg.Region)
	}
}

func TestS3TargetKey(t *testing.T) {
	tests := []struct {
		prefix   string
		filename string
		want     string
	}{
		{"", "backup.gz", "backup.gz"},
		{"myapp", "backup.gz", "myapp/backup.gz"},
		{"a/b", "f.tar.gz", "a/b/f.tar.gz"},
	}
	for _, tc := range tests {
		t.Run(tc.prefix+"/"+tc.filename, func(t *testing.T) {
			target := &S3Target{cfg: S3Config{Prefix: tc.prefix}}
			got := target.key(tc.filename)
			if got != tc.want {
				t.Errorf("key(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}
