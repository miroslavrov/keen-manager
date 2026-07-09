package engine

import (
	"reflect"
	"testing"
)

func TestNormalizeFailoverChain(t *testing.T) {
	conns := []string{"a", "b", "c"}

	t.Run("trims, dedupes, preserves order, flags unknown", func(t *testing.T) {
		clean, unknown := NormalizeFailoverChain([]string{" a ", "b", "b", "x", "", "direct"}, conns)
		if want := []string{"a", "b", "x", "direct"}; !reflect.DeepEqual(clean, want) {
			t.Fatalf("clean = %v, want %v", clean, want)
		}
		if want := []string{"x"}; !reflect.DeepEqual(unknown, want) {
			t.Fatalf("unknown = %v, want %v", unknown, want)
		}
	})

	t.Run("all known yields no unknown", func(t *testing.T) {
		clean, unknown := NormalizeFailoverChain([]string{"c", "a", "direct"}, conns)
		if want := []string{"c", "a", "direct"}; !reflect.DeepEqual(clean, want) {
			t.Fatalf("clean = %v, want %v", clean, want)
		}
		if len(unknown) != 0 {
			t.Fatalf("unknown = %v, want empty", unknown)
		}
	})

	t.Run("empty chain is empty", func(t *testing.T) {
		clean, unknown := NormalizeFailoverChain(nil, conns)
		if len(clean) != 0 || len(unknown) != 0 {
			t.Fatalf("clean=%v unknown=%v, want both empty", clean, unknown)
		}
	})
}
