package diag

import (
	"fmt"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type RelatedSpan struct {
	Message string
	Span    Span
}

type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	Span     Span
	Hint     string
	Related  []RelatedSpan
}

type Diagnostics struct {
	Items []Diagnostic
}

func (d *Diagnostics) Add(di Diagnostic) {
	d.Items = append(d.Items, di)
}

func (d *Diagnostics) AddError(code, message string, span Span, hint string, related ...RelatedSpan) {
	d.Add(Diagnostic{Severity: SeverityError, Code: code, Message: message, Span: span, Hint: hint, Related: related})
}

func (d *Diagnostics) AddWarning(code, message string, span Span, hint string, related ...RelatedSpan) {
	d.Add(Diagnostic{Severity: SeverityWarning, Code: code, Message: message, Span: span, Hint: hint, Related: related})
}

func (d Diagnostics) HasErrors() bool {
	for _, item := range d.Items {
		if item.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (d Diagnostics) String() string {
	if len(d.Items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(d.Items)*3)
	for _, item := range d.Items {
		head := strings.TrimSpace(fmt.Sprintf("%s %s %s", strings.ToUpper(string(item.Severity)), item.Code, item.Span.String()))
		lines = append(lines, head)
		lines = append(lines, item.Message)
		if item.Hint != "" {
			lines = append(lines, "Hint: "+item.Hint)
		}
		for _, rel := range item.Related {
			lines = append(lines, fmt.Sprintf("Related: %s (%s)", rel.Message, rel.Span.String()))
		}
	}
	return strings.Join(lines, "\n")
}

func (d Diagnostics) Error() string {
	return d.String()
}
