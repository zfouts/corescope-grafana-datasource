package plugin

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const pk64 = "988396af0ff369c2178a8c2a303661bf26193257136ed4452b6eadd9547011e9"

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want QueryModel
		bad  string // expected QueryError.Field; "" = success
	}{
		{"stats", `{"queryType":"stats"}`, QueryModel{QueryType: "stats"}, ""},
		{"unknown type", `{"queryType":"bogus"}`, QueryModel{}, "queryType"},
		{"empty type", `{"queryType":""}`, QueryModel{}, "queryType"},
		{"missing type", `{}`, QueryModel{}, "queryType"},
		{"malformed json", `{"queryType":`, QueryModel{}, "query"},
		{"wrong json type", `{"queryType":5}`, QueryModel{}, "query"},
		{"not an object", `[1]`, QueryModel{}, "query"},

		{"observerMetrics ok", `{"queryType":"observerMetrics","observerId":"AB_c-1","resolution":"1h"}`,
			QueryModel{QueryType: "observerMetrics", ObserverID: "AB_c-1", Resolution: "1h"}, ""},
		{"observerMetrics auto", `{"queryType":"observerMetrics","observerId":"x"}`,
			QueryModel{QueryType: "observerMetrics", ObserverID: "x"}, ""},
		{"observerId missing", `{"queryType":"observerMetrics"}`, QueryModel{}, "observerId"},
		{"observerId dotdot", `{"queryType":"observerMetrics","observerId":".."}`, QueryModel{}, "observerId"},
		{"observerId slash", `{"queryType":"observerMetrics","observerId":"a/b"}`, QueryModel{}, "observerId"},
		{"observerId pct", `{"queryType":"observerMetrics","observerId":"%2e%2e"}`, QueryModel{}, "observerId"},
		{"observerId query", `{"queryType":"observerMetrics","observerId":"x?y"}`, QueryModel{}, "observerId"},
		{"observerId too long", `{"queryType":"observerMetrics","observerId":"` + strings.Repeat("a", 129) + `"}`, QueryModel{}, "observerId"},
		{"observerId 128 ok", `{"queryType":"observerMetrics","observerId":"` + strings.Repeat("a", 128) + `"}`,
			QueryModel{QueryType: "observerMetrics", ObserverID: strings.Repeat("a", 128)}, ""},
		{"resolution bad", `{"queryType":"observerMetrics","observerId":"x","resolution":"2h"}`, QueryModel{}, "resolution"},

		{"pubkey ok", `{"queryType":"nodeActivity","nodePubkey":"` + pk64 + `"}`,
			QueryModel{QueryType: "nodeActivity", NodePubkey: pk64}, ""},
		{"pubkey upper lowercased", `{"queryType":"nodeBattery","nodePubkey":"` + strings.ToUpper(pk64) + `"}`,
			QueryModel{QueryType: "nodeBattery", NodePubkey: pk64}, ""},
		{"pubkey trailing space", `{"queryType":"nodeActivity","nodePubkey":" ` + pk64 + ` \n"}`,
			QueryModel{QueryType: "nodeActivity", NodePubkey: pk64}, ""},
		{"observerId trailing space", `{"queryType":"observerMetrics","observerId":" AB_c-1 "}`,
			QueryModel{QueryType: "observerMetrics", ObserverID: "AB_c-1"}, ""},
		{"search bidi stripped", `{"queryType":"nodes","search":"a\u202eb\u200bc"}`, QueryModel{QueryType: "nodes", Search: "abc", Limit: 100}, ""},
		{"pubkey 63", `{"queryType":"nodeActivity","nodePubkey":"` + pk64[:63] + `"}`, QueryModel{}, "nodePubkey"},
		{"pubkey 65", `{"queryType":"nodeActivity","nodePubkey":"` + pk64 + `a"}`, QueryModel{}, "nodePubkey"},
		{"pubkey non-hex", `{"queryType":"nodeActivity","nodePubkey":"` + strings.Repeat("g", 64) + `"}`, QueryModel{}, "nodePubkey"},
		{"pubkey missing", `{"queryType":"nodeBattery"}`, QueryModel{}, "nodePubkey"},

		{"region single", `{"queryType":"packetsPerHour","region":"OAK"}`, QueryModel{QueryType: "packetsPerHour", Region: "OAK"}, ""},
		{"region multi", `{"queryType":"payloadTypes","region":"OAK,SJC"}`, QueryModel{QueryType: "payloadTypes", Region: "OAK,SJC"}, ""},
		{"region All", `{"queryType":"retransmissions","region":"All"}`, QueryModel{QueryType: "retransmissions"}, ""},
		{"region $__all", `{"queryType":"nodes","region":"$__all"}`, QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"region empty", `{"queryType":"nodes","region":""}`, QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"region injection", `{"queryType":"nodes","region":"OAK;DROP"}`, QueryModel{}, "region"},
		{"region trailing comma", `{"queryType":"nodes","region":"OAK,"}`, QueryModel{}, "region"},
		{"region too long item", `{"queryType":"nodes","region":"` + strings.Repeat("a", 17) + `"}`, QueryModel{}, "region"},
		{"region 16 items ok", `{"queryType":"nodes","region":"` + strings.Repeat("a,", 15) + `a"}`,
			QueryModel{QueryType: "nodes", Region: strings.Repeat("a,", 15) + "a", Limit: 100}, ""},
		{"region 17 items", `{"queryType":"nodes","region":"` + strings.Repeat("a,", 16) + `a"}`, QueryModel{}, "region"},
		{"region ignored for stats", `{"queryType":"stats","region":"OAK;DROP"}`, QueryModel{QueryType: "stats"}, ""},

		{"role ok", `{"queryType":"nodes","role":"repeater"}`, QueryModel{QueryType: "nodes", Role: "repeater", Limit: 100}, ""},
		{"role bad", `{"queryType":"nodes","role":"admin"}`, QueryModel{}, "role"},
		{"role All", `{"queryType":"nodes","role":"All"}`, QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"role $__all", `{"queryType":"nodes","role":"$__all"}`, QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"role case", `{"queryType":"nodes","role":"Repeater"}`, QueryModel{}, "role"},

		{"limit default", `{"queryType":"nodes"}`, QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"limit clamp high", `{"queryType":"nodes","limit":9999}`, QueryModel{QueryType: "nodes", Limit: 500}, ""},
		{"limit negative", `{"queryType":"nodes","limit":-5}`, QueryModel{QueryType: "nodes", Limit: 1}, ""},
		{"limit mid", `{"queryType":"nodes","limit":42}`, QueryModel{QueryType: "nodes", Limit: 42}, ""},

		{"search trimmed+stripped", `{"queryType":"nodes","search":"  ab\u0007c\n "}`, QueryModel{QueryType: "nodes", Search: "abc", Limit: 100}, ""},
		{"search 64 runes", `{"queryType":"nodes","search":"` + strings.Repeat("é", 100) + `"}`,
			QueryModel{QueryType: "nodes", Search: strings.Repeat("é", 64), Limit: 100}, ""},

		{"unused fields zeroed", `{"queryType":"observers","observerId":"x","nodePubkey":"y","region":"OAK","role":"room","resolution":"1h","limit":7,"search":"s"}`,
			QueryModel{QueryType: "observers"}, ""},
		{"nodes zeroes foreign ids", `{"queryType":"nodes","observerId":"x","nodePubkey":"y","resolution":"1h"}`,
			QueryModel{QueryType: "nodes", Limit: 100}, ""},
		{"observerMetrics zeroes nodes opts", `{"queryType":"observerMetrics","observerId":"x","region":"OAK","role":"room","limit":7,"search":"s"}`,
			QueryModel{QueryType: "observerMetrics", ObserverID: "x"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseQuery(json.RawMessage(tc.in))
			if tc.bad != "" {
				var qe *QueryError
				if !errors.As(err, &qe) {
					t.Fatalf("want QueryError, got %v", err)
				}
				if qe.Field != tc.bad {
					t.Fatalf("field = %q, want %q", qe.Field, tc.bad)
				}
				if err.Error() != "invalid "+tc.bad {
					t.Fatalf("error string = %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseQueryErrorNeverContainsValue(t *testing.T) {
	for _, bad := range []string{"..", "a/b", "%2e%2e", "x?y", "SECRET;DROP"} {
		for _, in := range []string{
			`{"queryType":"observerMetrics","observerId":"` + bad + `"}`,
			`{"queryType":"nodeActivity","nodePubkey":"` + bad + `"}`,
			`{"queryType":"nodes","region":"` + bad + `"}`,
			`{"queryType":"` + bad + `"}`,
		} {
			_, err := ParseQuery(json.RawMessage(in))
			if err == nil {
				t.Fatalf("%s: expected error", in)
			}
			if strings.Contains(err.Error(), bad) {
				t.Fatalf("error %q contains value %q", err, bad)
			}
		}
	}
}
