package telegram

import "testing"

func TestIsSupportedAudio(t *testing.T) {
    cases := []struct{ mime, name string; ok bool }{
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

