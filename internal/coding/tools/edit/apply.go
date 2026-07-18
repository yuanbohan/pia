package edit

import (
	"fmt"
	"sort"
	"strings"
)

type replacement struct {
	oldText string
	newText string
}

type matchedReplacement struct {
	editIndex int
	start     int
	end       int
	newText   string
}

func applyExactReplacements(content string, replacements []replacement, path string) (string, error) {
	// Frozen Pi falls back to fuzzy normalization for whitespace and several
	// Unicode forms. Phase 1 deliberately stays exact: a failed match leaves the
	// file unchanged, while a guessed match could commit an unintended edit.
	// Fuzzy matching therefore needs separate design and test work before it can
	// broaden this mutation contract.
	matches := make([]matchedReplacement, 0, len(replacements))
	for index, replacement := range replacements {
		first, multiple := findFirstAndDuplicate(content, replacement.oldText)
		switch {
		case first < 0:
			if len(replacements) == 1 {
				return "", fmt.Errorf("edit %q: could not find the exact oldText", path)
			}
			return "", fmt.Errorf("edit %q: could not find edits[%d].oldText", path, index)
		case multiple:
			if len(replacements) == 1 {
				return "", fmt.Errorf("edit %q: found multiple occurrences of oldText; provide more context so it is unique", path)
			}
			return "", fmt.Errorf("edit %q: found multiple occurrences of edits[%d].oldText; provide more context so it is unique", path, index)
		}
		matches = append(matches, matchedReplacement{
			editIndex: index,
			start:     first,
			end:       first + len(replacement.oldText),
			newText:   replacement.newText,
		})
	}

	// Every match above is resolved against the same original content. Sorting
	// those immutable ranges lets us reject overlap before constructing any new
	// content, so a failing multi-edit cannot partially affect the target.
	sort.Slice(matches, func(left, right int) bool {
		return matches[left].start < matches[right].start
	})
	for index := 1; index < len(matches); index++ {
		previous := matches[index-1]
		current := matches[index]
		if previous.end > current.start {
			return "", fmt.Errorf(
				"edit %q: edits[%d] and edits[%d] overlap; merge them or target disjoint regions",
				path,
				previous.editIndex,
				current.editIndex,
			)
		}
	}

	var result strings.Builder
	result.Grow(len(content))
	cursor := 0
	for _, match := range matches {
		result.WriteString(content[cursor:match.start])
		result.WriteString(match.newText)
		cursor = match.end
	}
	result.WriteString(content[cursor:])
	updated := result.String()
	if updated == content {
		return "", fmt.Errorf("edit %q: no changes; replacements produced identical content", path)
	}
	return updated, nil
}

func findFirstAndDuplicate(content, target string) (first int, multiple bool) {
	first = strings.Index(content, target)
	if first < 0 {
		return -1, false
	}
	// Start one byte after the first position so overlapping occurrences still
	// make oldText non-unique. Only a second match matters; scanning and counting
	// every match would add unbounded work without changing the decision.
	return first, strings.Contains(content[first+1:], target)
}
