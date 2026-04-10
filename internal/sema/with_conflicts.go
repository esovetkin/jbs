package sema

import "jbs/internal/diag"

type importOrigin struct {
	Source string
	Span   diag.Span
}

type importConflictTracker struct {
	seen     map[string]importOrigin
	reported map[string]struct{}
}

func newImportConflictTracker() *importConflictTracker {
	return &importConflictTracker{
		seen:     make(map[string]importOrigin),
		reported: make(map[string]struct{}),
	}
}

func (t *importConflictTracker) Add(name, source string, span diag.Span) (importOrigin, bool, bool) {
	if prev, ok := t.seen[name]; ok {
		if prev.Source == source {
			return prev, false, false
		}
		left := prev.Source
		right := source
		if left > right {
			left, right = right, left
		}
		key := name + "|" + left + "|" + right
		if _, exists := t.reported[key]; exists {
			return prev, true, false
		}
		t.reported[key] = struct{}{}
		return prev, true, true
	}
	t.seen[name] = importOrigin{
		Source: source,
		Span:   span,
	}
	return importOrigin{}, false, false
}
