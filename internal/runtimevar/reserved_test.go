package runtimevar

import "testing"

func TestReservedName(t *testing.T) {
	for _, name := range []string{"JBS_RUN_DIR", "JBS_SRC_DIR", "JBS_STEP", "JBS_ROW", "JBS_WORK_DIR"} {
		if reason, ok := ReservedName(name); !ok || reason == "" {
			t.Fatalf("ReservedName(%q) = %q, %v; want reserved reason", name, reason, ok)
		}
	}
	for _, name := range []string{"JBS_WORK_DIR_EXTRA", "MY_JBS_WORK_DIR", "jbs_work_dir", ""} {
		if reason, ok := ReservedName(name); ok || reason != "" {
			t.Fatalf("ReservedName(%q) = %q, %v; want unreserved", name, reason, ok)
		}
	}
}

func TestReservedNamesReturnsCopy(t *testing.T) {
	names := ReservedNames()
	names["JBS_WORK_DIR"] = "changed"
	reason, ok := ReservedName("JBS_WORK_DIR")
	if !ok || reason == "changed" {
		t.Fatalf("ReservedNames did not return a copy")
	}
}
