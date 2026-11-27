package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// PluginSettings is the datasource instance configuration.
//
// jsonData (plain, visible to editors):
//
//	url                       CoreScope base URL, e.g. https://analyzer.example.net
//	timeoutSeconds            per-request budget incl. retries; clamp 1..60, default 15
//	basicAuthUser             user for HTTP basic auth (password is secret)
//	tlsSkipVerify             admin opt-in; CheckHealth warns when set
//	allowInsecureCredentials  allow credentials over http:// to a non-loopback host
//	meshOnly                  refuse the server-internals query types (health,
//	                          endpoints, mqtt) and drop server fields from stats;
//	                          for a datasource behind a public dashboard
//
// secureJsonData (write-only, decrypted only in the backend):
//
//	basicAuthPassword
//	bearerToken
//
// Validation (ValidateSettings, called by NewDatasource and CheckHealth):
//   - url parses; scheme is http or https; host non-empty; no userinfo,
//     query or fragment. Stored normalized with no trailing "/".
//   - basic auth (user or password) and bearerToken are mutually exclusive.
//   - credentials over http:// to a non-loopback host are refused unless
//     allowInsecureCredentials.
//
// The plugin never sends X-API-Key (CoreScope's admin key).
type PluginSettings struct {
	URL                      string `json:"url"`
	TimeoutSeconds           int    `json:"timeoutSeconds"`
	BasicAuthUser            string `json:"basicAuthUser"`
	TLSSkipVerify            bool   `json:"tlsSkipVerify"`
	AllowInsecureCredentials bool   `json:"allowInsecureCredentials"`
	MeshOnly                 bool   `json:"meshOnly"`

	Secrets *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds secureJsonData. Never log or format it.
type SecretPluginSettings struct {
	BasicAuthPassword string
	BearerToken       string
}

// DefaultTimeoutSeconds applies when timeoutSeconds is 0.
const DefaultTimeoutSeconds = 15

func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{}
	if len(source.JSONData) > 0 {
		if err := json.Unmarshal(source.JSONData, &settings); err != nil {
			return nil, fmt.Errorf("could not parse datasource settings: %w", err)
		}
	}
	settings.Secrets = &SecretPluginSettings{
		BasicAuthPassword: source.DecryptedSecureJSONData["basicAuthPassword"],
		BearerToken:       source.DecryptedSecureJSONData["bearerToken"],
	}
	return &settings, nil
}
