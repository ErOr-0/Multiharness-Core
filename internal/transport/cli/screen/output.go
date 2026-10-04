package screen

import "multiharness-core/internal/transport/cli/term"

func (v *View) Print(value string) error {
	return term.Write(v.Writer, v.StyledText(value))
}

func (v *View) StyledText(value string) string {
	if !v.Color {
		return value
	}
	return term.StyleBlock(value, v.TrueColor, v.ContentWidth()+2)
}
