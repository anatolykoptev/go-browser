package browser

import "testing"

func TestRefMap_StoreAndResolve(t *testing.T) {
	rm := NewRefMap()
	rm.Store("e1", 42)
	rm.Store("e2", 99)

	got, ok := rm.Resolve("e1")
	if !ok || got != 42 {
		t.Fatalf("Resolve(e1) = %d, %v; want 42, true", got, ok)
	}
	got, ok = rm.Resolve("e2")
	if !ok || got != 99 {
		t.Fatalf("Resolve(e2) = %d, %v; want 99, true", got, ok)
	}
	_, ok = rm.Resolve("e3")
	if ok {
		t.Fatal("Resolve(e3) should return false")
	}
}

func TestRefMap_Clear(t *testing.T) {
	rm := NewRefMap()
	rm.Store("e1", 10)
	rm.Clear()

	_, ok := rm.Resolve("e1")
	if ok {
		t.Fatal("after Clear, Resolve should return false")
	}
}

func TestRefMap_ParseRef(t *testing.T) {
	tests := []struct {
		sel   string
		ref   string
		isRef bool
	}{
		{"ref=e1", "e1", true},
		{"ref=e42", "e42", true},
		{"#myid", "", false},
		{"text=Hello", "", false},
		{"xpath=//div", "", false},
	}
	for _, tt := range tests {
		ref, ok := ParseRef(tt.sel)
		if ok != tt.isRef || ref != tt.ref {
			t.Errorf("ParseRef(%q) = %q, %v; want %q, %v", tt.sel, ref, ok, tt.ref, tt.isRef)
		}
	}
}
