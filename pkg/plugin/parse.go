package plugin

import (
	"encoding/json"
	"regexp"
	"strings"
)

// QueryError is a validation failure. It names the offending field and never
// carries the value.
type QueryError struct{ Field string }

func (e *QueryError) Error() string { return "invalid " + e.Field }

const (
	defaultLimit = 100
	maxLimit     = 500
	maxSearch    = 64
)

var (
	observerIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	pubkeyRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	regionRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,16}(,[A-Za-z0-9_-]{1,16}){0,15}$`)
)

// ParseQuery decodes and validates one query's JSON. Fields the chosen
// queryType does not use are zeroed in the result.
func ParseQuery(raw json.RawMessage) (QueryModel, error) {
	var in QueryModel
	if err := json.Unmarshal(raw, &in); err != nil {
		return QueryModel{}, &QueryError{Field: "query"}
	}
	out := QueryModel{QueryType: in.QueryType}

	usesRegion, usesNodesOpts := false, false
	switch in.QueryType {
	case QueryStats, QueryHealth, QueryEndpoints, QueryMqtt, QueryObservers:
	case QueryPacketsPerHour, QueryPayloadTypes, QueryRetransmissions:
		usesRegion = true
	case QueryNodes:
		usesRegion, usesNodesOpts = true, true
	case QueryObserverMetrics:
		id := strings.TrimSpace(in.ObserverID)
		if !observerIDRe.MatchString(id) {
			return QueryModel{}, &QueryError{Field: "observerId"}
		}
		out.ObserverID = id
		switch in.Resolution {
		case "", "5m", "1h", "1d":
			out.Resolution = in.Resolution
		default:
			return QueryModel{}, &QueryError{Field: "resolution"}
		}
	case QueryNodeActivity, QueryNodeBattery:
		pk := strings.ToLower(strings.TrimSpace(in.NodePubkey))
		if !pubkeyRe.MatchString(pk) {
			return QueryModel{}, &QueryError{Field: "nodePubkey"}
		}
		out.NodePubkey = pk
	default:
		return QueryModel{}, &QueryError{Field: "queryType"}
	}

	if usesRegion {
		region := in.Region
		if region == "All" || region == "$__all" {
			region = ""
		}
		if region != "" && !regionRe.MatchString(region) {
			return QueryModel{}, &QueryError{Field: "region"}
		}
		out.Region = region
	}

	if usesNodesOpts {
		switch in.Role {
		case "", "All", "$__all":
			// A dashboard variable's All option: no role filter, as for region.
		case "repeater", "companion", "room", "sensor":
			out.Role = in.Role
		default:
			return QueryModel{}, &QueryError{Field: "role"}
		}
		out.Limit = in.Limit
		switch {
		case out.Limit == 0:
			out.Limit = defaultLimit
		case out.Limit < 1:
			out.Limit = 1
		case out.Limit > maxLimit:
			out.Limit = maxLimit
		}
		out.Search = cleanSearch(in.Search)
	}
	return out, nil
}

// cleanSearch strips control characters, trims and caps at maxSearch runes.
func cleanSearch(s string) string {
	t := strings.TrimSpace(strings.Map(func(r rune) rune {
		if isStrippedRune(r) {
			return -1
		}
		return r
	}, s))
	if r := []rune(t); len(r) > maxSearch {
		t = strings.TrimSpace(string(r[:maxSearch]))
	}
	return t
}
