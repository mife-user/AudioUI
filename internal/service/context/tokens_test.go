package context

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEstimateTokensMixedText(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 1},
		{"abcd", 2},
		{"abc 123!", 4},
		{"中文hello!", 7},
		{"😀", 2},
		{"a\nb", 3},
	}
	for _, tt := range cases {
		if got := EstimateTokens(tt.text); got != tt.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestExcerptUsesSameEstimatorAndKeepsUTF8(t *testing.T) {
	text := strings.Repeat("中文 mixed! 😀", 600)
	got, truncated := excerpt(text, ExcerptTokens)
	if !truncated || !utf8.ValidString(got) || !strings.HasPrefix(text, got) {
		t.Fatalf("invalid excerpt: truncated=%v utf8=%v", truncated, utf8.ValidString(got))
	}
	if count := EstimateTokens(got); count > ExcerptTokens {
		t.Fatalf("excerpt uses %d tokens", count)
	}
	_, width := utf8.DecodeRuneInString(text[len(got):])
	if EstimateTokens(text[:len(got)+width]) <= ExcerptTokens {
		t.Fatal("excerpt could include another rune")
	}
	if full, cut := excerpt("短内容", 100); cut || full != "短内容" {
		t.Fatalf("short excerpt = %q, truncated=%v", full, cut)
	}
	if empty, cut := excerpt("内容", 0); !cut || empty != "" {
		t.Fatalf("zero-limit excerpt = %q, truncated=%v", empty, cut)
	}
}
