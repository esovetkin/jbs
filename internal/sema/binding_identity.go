package sema

import "fmt"

type BindingVersionKey struct {
	Public  string
	Version string
}

func (k BindingVersionKey) Display() string {
	if k.Public != "" {
		return k.Public
	}
	return k.Version
}

func BindingVersionKeyForSource(bindings map[string]*GlobalBinding, source string) BindingVersionKey {
	if binding := bindings[source]; binding != nil {
		return BindingVersionKeyForBinding(binding, source)
	}
	return BindingVersionKey{Public: source, Version: source}
}

func BindingVersionKeyForBinding(binding *GlobalBinding, fallback string) BindingVersionKey {
	if binding == nil {
		return BindingVersionKey{Public: fallback, Version: fallback}
	}
	public := binding.PublicName
	if public == "" {
		public = binding.Name
	}
	if public == "" {
		public = fallback
	}
	version := binding.VersionID
	if version == "" && !binding.Span.IsZero() {
		version = fmt.Sprintf("%s:%d:%d", binding.Span.File, binding.Span.Start.Offset, binding.Span.End.Offset)
	}
	if version == "" {
		version = binding.Name
	}
	if version == "" {
		version = fallback
	}
	return BindingVersionKey{Public: public, Version: version}
}
