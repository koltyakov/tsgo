// Monaco editor integration example
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/koltyakov/tsgo"
	"github.com/koltyakov/tsgo/internal/contract"
	"github.com/koltyakov/tsgo/internal/monaco"
	"github.com/koltyakov/tsgo/internal/typegen"
)

//go:embed index.html styles.css app.js
var content embed.FS

// Globals available to scripts
var globals = map[string]any{
	"currentUser": map[string]any{
		"id":    1,
		"name":  "John Doe",
		"email": "john@example.com",
		"role":  "admin",
	},
	"config": map[string]any{
		"apiUrl":  "https://api.example.com",
		"timeout": 5000,
		"debug":   true,
	},
}

// Shared executors - reused across requests for performance
var (
	gojaExec *tsgo.Executor
	bunExec  *tsgo.Executor
	execOnce sync.Once
	execMu   sync.RWMutex
)

func initExecutors() {
	execOnce.Do(func() {
		// Create GOJA executor (always available)
		gojaExec = tsgo.New(
			tsgo.WithEngine(tsgo.EngineGOJA),
			tsgo.WithTimeout(5*time.Second),
			tsgo.WithGlobals(globals),
		)

		// Create Bun executor (may not be available)
		bunExec = tsgo.New(
			tsgo.WithEngine(tsgo.EngineBun),
			tsgo.WithTimeout(5*time.Second),
			tsgo.WithGlobals(globals),
		)
	})
}

func getExecutor(engine string) *tsgo.Executor {
	initExecutors()
	execMu.RLock()
	defer execMu.RUnlock()

	if engine == "bun" && bunExec != nil {
		return bunExec
	}
	return gojaExec
}

func main() {
	// Create Monaco handler
	handler := monaco.NewHandler()

	// Set up custom type definitions
	builder := typegen.NewBuilder()
	builder.AddInterface("User", map[string]string{
		"id":    "number",
		"name":  "string",
		"email": "string",
		"role":  "'admin' | 'user' | 'guest'",
	})
	builder.AddInterface("Config", map[string]string{
		"apiUrl":  "string",
		"timeout": "number",
		"debug":   "boolean",
	})
	builder.AddGlobal("currentUser", "User")
	builder.AddGlobal("config", "Config")

	handler.SetTypes(builder)

	// Create contract analyzer with the same type definitions
	contractAnalyzer := contract.NewAnalyzer()
	contractAnalyzer.AddInterface("User", map[string]string{
		"id":    "number",
		"name":  "string",
		"email": "string",
		"role":  "'admin' | 'user' | 'guest'",
	})
	contractAnalyzer.AddInterface("Config", map[string]string{
		"apiUrl":  "string",
		"timeout": "number",
		"debug":   "boolean",
	})
	contractAnalyzer.AddGlobalFromTypeString("currentUser", "User")
	contractAnalyzer.AddGlobalFromTypeString("config", "Config")

	// Serve static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Remove leading slash for embed.FS
		filename := path[1:]
		data, err := content.ReadFile(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set content type based on extension
		switch {
		case filename == "index.html":
			w.Header().Set("Content-Type", "text/html")
		case filename == "styles.css":
			w.Header().Set("Content-Type", "text/css")
		case filename == "app.js":
			w.Header().Set("Content-Type", "application/javascript")
		}

		w.Write(data)
	})

	// Execute endpoint
	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Code   string `json:"code"`
			Engine string `json:"engine"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get shared executor for the requested engine
		exec := getExecutor(req.Engine)

		ctx := context.Background()
		result, err := exec.Execute(ctx, req.Code)

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"value":    result.Value,
			"type":     fmt.Sprintf("%T", result.Value),
			"duration": result.Metrics.ExecutionTime.String(),
			"engine":   result.EngineUsed.String(),
		})
	})

	// Contract generation endpoint
	http.HandleFunc("/contract", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		c, err := contractAnalyzer.Analyze(req.Code)

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
			})
			return
		}

		// Generate TypeScript definition
		tsDef := c.ToTypeScript()

		// Generate JSON Schema
		jsonSchema, _ := c.ToJSONSchemaJSON()

		// Get contract as JSON
		contractJSON, _ := c.ToJSON()

		json.NewEncoder(w).Encode(map[string]any{
			"typescript": tsDef,
			"jsonSchema": string(jsonSchema),
			"contract":   string(contractJSON),
		})
	})

	// Mount Monaco handler
	http.Handle("/monaco/", http.StripPrefix("/monaco", handler))

	addr := "localhost:8080"
	fmt.Printf("Monaco editor available at http://%s\n", addr)
	fmt.Println("Custom types: User, Config, currentUser, config")
	fmt.Println("\nPress Ctrl+C to stop")

	log.Fatal(http.ListenAndServe(addr, nil))
}
