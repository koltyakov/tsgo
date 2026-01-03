# tsgo

A secure TypeScript execution library for Go with multiple execution engines.

## Features

- **Multiple Execution Engines**: GOJA (pure Go), Bun (requires installation)
- **TypeScript Support**: Full TypeScript transpilation via esbuild
- **Automatic Engine Selection**: Chooses the best engine based on code analysis
- **Security Sandboxing**: Restrict globals, file access, and network operations
- **Monaco Integration**: Live TypeScript types for Monaco editor
- **Source Map Support**: Error traces mapped to original TypeScript

## Installation

```bash
go get github.com/koltyakov/tsgo
```

## Quick Start

```go
package main

import (
  "context"
  "fmt"
  "time"

  "github.com/koltyakov/tsgo"
)

func main() {
  executor := tsgo.New(
    tsgo.WithEngine(tsgo.EngineGOJA),
    tsgo.WithTimeout(5*time.Second),
    tsgo.WithGlobals(map[string]any{
      "userId": 42,
    }),
  )
  defer executor.Close()

  result, err := executor.Execute(context.Background(), `
    const greeting: string = "Hello, User " + userId;
    export default greeting;
  `)
  if err != nil {
    panic(err)
  }
  
  fmt.Println(result.Value) // "Hello, User 42"
}
```

## Engines

| Engine | Pure Go | TypeScript | Async | Network |
|--------|---------|------------|-------|---------|
| GOJA   | ✅      | via esbuild| ✅    | ❌      |
| Bun    | ❌      | Native     | ✅    | ✅      |

## Configuration Options

```go
tsgo.New(
  tsgo.WithEngine(tsgo.EngineGOJA),     // Engine selection
  tsgo.WithTimeout(10*time.Second),      // Execution timeout
  tsgo.WithMemoryLimit(64*1024*1024),    // Memory limit (64MB)
  tsgo.WithGlobals(map[string]any{...}), // Global variables
  tsgo.WithSecurity(tsgo.SecurityPolicy{
    RestrictedGlobals: []string{"eval", "Function"},
  }),
  tsgo.WithSourceMaps(true),             // Enable source maps
  tsgo.WithPoolSize(4),                  // Worker pool size
)
```

## Monaco Integration

```go
handler := tsgo.NewMonacoHandler()

builder := tsgo.NewTypeBuilder()
builder.AddInterface("User", map[string]string{
  "id":   "number",
  "name": "string",
})
builder.AddGlobal("currentUser", "User")

handler.SetTypes(builder)

http.Handle("/", handler)
http.ListenAndServe(":8080", nil)
```

## Project Structure

```
github.com/koltyakov/tsgo
├── tsgo.go              # Public API
├── internal/
│   ├── types/           # Core types
│   ├── engine/
│   │   ├── goja/        # GOJA engine
│   │   └── bun/         # Bun engine
│   ├── transpiler/      # TypeScript transpiler
│   ├── selector/        # Engine selection
│   ├── sandbox/         # Security sandboxing
│   ├── sourcemap/       # Source map handling
│   ├── typegen/         # Type definition generation
│   └── monaco/          # Monaco editor integration
└── cmd/basic/           # Example application
```

## License

MIT