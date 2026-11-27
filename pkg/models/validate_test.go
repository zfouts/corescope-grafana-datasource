package models

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSettings(t *testing.T) {
	const secret = "s3cr3t-value"
	tests := []struct {
		name        string
		in          PluginSettings
		wantErr     error
		wantURL     string
		wantTimeout int
	}{
		{name: "plain https", in: PluginSettings{URL: "https://analyzer.example.net"}, wantURL: "https://analyzer.example.net", wantTimeout: DefaultTimeoutSeconds},
		{name: "trailing slash trimmed", in: PluginSettings{URL: "https://analyzer.example.net/"}, wantURL: "https://analyzer.example.net", wantTimeout: DefaultTimeoutSeconds},
		{name: "base path kept, slashes trimmed", in: PluginSettings{URL: " https://host/corescope// "}, wantURL: "https://host/corescope", wantTimeout: DefaultTimeoutSeconds},
		{name: "port kept", in: PluginSettings{URL: "http://host:3000"}, wantURL: "http://host:3000", wantTimeout: DefaultTimeoutSeconds},
		{name: "empty", in: PluginSettings{URL: "  "}, wantErr: ErrURLRequired},
		{name: "unparseable", in: PluginSettings{URL: "http://[::1"}, wantErr: ErrURLInvalid},
		{name: "ftp scheme", in: PluginSettings{URL: "ftp://host"}, wantErr: ErrURLScheme},
		{name: "no scheme", in: PluginSettings{URL: "host/api"}, wantErr: ErrURLScheme},
		{name: "empty host", in: PluginSettings{URL: "https://"}, wantErr: ErrURLHost},
		{name: "port only host", in: PluginSettings{URL: "https://:443"}, wantErr: ErrURLHost},
		{name: "opaque", in: PluginSettings{URL: "https:host"}, wantErr: ErrURLHost},
		{name: "userinfo", in: PluginSettings{URL: "https://user:" + secret + "@host"}, wantErr: ErrURLUserinfo},
		{name: "user only", in: PluginSettings{URL: "https://user@host"}, wantErr: ErrURLUserinfo},
		{name: "query", in: PluginSettings{URL: "https://host/?x=1"}, wantErr: ErrURLQuery},
		{name: "empty query", in: PluginSettings{URL: "https://host/a?"}, wantErr: ErrURLQuery},
		{name: "fragment", in: PluginSettings{URL: "https://host/#frag"}, wantErr: ErrURLFragment},
		{name: "empty fragment", in: PluginSettings{URL: "https://host#"}, wantErr: ErrURLFragment},
		{
			name:    "both auth kinds",
			in:      PluginSettings{URL: "https://host", BasicAuthUser: "u", Secrets: &SecretPluginSettings{BasicAuthPassword: secret, BearerToken: secret}},
			wantErr: ErrAuthConflict,
		},
		{
			name:    "password and token, no user",
			in:      PluginSettings{URL: "https://host", Secrets: &SecretPluginSettings{BasicAuthPassword: secret, BearerToken: secret}},
			wantErr: ErrAuthConflict,
		},
		{
			name:    "password without user",
			in:      PluginSettings{URL: "https://host", Secrets: &SecretPluginSettings{BasicAuthPassword: secret}},
			wantErr: ErrPasswordWithoutUser,
		},
		{
			name: "basic auth over https", wantURL: "https://host", wantTimeout: DefaultTimeoutSeconds,
			in: PluginSettings{URL: "https://host", BasicAuthUser: "u", Secrets: &SecretPluginSettings{BasicAuthPassword: secret}},
		},
		{
			name:    "bearer over http to non-loopback refused",
			in:      PluginSettings{URL: "http://analyzer.example.net", Secrets: &SecretPluginSettings{BearerToken: secret}},
			wantErr: ErrInsecureCredentials,
		},
		{
			name:    "basic user over http to non-loopback refused",
			in:      PluginSettings{URL: "http://10.0.0.5:3000", BasicAuthUser: "u"},
			wantErr: ErrInsecureCredentials,
		},
		{
			name:    "localhost lookalike is not loopback",
			in:      PluginSettings{URL: "http://localhost.example.net", Secrets: &SecretPluginSettings{BearerToken: secret}},
			wantErr: ErrInsecureCredentials,
		},
		{
			name: "http creds allowed with opt-in", wantURL: "http://analyzer.example.net", wantTimeout: DefaultTimeoutSeconds,
			in: PluginSettings{URL: "http://analyzer.example.net", AllowInsecureCredentials: true, Secrets: &SecretPluginSettings{BearerToken: secret}},
		},
		{
			name: "http creds to 127.0.0.1 allowed", wantURL: "http://127.0.0.1:3000", wantTimeout: DefaultTimeoutSeconds,
			in: PluginSettings{URL: "http://127.0.0.1:3000", BasicAuthUser: "u", Secrets: &SecretPluginSettings{BasicAuthPassword: secret}},
		},
		{
			name: "http creds to localhost allowed", wantURL: "http://LocalHost:3000", wantTimeout: DefaultTimeoutSeconds,
			in: PluginSettings{URL: "http://LocalHost:3000", Secrets: &SecretPluginSettings{BearerToken: secret}},
		},
		{
			name: "http creds to ::1 allowed", wantURL: "http://[::1]:3000", wantTimeout: DefaultTimeoutSeconds,
			in: PluginSettings{URL: "http://[::1]:3000", Secrets: &SecretPluginSettings{BearerToken: secret}},
		},
		{name: "http without creds fine", in: PluginSettings{URL: "http://analyzer.example.net"}, wantURL: "http://analyzer.example.net", wantTimeout: DefaultTimeoutSeconds},
		{name: "timeout default", in: PluginSettings{URL: "https://h", TimeoutSeconds: 0}, wantURL: "https://h", wantTimeout: 15},
		{name: "timeout negative clamps to 1", in: PluginSettings{URL: "https://h", TimeoutSeconds: -5}, wantURL: "https://h", wantTimeout: 1},
		{name: "timeout 1 kept", in: PluginSettings{URL: "https://h", TimeoutSeconds: 1}, wantURL: "https://h", wantTimeout: 1},
		{name: "timeout 60 kept", in: PluginSettings{URL: "https://h", TimeoutSeconds: 60}, wantURL: "https://h", wantTimeout: 60},
		{name: "timeout over clamps to 60", in: PluginSettings{URL: "https://h", TimeoutSeconds: 600}, wantURL: "https://h", wantTimeout: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.in
			err := ValidateSettings(&s)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaks a secret: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if s.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", s.URL, tt.wantURL)
			}
			if s.TimeoutSeconds != tt.wantTimeout {
				t.Errorf("TimeoutSeconds = %d, want %d", s.TimeoutSeconds, tt.wantTimeout)
			}
		})
	}
}

func TestValidateSettingsNil(t *testing.T) {
	if err := ValidateSettings(nil); !errors.Is(err, ErrNilSettings) {
		t.Fatalf("err = %v", err)
	}
}
