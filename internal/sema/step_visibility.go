package sema

import (
	"jbs/internal/planutil"
)

func exposedVarNames(ps *Paramset) []string {
	return planutil.SourceVarNames(ps.Order, ps.Vars)
}
