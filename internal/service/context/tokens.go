package context

import "unicode/utf8"

const (
	// EstimatorVersion identifies the deterministic, provider-independent
	// heuristic. It is not an exact tokenizer or a guarantee of CLI usage.
	EstimatorVersion = 1
	ExcerptTokens    = 2_000
)

// EstimateTokens conservatively estimates the rendered prompt: ASCII word
// and horizontal-space runs cost one token per three bytes, punctuation and
// line breaks cost one each, and non-ASCII runes cost one per two UTF-8 bytes
// (rounded up). The real tokenizer and system/tool overhead can differ.
func EstimateTokens(text string) int {
	var counter tokenCounter
	for _, r := range text {
		counter.add(r)
	}
	return counter.total
}

type tokenCounter struct {
	total int
	class byte
	run   int
}

func (c *tokenCounter) add(r rune) {
	class := byte(0)
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		class = 'w'
	case r == ' ' || r == '\t':
		class = 's'
	}
	if class != 0 {
		if c.class != class {
			c.class = class
			c.run = 0
		}
		if c.run%3 == 0 {
			c.total++
		}
		c.run++
		return
	}
	c.class, c.run = 0, 0
	if r < utf8.RuneSelf {
		c.total++
		return
	}
	c.total += (utf8.RuneLen(r) + 1) / 2
}

// excerpt returns a UTF-8-safe prefix within the same token heuristic.
func excerpt(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", text != ""
	}
	var counter tokenCounter
	for i, r := range text {
		next := counter
		next.add(r)
		if next.total > limit {
			return text[:i], true
		}
		counter = next
	}
	return text, false
}
