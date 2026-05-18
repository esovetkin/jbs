package patternutil

import (
	"regexp"
	"strings"
	"testing"
)

func TestNormalizePercentPattern(t *testing.T) {
	got, ok := NormalizePercentPattern("id=%d ratio=%f word=%w literal=%%")
	if !ok {
		t.Fatal("expected valid pattern")
	}
	re := regexp.MustCompile(got.Regex)
	if !re.MatchString("id=-7 ratio=1.25 word=abc_1 literal=%") {
		t.Fatalf("normalized regex did not match: %q", got.Regex)
	}
	names := re.SubexpNames()
	kinds := make([]string, 0, re.NumSubexp())
	for i := 1; i < len(names); i++ {
		kinds = append(kinds, string(got.CaptureTypesByName[names[i]]))
	}
	if strings.Join(kinds, ",") != "int,float,string" {
		t.Fatalf("capture kinds = %#v", kinds)
	}
}

func TestNormalizePercentPatternPreservesManualGroups(t *testing.T) {
	got, ok := NormalizePercentPattern(`pair=([A-Z]+)-([0-9]+)`)
	if !ok {
		t.Fatal("expected valid pattern")
	}
	if got.Regex != `pair=([A-Z]+)-([0-9]+)` || len(got.CaptureTypesByName) != 0 {
		t.Fatalf("normalized = %#v", got)
	}
}

func TestNormalizePercentPatternLiteralPercent(t *testing.T) {
	got, ok := NormalizePercentPattern("rate=%f%%")
	if !ok {
		t.Fatal("expected valid pattern")
	}
	re := regexp.MustCompile(got.Regex)
	if !re.MatchString("rate=12.5%") {
		t.Fatalf("normalized regex did not match literal percent: %q", got.Regex)
	}
}

func TestNormalizePercentPatternRejectsInvalidPercent(t *testing.T) {
	for _, input := range []string{"value %", "value=%x"} {
		if _, ok := NormalizePercentPattern(input); ok {
			t.Fatalf("expected %q to be invalid", input)
		}
	}
}

func TestNormalizePercentPatternAvoidsInputCaptureNameCollision(t *testing.T) {
	got, ok := NormalizePercentPattern("JBS_CAPTURE_INT_0=%d")
	if !ok {
		t.Fatal("expected valid pattern")
	}
	if strings.Contains(got.Regex, `?P<JBS_CAPTURE_INT_0>`) {
		t.Fatalf("normalized regex reused input capture name: %q", got.Regex)
	}
}
