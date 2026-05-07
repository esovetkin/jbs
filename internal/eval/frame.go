package eval

import (
	"fmt"
	"sort"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type Frame struct {
	Parent  *Frame
	Values  map[string]*Cell
	Resolve func(name string, at diag.Span, diags *diag.Diagnostics) (Value, bool)
}

func NewRootFrame(env map[string]Value) *Frame {
	frame := &Frame{Values: make(map[string]*Cell, len(env))}
	for name, value := range env {
		if name == "" {
			continue
		}
		frame.Values[name] = &Cell{Value: value, Assigned: true}
	}
	return frame
}

func NewChildFrame(parent *Frame) *Frame {
	return &Frame{
		Parent: parent,
		Values: make(map[string]*Cell),
	}
}

func (f *Frame) LookupCell(name string) (*Cell, bool) {
	for cur := f; cur != nil; cur = cur.Parent {
		if cur.Values == nil {
			continue
		}
		cell, ok := cur.Values[name]
		if ok {
			return cell, true
		}
	}
	return nil, false
}

func (f *Frame) ResolveValue(name string, at diag.Span, diags *diag.Diagnostics) (Value, bool, bool) {
	if cell, ok := f.LookupCell(name); ok {
		return cell.Value, true, cell.Assigned
	}
	for cur := f; cur != nil; cur = cur.Parent {
		if cur.Resolve == nil {
			continue
		}
		if value, ok := cur.Resolve(name, at, diags); ok {
			return value, true, true
		}
	}
	return Null(), false, false
}

func (f *Frame) HasLocal(name string) bool {
	if f == nil || f.Values == nil {
		return false
	}
	_, ok := f.Values[name]
	return ok
}

func (f *Frame) DeclareLocal(name string) {
	if f == nil || name == "" {
		return
	}
	if f.Values == nil {
		f.Values = make(map[string]*Cell)
	}
	if _, ok := f.Values[name]; ok {
		return
	}
	f.Values[name] = &Cell{}
}

func (f *Frame) AssignLocal(name string, value Value, origin diag.Span) {
	if f == nil || name == "" {
		return
	}
	if f.Values == nil {
		f.Values = make(map[string]*Cell)
	}
	cell, ok := f.Values[name]
	if !ok {
		cell = &Cell{}
		f.Values[name] = cell
	}
	cell.Value = value
	cell.Origin = origin
	cell.Assigned = true
}

func (f *Frame) Read(name string, at diag.Span, diags *diag.Diagnostics) (Value, bool) {
	if value, ok, assigned := f.ResolveValue(name, at, diags); ok {
		if assigned {
			return value, true
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", name), at, "assign the local before reading it")
		return Null(), false
	}
	diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", name), at, "import or define the variable before use")
	return Null(), false
}

func (f *Frame) VisibleNames() []string {
	if f == nil {
		return nil
	}
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for cur := f; cur != nil; cur = cur.Parent {
		for name := range cur.Values {
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
