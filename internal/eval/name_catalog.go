package eval

import "sort"

type NamespaceCatalog struct {
	Name    string
	Members []string
}

type NameCatalog struct {
	Visible    []string
	Namespaces map[string]NamespaceCatalog
}

func NewNameCatalog(visible []string, namespaces map[string][]string) *NameCatalog {
	out := &NameCatalog{
		Visible:    cloneSortedUniqueStrings(visible),
		Namespaces: make(map[string]NamespaceCatalog),
	}
	for name, members := range namespaces {
		if name == "" {
			continue
		}
		out.Namespaces[name] = NamespaceCatalog{
			Name:    name,
			Members: cloneSortedUniqueStrings(members),
		}
	}
	return out
}

func cloneSortedUniqueStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func uniqueStringsPreserveOrder(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
