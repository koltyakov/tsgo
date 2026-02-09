package codeanalyzer

import "testing"

func TestAnalyze_IgnoresStringsAndComments(t *testing.T) {
	code := `
		// await fetch("/ignored")
		const text = "readFile('ignored')";
		/* WebSocket setTimeout() */
		const x = 1;
	`

	result := Analyze(code)
	if result.HasAwait || result.HasAsync {
		t.Fatalf("expected async flags to be false, got async=%v await=%v", result.HasAsync, result.HasAwait)
	}
	if result.HasCall("fetch") || result.HasCall("readFile") || result.HasIdentifier("WebSocket") {
		t.Fatalf("expected no feature calls/identifiers from comments/strings")
	}
}

func TestAnalyze_DetectsTemplateExpression(t *testing.T) {
	code := "const value = `${await fetch('/api')}`;"
	result := Analyze(code)

	if !result.HasAwait {
		t.Fatal("expected await to be detected in template expression")
	}
	if !result.HasCall("fetch") {
		t.Fatal("expected fetch call to be detected in template expression")
	}
}

func TestAnalyze_Complexity(t *testing.T) {
	code := `for (let i = 0; i < 1; i++) { const f = () => i; if (i) {} }`
	result := Analyze(code)

	if result.Complexity < 3 {
		t.Fatalf("expected complexity >= 3, got %d", result.Complexity)
	}
}
