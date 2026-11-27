package plugin

import (
	"strings"
	"testing"
)

func TestSafeString(t *testing.T) {
	long := strings.Repeat("é", MaxStringRunes+50)
	tests := []struct{ name, in, want string }{
		{"plain", "hello", "hello"},
		{"html passes through", "<img src=x onerror=alert(1)>", "<img src=x onerror=alert(1)>"},
		{"formula passes through", `=HYPERLINK("x")`, `=HYPERLINK("x")`},
		{"BEL stripped", "a\x07b", "ab"},
		{"newline, tab, NUL, DEL stripped", "a\nb\tc\x00d\x7fe", "abcde"},
		{"C1 control stripped", "a\u0085b", "ab"},
		{"line/paragraph separators stripped", "a b c", "abc"},
		{"bidi override stripped", "adm\u202ein", "admin"},
		{"zero-width stripped", "a\u200bb\u200dc\ufeffd", "abcd"},
		{"replacement char stripped", "a\ufffdb", "ab"},
		{"unicode kept", "héllo 日本", "héllo 日本"},
		{"empty", "", ""},
		{"truncated to 256 runes", long, strings.Repeat("é", MaxStringRunes)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeString(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSafeStringCapCountsKeptRunes(t *testing.T) {
	in := strings.Repeat("\x07", 1000) + strings.Repeat("a", 300)
	if got := SafeString(in); got != strings.Repeat("a", MaxStringRunes) {
		t.Fatalf("got %d runes", len([]rune(got)))
	}
}

func TestSafeStringPtr(t *testing.T) {
	if got := SafeStringPtr(nil); got != "" {
		t.Fatalf("nil → %q", got)
	}
	s := "a\x07b"
	if got := SafeStringPtr(&s); got != "ab" {
		t.Fatalf("got %q", got)
	}
}
