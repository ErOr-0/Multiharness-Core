package opencode

import "bytes"

// unwrapJSONFence accepts the presentation wrapper some OpenCode models add to
// their final response. Only a complete, standalone ```json or unlabelled fence
// is removed, once. The caller must still strictly decode and validate the JSON;
// this does not extract JSON from prose, repair it, or reinterpret its contents.
func unwrapJSONFence(data []byte) []byte {
	trimmed := bytes.Trim(data, " \t\r\n")
	header, rest, ok := bytes.Cut(trimmed, []byte("\n"))
	if !ok {
		return data
	}
	header = bytes.TrimRight(header, " \t\r")
	if !bytes.Equal(header, []byte("```json")) && !bytes.Equal(header, []byte("```")) {
		return data
	}
	lastLine := bytes.LastIndexByte(rest, '\n')
	if lastLine < 0 || !bytes.Equal(rest[lastLine+1:], []byte("```")) {
		return data
	}
	return rest[:lastLine]
}
