package run

import (
	"fmt"
	"runtime"
)

var availableNProcForRun = availableNProc

var runtimeGOMAXPROCS = runtime.GOMAXPROCS
var runtimeNumCPU = runtime.NumCPU

func availableNProc() int {
	if n := runtimeGOMAXPROCS(0); n > 0 {
		return n
	}
	if n := runtimeNumCPU(); n > 0 {
		return n
	}
	return 1
}

func resolveNProc(raw int, defaultLimit int) (int, error) {
	if raw < 0 {
		return 0, fmt.Errorf("nproc must be >= 0")
	}
	if defaultLimit < 1 {
		defaultLimit = 1
	}
	if raw == 0 {
		return defaultLimit, nil
	}
	return raw, nil
}
