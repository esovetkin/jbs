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

func TestRenderBenchmarkColumn(t *testing.T) {
	table := Table{
		BenchmarkColumn: true,
		Columns:         []string{"cases.x"},
		Rows: []Row{
			{Benchmark: "small", StepKind: "do", StepName: "run_small", Values: map[string]string{"cases.x": "1"}},
			{Benchmark: "large", StepKind: "do", StepName: "run_large", Values: map[string]string{"cases.x": "2"}},
		},
	}
	pretty, err := Render(table, RenderPretty)
	if err != nil {
		t.Fatalf("unexpected pretty render error: %v", err)
	}
	wantPretty := "| benchmark | cases.x | step          |\n" +
		"|-----------|---------|---------------|\n" +
		"| small     | 1       | do: run_small |\n" +
		"| large     | 2       | do: run_large |\n"
	if pretty != wantPretty {
		t.Fatalf("unexpected pretty render\n--- got ---\n%s\n--- expected ---\n%s", pretty, wantPretty)
	}

	csv, err := Render(table, RenderCSV)
	if err != nil {
		t.Fatalf("unexpected csv render error: %v", err)
	}
	wantCSV := "benchmark,cases.x,step\nsmall,1,do: run_small\nlarge,2,do: run_large\n"
	if csv != wantCSV {
		t.Fatalf("unexpected csv render\n--- got ---\n%s\n--- expected ---\n%s", csv, wantCSV)
	}
}

func TestRenderUnknownType(t *testing.T) {
	_, err := Render(Table{}, RenderType("json"))
	if err == nil {
		t.Fatalf("expected unknown type error")
	}
}
