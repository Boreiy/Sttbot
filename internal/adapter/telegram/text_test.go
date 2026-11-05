package telegram

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSplitText(t *testing.T) {
	for _, input := range []string{"", "Привет", strings.Repeat("я", 9000), strings.Repeat("😀", 5000), strings.Repeat("a😀\n", 3000)} {
		chunks := SplitText(input)
		if strings.Join(chunks, "") != input {
			t.Fatal("text changed")
		}
		for _, chunk := range chunks {
			if n := len(utf16.Encode([]rune(chunk))); n > 4096 || n == 0 {
				t.Fatalf("invalid chunk size %d", n)
			}
		}
	}
}
