package sema

import (
	"sort"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func scopeNameCatalog(visible []string, namespaces map[string]*Namespace) *eval.NameCatalog {
	return eval.NewNameCatalog(visible, namespaceCatalogMembers(namespaces))
}

func namespaceCatalogMembers(namespaces map[string]*Namespace) map[string][]string {
	out := make(map[string][]string, len(namespaces))
	for name, ns := range namespaces {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out[name] = directNamespaceMembers(name, ns)
	}
	return out
}

func directNamespaceMembers(nsName string, ns *Namespace) []string {
	if ns == nil || strings.TrimSpace(nsName) == "" {
		return []string{}
	}
	prefix := nsName + "."
	names := ns.Members
	if len(names) == 0 {
		names = ns.Bindings
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, memberName := range names {
		rest := strings.TrimPrefix(memberName, prefix)
		if rest == memberName || strings.Contains(rest, ".") {
			continue
		}
		if _, ok := seen[rest]; ok {
			continue
		}
		seen[rest] = struct{}{}
		out = append(out, rest)
	}
	sort.Strings(out)
	return out
}

func cloneVisibleNamespaces(in map[string]*Namespace) map[string]*Namespace {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*Namespace, len(in))
	for name, ns := range in {
		if ns == nil {
			continue
		}
		out[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	return out
}

func mergeVisibleNamespaces(dst map[string]*Namespace, src map[string]*Namespace) map[string]*Namespace {
	if dst == nil {
		dst = make(map[string]*Namespace)
	}
	for name, ns := range src {
		if ns == nil {
			continue
		}
		current := dst[name]
		if current == nil {
			dst[name] = &Namespace{
				Name:     ns.Name,
				Members:  append([]string(nil), ns.Members...),
				Bindings: append([]string(nil), ns.Bindings...),
				Steps:    append([]string(nil), ns.Steps...),
			}
			continue
		}
		current.Members = mergeUniqueStrings(current.Members, ns.Members)
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
	return dst
}

func visibleNamesFromEnv(env map[string]eval.Value) []string {
	out := make([]string, 0, len(env))
	for name := range env {
		if isUnqualifiedVisibleName(name) {
			out = append(out, name)
		}
	}
	return out
}

func isUnqualifiedVisibleName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.Contains(name, ".")
}
