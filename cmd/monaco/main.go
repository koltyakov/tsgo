// Monaco editor integration example
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/koltyakov/tsgo"
)

//go:embed index.html styles.css app.js samples.json samples/*.ts
var content embed.FS

const codeSeparator = "// --- Code ---"

// Shared executors - reused across requests for performance
var (
	autoExec *tsgo.Executor
	gojaExec *tsgo.Executor
	bunExec  *tsgo.Executor
	execOnce sync.Once
	execMu   sync.RWMutex
)

func initExecutors() {
	execOnce.Do(func() {
		// Create executors without globals - context will be loaded per-sample
		autoExec = tsgo.New(
			tsgo.WithEngine(tsgo.EngineAuto),
			tsgo.WithTimeout(5*time.Second),
		)

		gojaExec = tsgo.New(
			tsgo.WithEngine(tsgo.EngineGOJA),
			tsgo.WithTimeout(5*time.Second),
		)

		bunExec = tsgo.New(
			tsgo.WithEngine(tsgo.EngineBun),
			tsgo.WithTimeout(5*time.Second),
		)

		// Prewarm engines in background
		go prewarmEngines()
	})
}

// prewarmEngines runs a simple script on each engine to warm up JIT and caches
func prewarmEngines() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	warmupCode := `export default 1 + 1`

	if gojaExec != nil {
		if _, err := gojaExec.Execute(ctx, warmupCode); err != nil {
			log.Printf("GOJA warmup failed: %v", err)
		} else {
			log.Println("GOJA engine warmed up")
		}
	}

	if bunExec != nil {
		if _, err := bunExec.Execute(ctx, warmupCode); err != nil {
			log.Printf("Bun warmup failed: %v", err)
		} else {
			log.Println("Bun engine warmed up")
		}
	}
}

func getExecutor(engine string) *tsgo.Executor {
	initExecutors()
	execMu.RLock()
	defer execMu.RUnlock()

	switch engine {
	case "bun":
		if bunExec != nil {
			return bunExec
		}
		return gojaExec
	case "goja":
		return gojaExec
	default:
		return autoExec
	}
}

// executeWithContext executes code with context prepended
// The context file exports are available as globals in the main script
func executeWithContext(ctx context.Context, exec *tsgo.Executor, contextCode, mainCode string) (*tsgo.Result, error) {
	// If there's context code, prepend it to make exports available
	var fullCode string
	if contextCode != "" {
		// Remove export keywords from context to make variables available in scope
		// and wrap the main code to use those variables
		fullCode = contextCode + "\n\n" + mainCode
	} else {
		fullCode = mainCode
	}

	return exec.Execute(ctx, fullCode)
}

