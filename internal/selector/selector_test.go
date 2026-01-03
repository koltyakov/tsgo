package selector

import (
	"testing"

	"github.com/koltyakov/tsgo/internal/types"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("expected selector to be created")
	}
}

func TestSelect_SimpleCode(t *testing.T) {
	s := New()
	code := "const x = 1 + 2;"

	engine := s.Select(code)

	if engine != types.EngineGOJA {
		t.Errorf("expected GOJA for simple code, got %v", engine)
	}
}

func TestSelect_NetworkCode(t *testing.T) {
	s := New()
	code := `const resp = await fetch("https://example.com");`

	engine := s.Select(code)

	if engine != types.EngineBun {
		t.Errorf("expected Bun for network code, got %v", engine)
	}
}

func TestSelect_UntrustedCode(t *testing.T) {
	s := New()
	code := `eval("1 + 1");`

	engine := s.Select(code)

	if engine != types.EngineGOJA {
		t.Errorf("expected GOJA for untrusted code, got %v", engine)
	}
}

func TestComplexity(t *testing.T) {
	s := New()

	tests := []struct {
		code     string
		minScore int
	}{
		{"const x = 1;", 0},
		{"for (let i = 0; i < 10; i++) {}", 1},
		{"if (x) { } else { }", 1},
		{"const f = () => {};", 1},
	}

	for _, tt := range tests {
		score := s.Complexity(tt.code)
		if score < tt.minScore {
			t.Errorf("expected complexity >= %d for %q, got %d", tt.minScore, tt.code, score)
		}
	}
}
