package valueobject

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateID(t *testing.T) {
	valid := []string{"n_abc", "p_123", "pa_ff", "r_m1_import", "c_n_a"}
	for _, id := range valid {
		if err := ValidateID(id); err != nil {
			t.Fatalf("ValidateID(%q) error = %v", id, err)
		}
	}

	invalid := []string{"", ".", "..", "N_ABC", "n-abc", "n/abc", `n\abc`, "n:a", "con", strings.Repeat("a", MaxIDLength+1)}
	for _, id := range invalid {
		if err := ValidateID(id); err == nil {
			t.Fatalf("ValidateID(%q) error = nil, want error", id)
		} else if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("ValidateID(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestNewIDUsesPrefixAndValidCharacters(t *testing.T) {
	id, err := NewID(NodeIDPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, NodeIDPrefix) {
		t.Fatalf("id = %q, want prefix %q", id, NodeIDPrefix)
	}
	if err := ValidateID(id); err != nil {
		t.Fatalf("generated id invalid: %v", err)
	}
	other, err := NewID(NodeIDPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if id == other {
		t.Fatalf("generated duplicate id %q", id)
	}
}

func TestNewIDRejectsEmptyPrefix(t *testing.T) {
	if _, err := NewID(""); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("error = %v, want ErrInvalidID", err)
	}
}