func main() {
	// Initialize and prewarm engines early
	initExecutors()

	// Create Monaco handler (no default types - loaded per sample)
	handler := tsgo.NewMonacoHandler()
	builder := tsgo.NewTypeBuilder()
	handler.SetTypes(builder)

	// Create contract analyzer (no default types - types come from context)
	contractAnalyzer := tsgo.NewContractAnalyzer()

	// Serve static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		filename := path[1:]
		data, err := content.ReadFile(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		switch {
		case filename == "index.html":
			w.Header().Set("Content-Type", "text/html")
		case filename == "styles.css":
			w.Header().Set("Content-Type", "text/css")
		case filename == "app.js":
			w.Header().Set("Content-Type", "application/javascript")
		case strings.HasSuffix(filename, ".ts"):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		case strings.HasSuffix(filename, ".json"):
			w.Header().Set("Content-Type", "application/json")
		}

		_, _ = w.Write(data)
	})

	// Execute endpoint - now accepts context code
	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Code        string `json:"code"`
			ContextCode string `json:"contextCode"`
			Engine      string `json:"engine"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		exec := getExecutor(req.Engine)
		ctx := context.Background()

		result, err := executeWithContext(ctx, exec, req.ContextCode, req.Code)

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"value":    result.Value,
			"type":     fmt.Sprintf("%T", result.Value),
			"duration": result.Metrics.ExecutionTime.String(),
			"engine":   result.EngineUsed.String(),
		})
	})

	// Type inferrer using TypeScript Compiler API (more accurate)
	typeInferrer := tsgo.NewTypeInferrer()
	useTSInferrer := tsgo.IsBunAvailable()
	if useTSInferrer {
		log.Println("TypeScript type inferrer enabled (Bun available)")
	} else {
		log.Println("TypeScript type inferrer disabled (Bun not found), using Go-based analyzer")
	}

	// Contract generation endpoint
	http.HandleFunc("/contract", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Code        string `json:"code"`
			ContextCode string `json:"contextCode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Analyze the full code (context + main) for contract generation
		fullCode := req.Code
		if req.ContextCode != "" {
			fullCode = req.ContextCode + "\n\n" + req.Code
		}

		w.Header().Set("Content-Type", "application/json")

		// Try TypeScript Compiler API first (more accurate)
		if useTSInferrer {
			ctx := context.Background()
			inferResult, err := typeInferrer.InferDefaultExport(ctx, fullCode)
			if err == nil && inferResult.Error == "" {
				// Convert inference result to Contract for proper formatting
				c := tsgo.InferenceResultToContract(inferResult)
				tsDef := c.ToTypeScript()
				jsonSchema, _ := c.ToJSONSchemaJSON()

				_ = json.NewEncoder(w).Encode(map[string]any{
					"typescript": tsDef,
					"jsonSchema": string(jsonSchema),
					"contract":   fmt.Sprintf(`{"type":"%s","kind":"%s"}`, inferResult.Type, inferResult.Kind),
					"inferrer":   "typescript",
				})
				return
			}
			// Fall through to Go-based analyzer on error
			log.Printf("TS inferrer error (falling back to Go): %v", err)
		}

		// Fallback: Use Go-based regex analyzer
		c, err := contractAnalyzer.Analyze(fullCode)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
			})
			return
		}

		tsDef := c.ToTypeScript()
		jsonSchema, _ := c.ToJSONSchemaJSON()
		contractJSON, _ := c.ToJSON()

		_ = json.NewEncoder(w).Encode(map[string]any{
			"typescript": tsDef,
			"jsonSchema": string(jsonSchema),
			"contract":   string(contractJSON),
			"inferrer":   "go",
		})
	})

	// Get sample with split context/code
	http.HandleFunc("/sample/", func(w http.ResponseWriter, r *http.Request) {
		sampleId := strings.TrimPrefix(r.URL.Path, "/sample/")
		if sampleId == "" {
			http.Error(w, "Sample ID required", http.StatusBadRequest)
			return
		}

		// Read the sample file
		filename := "samples/" + sampleId + ".ts"
		data, err := content.ReadFile(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Split on the separator
		fullContent := string(data)
		parts := strings.SplitN(fullContent, codeSeparator, 2)

		var contextCode, mainCode string
		if len(parts) == 2 {
			contextCode = strings.TrimSpace(parts[0])
			mainCode = strings.TrimSpace(parts[1])
		} else {
			// No separator - everything is main code
			mainCode = strings.TrimSpace(fullContent)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"context": contextCode,
			"code":    mainCode,
		})
	})

	// Mount Monaco handler
	http.Handle("/monaco/", http.StripPrefix("/monaco", handler))

	addr := "localhost:8080"
	fmt.Printf("Monaco editor available at http://%s\n", addr)
	fmt.Println("Select a sample to load context and code")
	fmt.Println("\nPress Ctrl+C to stop")

	log.Fatal(http.ListenAndServe(addr, nil))
}

// buildTypeScriptFromInference creates a TypeScript type definition from inference result
func buildTypeScriptFromInference(result *tsgo.InferenceResult) string {
	if result.Type == "" {
		return "// Contract definition\nexport type Result = any;\n"
	}

	switch result.Kind {
	case "primitive", "literal":
		return fmt.Sprintf("// Contract definition\nexport type Result = %s;\n", result.Type)
	case "object":
		if len(result.Properties) > 0 {
			var props []string
			for _, prop := range result.Properties {
				opt := ""
				if prop.Optional {
					opt = "?"
				}
				props = append(props, fmt.Sprintf("  %s%s: %s;", prop.Name, opt, prop.Type))
			}
			return fmt.Sprintf("// Contract definition\nexport type Result = {\n%s\n};\n", strings.Join(props, "\n"))
		}
		return fmt.Sprintf("// Contract definition\nexport type Result = %s;\n", result.Type)
	case "array":
		return fmt.Sprintf("// Contract definition\nexport type Result = %s;\n", result.Type)
	case "function":
		return fmt.Sprintf("// Contract definition\nexport type Result = %s;\n", result.Type)
	default:
		return fmt.Sprintf("// Contract definition\nexport type Result = %s;\n", result.Type)
	}
}

// buildJSONSchemaFromInference creates a JSON Schema from inference result
func buildJSONSchemaFromInference(result *tsgo.InferenceResult) string {
	schema := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
	}

	switch result.Kind {
	case "primitive":
		schema["type"] = tsToJSONSchemaType(result.Type)
	case "object":
		schema["type"] = "object"
		if len(result.Properties) > 0 {
			props := make(map[string]any)
			for _, prop := range result.Properties {
				props[prop.Name] = map[string]any{
					"type": tsToJSONSchemaType(prop.Type),
				}
			}
			schema["properties"] = props
		}
	case "array":
		schema["type"] = "array"
	case "function":
		// Functions don't map well to JSON Schema
		schema["type"] = "object"
		schema["description"] = result.Type
	default:
		schema["type"] = "object"
	}

	jsonBytes, _ := json.Marshal(schema)
	return string(jsonBytes)
}

// tsToJSONSchemaType converts TypeScript types to JSON Schema types
func tsToJSONSchemaType(tsType string) string {
	switch tsType {
	case "string":
		return "string"
	case "number", "bigint":
		return "number"
	case "boolean":
		return "boolean"
	case "null":
		return "null"
	case "undefined":
		return "null"
	default:
		if strings.HasSuffix(tsType, "[]") {
			return "array"
		}
		return "object"
	}
}
