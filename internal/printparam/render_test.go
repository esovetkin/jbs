package printparam

import "testing"

func TestRenderPrettyAlignment(t *testing.T) {
	table := Table{
		Columns: []string{"p.a", "p.b"},
		Rows: []Row{
			{StepKind: "do", StepName: "s0", Values: map[string]string{"p.a": "1"}},
			{StepKind: "do", StepName: "s1", Values: map[string]string{"p.a": "2", "p.b": "xyz"}},
		},
	}
	out, err := Render(table, RenderPretty)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	expected := "| p.a | p.b | step   |\n" +
		"|-----|-----|--------|\n" +
		"| 1   |     | do: s0 |\n" +
		"| 2   | xyz | do: s1 |\n"
	if out != expected {
		t.Fatalf("unexpected pretty render\n--- got ---\n%s\n--- expected ---\n%s", out, expected)
	}
}

func TestRenderCSVEscaping(t *testing.T) {
	table := Table{
		Columns: []string{"p.a", "p.b"},
		Rows: []Row{
			{StepKind: "do", StepName: "s", Values: map[string]string{"p.a": "1", "p.b": "x,y\"z"}},
		},
	}
	out, err := Render(table, RenderCSV)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	expected := "p.a,p.b,step\n1,\"x,y\"\"z\",do: s\n"
	if out != expected {
		t.Fatalf("unexpected csv render\n--- got ---\n%s\n--- expected ---\n%s", out, expected)
	}
}

func TestRenderUnknownType(t *testing.T) {
	_, err := Render(Table{}, RenderType("json"))
	if err == nil {
		t.Fatalf("expected unknown type error")
	}
}
