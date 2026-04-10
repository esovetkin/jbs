package sema_test

import (
	"strings"

	"jbs/internal/diag"
	"jbs/internal/sema"
)

func diagCount(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, d := range diags.Items {
		if d.Code == code {
			count++
		}
	}
	return count
}

func hasDiagCode(diags *diag.Diagnostics, code string) bool {
	return diagCount(diags, code) > 0
}

func hasW310For(diags *diag.Diagnostics, param, variable string) bool {
	target := "exposed variable '" + variable + "' from param '" + param + "'"
	for _, d := range diags.Items {
		if d.Code != "W310" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return true
		}
	}
	return false
}

func hasW310ForLet(diags *diag.Diagnostics, namespace, variable string) bool {
	target := "exposed variable '" + variable + "' from let '" + namespace + "'"
	for _, d := range diags.Items {
		if d.Code != "W310" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return true
		}
	}
	return false
}

func hasW312For(diags *diag.Diagnostics, variable string) bool {
	target := "param variable '" + variable + "'"
	for _, d := range diags.Items {
		if d.Code != "W312" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return true
		}
	}
	return false
}

func w310HintFor(diags *diag.Diagnostics, param, variable string) string {
	target := "exposed variable '" + variable + "' from param '" + param + "'"
	for _, d := range diags.Items {
		if d.Code != "W310" {
			continue
		}
		if strings.Contains(d.Message, target) {
			return d.Hint
		}
	}
	return ""
}

func submitValueByName(spec *sema.SubmitSpec, name string) (sema.SubmitValue, bool) {
	if spec == nil {
		return sema.SubmitValue{}, false
	}
	for _, value := range spec.Values {
		if value.Name == name {
			return value, true
		}
	}
	return sema.SubmitValue{}, false
}

func submitHelperByOriginal(spec *sema.SubmitSpec, name string) (sema.SubmitHelper, bool) {
	if spec == nil {
		return sema.SubmitHelper{}, false
	}
	for _, helper := range spec.Helpers {
		if helper.Original == name {
			return helper, true
		}
	}
	return sema.SubmitHelper{}, false
}
