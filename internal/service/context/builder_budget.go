package context

import "context"

func renderWithinBudget(ctx context.Context, body contextBody, ancestors []resolvedAncestor, dag bool, budget int) (string, string, int, error) {
	levels := make([]int, len(ancestors))
	nextAncestor := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", "", 0, err
		}
		if dag {
			body.Sources = renderAncestors(ancestors, levels)
			body.Path = nil
		} else {
			body.Path = renderAncestors(ancestors, levels)
			body.Sources = nil
		}
		prompt, contextHash, estimated, err := renderPrompt(body)
		if err != nil {
			return "", "", 0, err
		}
		if estimated <= budget {
			return prompt, contextHash, estimated, nil
		}
		for nextAncestor < len(levels) && ancestors[nextAncestor].shared {
			nextAncestor++
		}
		if nextAncestor == len(levels) {
			return "", "", 0, &ContextTooLargeError{EstimatedTokens: estimated, TokenBudget: budget}
		}
		levels[nextAncestor]++
		if levels[nextAncestor] == 2 {
			nextAncestor++
		}
	}
}

func renderAncestors(ancestors []resolvedAncestor, levels []int) []ancestorSection {
	sections := make([]ancestorSection, 0, len(ancestors))
	for i, ancestor := range ancestors {
		section := ancestorSection{ancestorIdentity: ancestor.identity}
		if ancestor.shared {
			section.Representation = "shared"
			sections = append(sections, section)
			continue
		}
		switch levels[i] {
		case 0:
			section.Representation = "full"
			section.Payload = ancestor.payload
		case 1:
			section.Representation = "excerpt"
			text, truncated := excerpt(string(ancestor.payload), ExcerptTokens)
			section.Excerpt = text
			section.ExcerptTruncated = &truncated
		default:
			section.Representation = "reference"
		}
		sections = append(sections, section)
	}
	return sections
}
