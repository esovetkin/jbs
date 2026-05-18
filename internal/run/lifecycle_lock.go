package run

import (
	"path/filepath"
	"slices"
)

type rootLockRequest struct {
	Root     string
	Existing bool
}

type heldRootLocks struct {
	unlocks []func()
}

func (h *heldRootLocks) release() {
	if h == nil {
		return
	}
	for i := len(h.unlocks) - 1; i >= 0; i-- {
		h.unlocks[i]()
	}
	h.unlocks = nil
}

func acquireRootLockRequests(requests []rootLockRequest) (*heldRootLocks, error) {
	held := &heldRootLocks{}
	for _, req := range requests {
		var (
			unlock func()
			err    error
		)
		if req.Existing {
			unlock, err = acquireExistingRootLock(req.Root)
		} else {
			unlock, err = acquireRootLock(req.Root)
		}
		if err != nil {
			held.release()
			return nil, err
		}
		held.unlocks = append(held.unlocks, unlock)
	}
	return held, nil
}

func suiteLifecycleLockRequests(suite runtimeSuitePlan, existing bool) []rootLockRequest {
	top := filepath.Clean(suite.RootName)
	if top == "." || top == "" {
		top = inferSuiteRoot(suite)
	}

	seen := map[string]struct{}{}
	requests := make([]rootLockRequest, 0, 1+len(suite.Plans))
	add := func(root string) {
		root = filepath.Clean(root)
		if root == "." || root == "" {
			return
		}
		if _, ok := seen[root]; ok {
			return
		}
		seen[root] = struct{}{}
		requests = append(requests, rootLockRequest{Root: root, Existing: existing})
	}

	add(top)
	componentRoots := make([]string, 0, len(suite.Plans))
	for _, plan := range suite.Plans {
		componentRoots = append(componentRoots, plan.RootDir)
	}
	slices.Sort(componentRoots)
	for _, root := range componentRoots {
		add(root)
	}
	return requests
}

func inferSuiteRoot(suite runtimeSuitePlan) string {
	if len(suite.Plans) == 1 {
		return suite.Plans[0].RootDir
	}
	return ""
}
