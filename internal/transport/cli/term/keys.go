package term

// Key is an editing action decoded from one terminal input sequence.
type Key string

const (
	KeyNone                  Key = ""
	KeySubmit                Key = "submit"
	KeyNewline               Key = "newline"
	KeyTab                   Key = "tab"
	KeyEscape                Key = "escape"
	KeyEOF                   Key = "eof"
	KeyBackspace             Key = "backspace"
	KeyDelete                Key = "delete"
	KeyDeleteWordBackward    Key = "delete-word-backward"
	KeyDeleteBigWordBackward Key = "delete-big-word-backward"
	KeyDeleteWordForward     Key = "delete-word-forward"
	KeyKillToLineEnd         Key = "kill-to-line-end"
	KeyKillToLineStart       Key = "kill-to-line-start"
	KeyUp                    Key = "up"
	KeyDown                  Key = "down"
	KeyLeft                  Key = "left"
	KeyRight                 Key = "right"
	KeyWordLeft              Key = "word-left"
	KeyWordRight             Key = "word-right"
	KeyLineStart             Key = "line-start"
	KeyLineEnd               Key = "line-end"
	KeyTextStart             Key = "text-start"
	KeyTextEnd               Key = "text-end"
	KeyPasteStart            Key = "paste-start"
	KeyPasteEnd              Key = "paste-end"
)

// IncompleteKey reports whether an escape sequence read so far may still be
// extended by further bytes. CSI sequences end at their final byte, SS3
// sequences take exactly one more byte, and any other byte after Escape is
// an Alt chord. A lone Escape is resolved by the caller once input pauses.
func IncompleteKey(key string) bool {
	if key == "" || key[0] != 0x1b {
		return false
	}
	if len(key) == 1 {
		return true
	}
	switch key[1] {
	case 0x1b:
		return IncompleteKey(key[1:])
	case '[':
		if len(key) == 2 {
			return true
		}
		final := key[len(key)-1]
		return len(key) < 64 && (final < 0x40 || final > 0x7e)
	case 'O':
		return len(key) == 2
	}
	return false
}

// DecodeKey maps the sequences common terminals send, with Enter kept apart
// from Ctrl+J, to editing actions. Shift+Enter arrives as a CSI u or
// modifyOtherKeys sequence from terminals that distinguish it, Alt+Enter as
// an Escape-prefixed Enter. Unbound sequences decode to KeyNone.
func DecodeKey(seq string) Key {
	for len(seq) > 1 && seq[0] == 0x1b && seq[1] == 0x1b {
		seq = seq[1:]
	}
	switch seq {
	case "\r", "\x1bOM":
		return KeySubmit
	case "\n", "\x1b\r", "\x1b\n", "\x1b[13;2u", "\x1b[13;3u", "\x1b[13;5u", "\x1b[27;2;13~", "\x1b[27;3;13~", "\x1b[27;5;13~":
		return KeyNewline
	case "\t":
		return KeyTab
	case "\x1b":
		return KeyEscape
	case "\x04":
		return KeyEOF
	case "\x7f", "\b":
		return KeyBackspace
	case "\x1b[3~":
		return KeyDelete
	case "\x17":
		return KeyDeleteBigWordBackward
	case "\x1b\x7f", "\x1b\b":
		return KeyDeleteWordBackward
	case "\x1bd", "\x1b[3;3~", "\x1b[3;5~":
		return KeyDeleteWordForward
	case "\x0b":
		return KeyKillToLineEnd
	case "\x15":
		return KeyKillToLineStart
	case "\x1b[A", "\x1bOA", "\x10":
		return KeyUp
	case "\x1b[B", "\x1bOB", "\x0e":
		return KeyDown
	case "\x1b[D", "\x1bOD", "\x02":
		return KeyLeft
	case "\x1b[C", "\x1bOC", "\x06":
		return KeyRight
	case "\x1b[1;3D", "\x1b[1;5D", "\x1bb":
		return KeyWordLeft
	case "\x1b[1;3C", "\x1b[1;5C", "\x1bf":
		return KeyWordRight
	case "\x1b[H", "\x1bOH", "\x1b[1~", "\x1b[7~", "\x01":
		return KeyLineStart
	case "\x1b[F", "\x1bOF", "\x1b[4~", "\x1b[8~", "\x05":
		return KeyLineEnd
	case "\x1b[1;5H", "\x1b<":
		return KeyTextStart
	case "\x1b[1;5F", "\x1b>":
		return KeyTextEnd
	case "\x1b[200~":
		return KeyPasteStart
	case "\x1b[201~":
		return KeyPasteEnd
	}
	return KeyNone
}
