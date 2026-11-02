package claudecli

import "testing"

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		got, min string
		want     bool
	}{
		{"2.1.224", "2.1.224", true},
		{"2.2.0", "2.1.224", true},
		{"2.1.223", "2.1.224", false},
		{"3.0.0", "2.1.224", true},
		{"bad", "2.1.224", false},
	}
	for _, item := range cases {
		if got := versionAtLeast(item.got, item.min); got != item.want {
			t.Fatalf("versionAtLeast(%q,%q) = %v, want %v", item.got, item.min, got, item.want)
		}
	}
}
