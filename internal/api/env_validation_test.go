package api

import "testing"

func TestValidateEnvVars(t *testing.T) {
	cases := []struct {
		name    string
		vars    []envVar
		wantErr bool
	}{
		{"valid", []envVar{{Key: "DB_URL", Value: "postgres://x:y@h/d"}}, false},
		{"empty key", []envVar{{Key: "", Value: "v"}}, true},
		{"key with =", []envVar{{Key: "A=B", Value: "v"}}, true},
		{"key with space", []envVar{{Key: "A B", Value: "v"}}, true},
		{"key with newline", []envVar{{Key: "A\nB", Value: "v"}}, true},
		{"key with tab", []envVar{{Key: "A\tB", Value: "v"}}, true},
		{"value with newline", []envVar{{Key: "A", Value: "v\nINJECTED=1"}}, true},
		{"value with CR", []envVar{{Key: "A", Value: "v\rx"}}, true},
		{"value with NUL", []envVar{{Key: "A", Value: "v\x00x"}}, true},
		{"value with =", []envVar{{Key: "A", Value: "k=v"}}, false}, // = is fine in values
		{"empty value", []envVar{{Key: "A", Value: ""}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEnvVars(tc.vars)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateEnvVars() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
