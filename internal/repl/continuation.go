package repl

import parserpkg "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"

type ContinuationState = parserpkg.StructuralScanState

func ScanContinuationState(src string) ContinuationState {
	return parserpkg.ScanStructuralState(src)
}
