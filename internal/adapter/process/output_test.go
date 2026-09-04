package process

import "testing"

func TestTailBufferRetainsTrailingBytes(t *testing.T) {
	buffer := newTailBuffer(8)
	for _, value := range []string{"abc", "def", "ghijkl"} {
		if _, err := buffer.Write([]byte(value)); err != nil {
			t.Fatalf("Write() returned an error: %v", err)
		}
	}

	if actual := buffer.String(); actual != "efghijkl" {
		t.Fatalf("String() = %q; want %q", actual, "efghijkl")
	}
	if !buffer.Truncated() {
		t.Fatal("Truncated() = false; want true")
	}
}

func TestTailBufferHandlesSingleOversizedWrite(t *testing.T) {
	buffer := newTailBuffer(4)
	if _, err := buffer.Write([]byte("0123456789")); err != nil {
		t.Fatalf("Write() returned an error: %v", err)
	}

	if actual := buffer.String(); actual != "6789" {
		t.Fatalf("String() = %q; want %q", actual, "6789")
	}
	if !buffer.Truncated() {
		t.Fatal("Truncated() = false; want true")
	}
}

func TestTailBufferDoesNotTruncateWithinLimit(t *testing.T) {
	buffer := newTailBuffer(4)
	if _, err := buffer.Write([]byte("1234")); err != nil {
		t.Fatalf("Write() returned an error: %v", err)
	}

	if actual := buffer.String(); actual != "1234" {
		t.Fatalf("String() = %q; want %q", actual, "1234")
	}
	if buffer.Truncated() {
		t.Fatal("Truncated() = true; want false")
	}
}
