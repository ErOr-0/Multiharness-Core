package term

import "strings"

// IncompleteKey reports whether an escape sequence read so far may
// still be extended by further bytes.
func IncompleteKey(key string) bool {
	if key == "\x1b" || key == "\x1b[" {
		return true
	}
	if strings.HasPrefix(key, "\x1b[<") {
		return len(key) < 48 && !strings.HasSuffix(key, "M") && !strings.HasSuffix(key, "m")
	}
	return len(key) < 12 && key[len(key)-1] >= '0' && key[len(key)-1] <= '9'
}
