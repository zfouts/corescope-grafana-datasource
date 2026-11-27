package models

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// MaxTimeoutSeconds is the upper clamp for PluginSettings.TimeoutSeconds.
const MaxTimeoutSeconds = 60

// Validation errors. The text is fixed and never contains a setting's value,
// so it is safe to show to editors and to log.
var (
	ErrNilSettings         = errors.New("datasource settings are missing")
	ErrURLRequired         = errors.New("url is required")
	ErrURLInvalid          = errors.New("url is not a valid URL")
	ErrURLScheme           = errors.New("url scheme must be http or https")
	ErrURLHost             = errors.New("url must include a host")
	ErrURLUserinfo         = errors.New("url must not contain a user or password; use the basic auth settings")
	ErrURLQuery            = errors.New("url must not contain a query string")
	ErrURLFragment         = errors.New("url must not contain a fragment")
	ErrAuthConflict        = errors.New("basic auth and bearer token are mutually exclusive; configure only one")
	ErrPasswordWithoutUser = errors.New("basic auth password is set but basic auth user is empty")
	ErrInsecureCredentials = errors.New("refusing to send credentials over http:// to a non-loopback host; use https or enable allowInsecureCredentials")
)

// ValidateSettings checks s against the rules documented on PluginSettings and
// normalizes it in place: URL loses any trailing "/" (a base path such as
// https://host/corescope is kept), and TimeoutSeconds is clamped (0 means
// DefaultTimeoutSeconds, otherwise 1..MaxTimeoutSeconds).
func ValidateSettings(s *PluginSettings) error {
	if s == nil {
		return ErrNilSettings
	}

	raw := strings.TrimRight(strings.TrimSpace(s.URL), "/")
	if raw == "" {
		return ErrURLRequired
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ErrURLInvalid
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrURLScheme
	}
	if u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return ErrURLHost
	}
	if u.User != nil {
		return ErrURLUserinfo
	}
	if u.RawQuery != "" || u.ForceQuery {
		return ErrURLQuery
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return ErrURLFragment
	}

	var password, token string
	if s.Secrets != nil {
		password = s.Secrets.BasicAuthPassword
		token = s.Secrets.BearerToken
	}
	basic := s.BasicAuthUser != "" || password != ""
	if basic && token != "" {
		return ErrAuthConflict
	}
	if password != "" && s.BasicAuthUser == "" {
		return ErrPasswordWithoutUser
	}
	if (basic || token != "") && u.Scheme == "http" && !isLoopback(u.Hostname()) && !s.AllowInsecureCredentials {
		return ErrInsecureCredentials
	}

	switch {
	case s.TimeoutSeconds == 0:
		s.TimeoutSeconds = DefaultTimeoutSeconds
	case s.TimeoutSeconds < 1:
		s.TimeoutSeconds = 1
	case s.TimeoutSeconds > MaxTimeoutSeconds:
		s.TimeoutSeconds = MaxTimeoutSeconds
	}
	s.URL = u.String()
	return nil
}

// isLoopback reports whether host is "localhost" or a loopback IP literal.
// Other names are not resolved: a DNS answer is not proof of loopback.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
