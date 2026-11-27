package plugin

import (
	"strings"
	"unicode"
)

// MaxStringRunes caps every upstream string placed in a frame.
const MaxStringRunes = 256

// isStrippedRune reports runes removed from every upstream string: control
// (Cc), format (Cf: bidi overrides, zero-width), U+2028/U+2029 and U+FFFD.
func isStrippedRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' || r == '\ufffd'
}

// SafeString strips control and format characters (see isStrippedRune) and
// truncates to MaxStringRunes runes. Apply it to every string from CoreScope
// before it goes into a frame (node names travel over the air, unauthenticated).
func SafeString(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if isStrippedRune(r) {
			continue
		}
		if n == MaxStringRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// SafeStringPtr is SafeString for nullable fields; nil → "".
func SafeStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return SafeString(*s)
}
