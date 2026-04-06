package lower

import (
	"strings"

	"jbs/internal/eval"
	"jbs/internal/sema"
)

func globalString(globals sema.GlobalState, name, fallback string) string {
	v, ok := globals.Values[name]
	if !ok {
		return fallback
	}
	if v.Kind == eval.KindString {
		return v.S
	}
	s := v.String()
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
