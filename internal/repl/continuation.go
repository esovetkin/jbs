package repl

import parserpkg "jbs/internal/parser"

type ContinuationState = parserpkg.StructuralScanState

func ScanContinuationState(src string) ContinuationState {
	return parserpkg.ScanStructuralState(src)
}
