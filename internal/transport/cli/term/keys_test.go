package term

import (
	"strings"
	"testing"
)

func TestDecodeKeyCoversCommonTerminalVariants(t *testing.T) {
	for seq, want := range map[string]Key{
		"\r": KeySubmit, "\x1bOM": KeySubmit,
		"\n": KeyNewline, "\x1b[13;2u": KeyNewline, "\x1b[27;2;13~": KeyNewline, "\x1b\r": KeyNewline,
		"\x1b[A": KeyUp, "\x1bOA": KeyUp, "\x1b\x1b[A": KeyUp, "\x10": KeyUp, "\x1b[B": KeyDown, "\x0e": KeyDown,
		"\x1b[D": KeyLeft, "\x02": KeyLeft, "\x1bOC": KeyRight, "\x06": KeyRight,
		"\x1b[1;5D": KeyWordLeft, "\x1bb": KeyWordLeft, "\x1b[1;3C": KeyWordRight, "\x1bf": KeyWordRight,
		"\x1b[H": KeyLineStart, "\x01": KeyLineStart, "\x1b[4~": KeyLineEnd, "\x05": KeyLineEnd,
		"\x1b[1;5H": KeyTextStart, "\x1b>": KeyTextEnd,
		"\x17": KeyDeleteBigWordBackward, "\x1b\x7f": KeyDeleteWordBackward, "\x1bd": KeyDeleteWordForward,
		"\x0b": KeyKillToLineEnd, "\x15": KeyKillToLineStart,
		"\x7f": KeyBackspace, "\b": KeyBackspace, "\x1b[3~": KeyDelete, "\x04": KeyEOF, "\t": KeyTab,
		"\x1b": KeyEscape, "\x1b\x1b": KeyEscape, "\x1b[200~": KeyPasteStart, "\x1b[201~": KeyPasteEnd,
		"\x1b[<0;3;4M": KeyNone, "\x1b[1;2D": KeyNone, "a": KeyNone, "": KeyNone,
	} {
		if got := DecodeKey(seq); got != want {
			t.Errorf("%q decoded to %q, want %q", seq, got, want)
		}
	}
}

func TestIncompleteKeyFollowsEscapeSequenceFraming(t *testing.T) {
	for _, key := range []string{"\x1b", "\x1b[", "\x1bO", "\x1b[1", "\x1b[1;", "\x1b[1;5", "\x1b[<0;3", "\x1b[200", "\x1b\x1b", "\x1b\x1b["} {
		if !IncompleteKey(key) {
			t.Errorf("%q was treated as complete", key)
		}
	}
	for _, key := range []string{"", "a", "\x1b[A", "\x1bOA", "\x1b[1;5D", "\x1b[3~", "\x1b[<0;3;4M", "\x1b[<0;3;4m", "\x1b[200~", "\x1b[13;2u", "\x1bb", "\x1b\x7f", "\x1b\x1b[A", "\x1b[" + strings.Repeat("1", 70)} {
		if IncompleteKey(key) {
			t.Errorf("%q was treated as incomplete", key)
		}
	}
}
