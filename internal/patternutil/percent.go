package patternutil

import (
	"fmt"
	"strings"
)

type CaptureKind string

const (
	CaptureString CaptureKind = "string"
	CaptureInt    CaptureKind = "int"
	CaptureFloat  CaptureKind = "float"
)

const (
	runtimeIntBody   = `[-+]?[0-9]+`
	runtimeFloatBody = `[-+]?(?:(?:[0-9]+(?:\.[0-9]*)?)|(?:\.[0-9]+))(?:[eE][-+]?[0-9]+)?`
	runtimeWordBody  = `[A-Za-z0-9_]+`
)

type PercentPattern struct {
	Regex              string
	CaptureTypesByName map[string]CaptureKind
}

func NormalizePercentPattern(input string) (PercentPattern, bool) {
	var out strings.Builder
	captureTypes := make(map[string]CaptureKind)
	usedNames := make(map[string]struct{})
	counter := 0
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '%' {
			out.WriteRune(r)
			continue
		}
		if i+1 >= len(runes) {
			return PercentPattern{}, false
		}
		next := runes[i+1]
		switch next {
		case '%':
			out.WriteRune('%')
		case 'd':
			name := nextCaptureName(input, usedNames, "int", &counter)
			out.WriteString(namedCapture(name, runtimeIntBody))
			captureTypes[name] = CaptureInt
		case 'f':
			name := nextCaptureName(input, usedNames, "float", &counter)
			out.WriteString(namedCapture(name, runtimeFloatBody))
			captureTypes[name] = CaptureFloat
		case 'w':
			name := nextCaptureName(input, usedNames, "string", &counter)
			out.WriteString(namedCapture(name, runtimeWordBody))
			captureTypes[name] = CaptureString
		default:
			return PercentPattern{}, false
		}
		i++
	}
	return PercentPattern{Regex: out.String(), CaptureTypesByName: captureTypes}, true
}

func namedCapture(name, body string) string {
	return `(?P<` + name + `>` + body + `)`
}

func nextCaptureName(input string, used map[string]struct{}, kind string, counter *int) string {
	for {
		name := fmt.Sprintf("JBS_CAPTURE_%s_%d", strings.ToUpper(kind), *counter)
		*counter = *counter + 1
		if strings.Contains(input, name) {
			continue
		}
		if _, ok := used[name]; ok {
			continue
		}
		used[name] = struct{}{}
		return name
	}
}
