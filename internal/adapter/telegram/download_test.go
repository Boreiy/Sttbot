package telegram

import "testing"

func TestIsSupportedAudio(t *testing.T) {
	cases := []struct {
		mime, name string
		ok         bool
	}{
		{"audio/ogg", "a.ogg", true},
		{"audio/mpeg", "a.mp3", true},
		{"application/zip", "a.zip", false},
		{"video/mp4", "a.mp4", false},
		{"", "a.webm", true},
		{"", "a.txt", false},
	}
	for i, c := range cases {
		if got := IsSupportedAudio(c.mime, c.name); got != c.ok {
			t.Fatalf("case %d: got %v want %v", i, got, c.ok)
		}
	}
}

func TestNormalizeOGGName(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"file.oga", "file.ogg"},
		{"voice/file_0.oga", "voice/file_0.ogg"},
		{"audio.ogg", "audio.ogg"},
		{"doc.mp3", "doc.mp3"},
	}

	for i, c := range cases {
		if got := normalizeOGGName(c.in); got != c.out {
			t.Fatalf("case %d: got %s want %s", i, got, c.out)
		}
	}
}
