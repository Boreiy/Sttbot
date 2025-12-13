package middleware

import "testing"

func TestParseAllowedIDs(t *testing.T) {
    ids := ParseAllowedIDs("1, 2,3,\n4")
    if len(ids) != 4 { t.Fatalf("len=%d", len(ids)) }
    want := []int64{1, 2, 3, 4}
    for i := range want {
        if ids[i] != want[i] { t.Fatalf("idx %d: got %d want %d", i, ids[i], want[i]) }
    }
}

func TestACL_IsAllowed(t *testing.T) {
    a := NewACL([]int64{10, 20, 30})
    if !a.IsAllowed(10) { t.Fatalf("expected allowed") }
    if a.IsAllowed(11) { t.Fatalf("expected denied") }
}

