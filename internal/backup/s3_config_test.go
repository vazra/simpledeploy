package backup

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestS3Config_UnmarshalJSON(t *testing.T) {
	full := S3Config{
		Endpoint:  "https://s3.example.com",
		Bucket:    "my-backups",
		Prefix:    "simpledeploy/app",
		AccessKey: "AKIDEXAMPLE",
		SecretKey: "s3cr3t",
		Region:    "eu-west-1",
	}
	cases := []struct {
		name string
		in   string
		want S3Config
	}{
		{
			// What the UI wizard and docs send.
			name: "snake_case",
			in:   `{"endpoint":"https://s3.example.com","bucket":"my-backups","prefix":"simpledeploy/app","access_key":"AKIDEXAMPLE","secret_key":"s3cr3t","region":"eu-west-1"}`,
			want: full,
		},
		{
			// What older API clients (and the e2e suite) send, and what
			// configs stored before the struct had json tags look like.
			name: "legacy Go field names",
			in:   `{"Endpoint":"https://s3.example.com","Bucket":"my-backups","Prefix":"simpledeploy/app","AccessKey":"AKIDEXAMPLE","SecretKey":"s3cr3t","Region":"eu-west-1"}`,
			want: full,
		},
		{
			name: "camelCase",
			in:   `{"endpoint":"https://s3.example.com","bucket":"my-backups","prefix":"simpledeploy/app","accessKey":"AKIDEXAMPLE","secretKey":"s3cr3t","region":"eu-west-1"}`,
			want: full,
		},
		{
			name: "snake_case wins over legacy",
			in:   `{"bucket":"b","access_key":"new-key","secret_key":"new-secret","AccessKey":"old-key","SecretKey":"old-secret"}`,
			want: S3Config{Bucket: "b", AccessKey: "new-key", SecretKey: "new-secret"},
		},
		{
			name: "empty snake_case falls back to legacy",
			in:   `{"bucket":"b","access_key":"","secret_key":"","AccessKey":"old-key","SecretKey":"old-secret"}`,
			want: S3Config{Bucket: "b", AccessKey: "old-key", SecretKey: "old-secret"},
		},
		{
			name: "empty object",
			in:   `{}`,
			want: S3Config{},
		},
		{
			name: "null",
			in:   `null`,
			want: S3Config{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got S3Config
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestS3Config_UnmarshalJSON_Errors(t *testing.T) {
	for _, in := range []string{`{"bucket":1}`, `{"AccessKey":true}`, `[]`, `{`} {
		var got S3Config
		if err := json.Unmarshal([]byte(in), &got); err == nil {
			t.Errorf("Unmarshal(%s) = nil error, want error", in)
		}
	}
}

func TestS3Config_DecoderUsesSameParsing(t *testing.T) {
	// The test-s3 handler decodes the request body with json.Decoder.
	var got S3Config
	if err := json.NewDecoder(strings.NewReader(`{"bucket":"b","access_key":"k","secret_key":"s"}`)).Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.AccessKey != "k" || got.SecretKey != "s" || got.Bucket != "b" {
		t.Errorf("got %+v", got)
	}
}

func TestS3Config_MarshalRoundTrip(t *testing.T) {
	in := S3Config{Endpoint: "https://s3.example.com", Bucket: "b", Prefix: "p", AccessKey: "k", SecretKey: "s", Region: "r"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{`"endpoint"`, `"bucket"`, `"prefix"`, `"access_key"`, `"secret_key"`, `"region"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("marshalled JSON %s missing key %s", b, key)
		}
	}
	var out S3Config
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip got %+v, want %+v", out, in)
	}
}
