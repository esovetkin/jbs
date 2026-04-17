package sema

import "jbs/internal/diag"

func countDiagCode(diags *diag.Diagnostics, code string) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}
