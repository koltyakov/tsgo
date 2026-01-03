package sandbox

import (
	"testing"
)

func TestValidateCode_Clean(t *testing.T) {
	code := "const x = 1 + 2;"
	restricted := []string{"eval", "Function"}

	err := ValidateCode(code, restricted)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCode_Restricted(t *testing.T) {
	code := `const result = eval("1 + 1");`
	restricted := []string{"eval", "Function"}

	err := ValidateCode(code, restricted)
	if err == nil {
		t.Error("expected error for restricted global")
	}
}

func TestValidatePath_Allowed(t *testing.T) {
	err := ValidatePath("/tmp/test.txt", []string{"/tmp"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidatePath_NotAllowed(t *testing.T) {
	err := ValidatePath("/etc/passwd", []string{"/tmp"})
	if err == nil {
		t.Error("expected error for disallowed path")
	}
}

func TestValidatePath_NoPaths(t *testing.T) {
	err := ValidatePath("/tmp/test.txt", []string{})
	if err == nil {
		t.Error("expected error when no paths allowed")
	}
}

func TestRestrictedGlobals(t *testing.T) {
	globals := RestrictedGlobals()
	if len(globals) == 0 {
		t.Error("expected restricted globals list")
	}

	// Check for common dangerous globals
	found := make(map[string]bool)
	for _, g := range globals {
		found[g] = true
	}

	required := []string{"eval", "Function", "fetch"}
	for _, r := range required {
		if !found[r] {
			t.Errorf("expected %s in restricted globals", r)
		}
	}
}
