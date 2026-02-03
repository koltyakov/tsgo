package main

import (
	"fmt"
	"github.com/koltyakov/tsgo/internal/contract"
)

func main() {
	analyzer := contract.NewAnalyzer()

	tests := []struct {
		name string
		code string
	}{
		{"null", `export default null`},
		{"undefined", `export default undefined`},
		{"number literal type", `const x = 42 as const; export default x`},
		{"nullable", `const x: string | null = null; export default x`},
		{"arrow function", `const fn = (x: number): number => x * 2; export default fn`},
		{"enum value", `enum Color { Red, Green, Blue } export default Color.Green`},
		{"class instance", `class Point { constructor(public x: number, public y: number) {} } const p = new Point(1, 2); export default p`},
	}

	for _, t := range tests {
		c, err := analyzer.Analyze(t.code)
		if err != nil {
			fmt.Printf("%-20s Error: %v\n", t.name, err)
		} else if c.Type != nil {
			fmt.Printf("%-20s Type: %-20s Kind: %s\n", t.name, c.Type.Name, c.Type.Kind)
		} else {
			fmt.Printf("%-20s Type is nil\n", t.name)
		}
	}
}
