// Monaco editor integration example
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/koltyakov/tsgo"
	"github.com/koltyakov/tsgo/internal/monaco"
	"github.com/koltyakov/tsgo/internal/typegen"
)

//go:embed index.html
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

	// Serve static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, _ := content.ReadFile("index.html")
			w.Header().Set("Content-Type", "text/html")
			w.Write(data)
			return
		}
		http.NotFound(w, r)
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

		// Select engine
		var opts []tsgo.Option
		opts = append(opts, tsgo.WithTimeout(5*time.Second))
		opts = append(opts, tsgo.WithGlobals(globals))

		switch req.Engine {
		case "bun":
			opts = append(opts, tsgo.WithEngine(tsgo.EngineBun))
		default:
			opts = append(opts, tsgo.WithEngine(tsgo.EngineGOJA))
		}

		exec := tsgo.New(opts...)
		defer exec.Close()

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

	// Mount Monaco handler
	http.Handle("/monaco/", http.StripPrefix("/monaco", handler))

	addr := "localhost:8080"
	fmt.Printf("Monaco editor available at http://%s\n", addr)
	fmt.Println("Custom types: User, Config, currentUser, config, log()")
	fmt.Println("\nPress Ctrl+C to stop")

	log.Fatal(http.ListenAndServe(addr, nil))
}
