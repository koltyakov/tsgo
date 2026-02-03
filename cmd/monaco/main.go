// Monaco editor integration for tsgo TypeScript playground.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/koltyakov/tsgo"
)

//go:embed index.html styles.css app.js samples.json samples/*.ts
var content embed.FS

// Configuration constants.
const (
	codeSeparator   = "// --- Code ---"
	defaultAddr     = "localhost:8080"
	executorTimeout = 5 * time.Second
	warmupTimeout   = 10 * time.Second
)

// MIME types for static file serving.
var mimeTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript; charset=utf-8",
	".ts":   "text/plain; charset=utf-8",
	".json": "application/json; charset=utf-8",
}

// executorPool manages shared executor instances.
type executorPool struct {
	auto *tsgo.Executor
	goja *tsgo.Executor
	bun  *tsgo.Executor
	once sync.Once
	mu   sync.RWMutex
}

var pool executorPool

func (p *executorPool) init() {
	p.once.Do(func() {
		p.auto = tsgo.New(
			tsgo.WithEngine(tsgo.EngineAuto),
			tsgo.WithTimeout(executorTimeout),
			tsgo.WithSecurity(tsgo.SecurityPolicy{
				NetworkAccess:  true,
				AllowedGlobals: []string{"fetch", "process", "eval"},
			}),
		)
		p.goja = tsgo.New(
			tsgo.WithEngine(tsgo.EngineGOJA),
			tsgo.WithTimeout(executorTimeout),
			tsgo.WithSecurity(tsgo.SecurityPolicy{
				NetworkAccess:  true,
				AllowedGlobals: []string{"fetch", "process", "eval"},
			}),
		)
		p.bun = tsgo.New(
			tsgo.WithEngine(tsgo.EngineBun),
			tsgo.WithTimeout(executorTimeout),
			tsgo.WithSecurity(tsgo.SecurityPolicy{
				NetworkAccess:  true,
				AllowedGlobals: []string{"fetch", "process", "eval"},
			}),
		)
		go p.warmup()
	})
}

func (p *executorPool) warmup() {
	ctx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
	defer cancel()

	const warmupCode = `export default 1 + 1`

	if p.goja != nil {
		if _, err := p.goja.Execute(ctx, warmupCode); err != nil {
			log.Printf("GOJA warmup failed: %v", err)
		} else {
			log.Println("GOJA engine warmed up")
		}
	}

	if p.bun != nil {
		if _, err := p.bun.Execute(ctx, warmupCode); err != nil {
			log.Printf("Bun warmup failed: %v", err)
		} else {
			log.Println("Bun engine warmed up")
		}
	}
}

func (p *executorPool) get(engine string) *tsgo.Executor {
	p.init()
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch engine {
	case "bun":
		if p.bun != nil {
			return p.bun
		}
		return p.goja
	case "goja":
		return p.goja
	default:
		return p.auto
	}
}

// server encapsulates all HTTP handler dependencies.
type server struct {
	monacoHandler    http.Handler
	contractAnalyzer *tsgo.ContractAnalyzer
	typeInferrer     *tsgo.TypeInferrer
	useTSInferrer    bool
}

func newServer() *server {
	handler := tsgo.NewMonacoHandler()
	handler.SetTypes(tsgo.NewTypeBuilder())

	s := &server{
		monacoHandler:    handler,
		contractAnalyzer: tsgo.NewContractAnalyzer(),
		typeInferrer:     tsgo.NewTypeInferrer(),
		useTSInferrer:    tsgo.IsBunAvailable(),
	}

	if s.useTSInferrer {
		log.Println("TypeScript type inferrer enabled (Bun available)")
	} else {
		log.Println("TypeScript type inferrer disabled (Bun not found), using Go-based analyzer")
	}

	return s
}

// serveStatic handles embedded static file serving.
func (s *server) serveStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	filename := strings.TrimPrefix(path, "/")

	data, err := content.ReadFile(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(filename)
	if mimeType, ok := mimeTypes[ext]; ok {
		w.Header().Set("Content-Type", mimeType)
	}

	_, _ = w.Write(data)
}

// executeRequest represents a code execution request.
type executeRequest struct {
	Code        string `json:"code"`
	ContextCode string `json:"contextCode"`
	Engine      string `json:"engine"`
}

// executeResponse represents a code execution response.
type executeResponse struct {
	Value    any    `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
	Duration string `json:"duration,omitempty"`
	Engine   string `json:"engine,omitempty"`
	Error    string `json:"error,omitempty"`
}

// handleExecute processes code execution requests.
func (s *server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	exec := pool.get(req.Engine)
	fullCode := combineCode(req.ContextCode, req.Code)

	result, err := exec.Execute(r.Context(), fullCode)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(executeResponse{Error: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(executeResponse{
		Value:    result.Value,
		Type:     fmt.Sprintf("%T", result.Value),
		Duration: result.Metrics.ExecutionTime.String(),
		Engine:   result.EngineUsed.String(),
	})
}

// contractRequest represents a contract generation request.
type contractRequest struct {
	Code        string `json:"code"`
	ContextCode string `json:"contextCode"`
}

// contractResponse represents a contract generation response.
type contractResponse struct {
	TypeScript string `json:"typescript,omitempty"`
	JSONSchema string `json:"jsonSchema,omitempty"`
	Contract   string `json:"contract,omitempty"`
	Inferrer   string `json:"inferrer,omitempty"`
	Error      string `json:"error,omitempty"`
}

// handleContract generates type contracts from code.
func (s *server) handleContract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req contractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	fullCode := combineCode(req.ContextCode, req.Code)
	w.Header().Set("Content-Type", "application/json")

	// Try TypeScript Compiler API first (more accurate)
	if s.useTSInferrer {
		if resp := s.inferWithTypeScript(r.Context(), fullCode); resp != nil {
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Fallback: Use Go-based regex analyzer
	s.analyzeWithGo(w, fullCode)
}

// inferWithTypeScript attempts type inference using TypeScript compiler.
func (s *server) inferWithTypeScript(ctx context.Context, code string) *contractResponse {
	result, err := s.typeInferrer.InferDefaultExport(ctx, code)
	if err != nil || result.Error != "" {
		if err != nil {
			log.Printf("TS inferrer error (falling back to Go): %v", err)
		}
		return nil
	}

	c := tsgo.InferenceResultToContract(result)
	jsonSchema, _ := c.ToJSONSchemaJSON()

	return &contractResponse{
		TypeScript: c.ToTypeScript(),
		JSONSchema: string(jsonSchema),
		Contract:   fmt.Sprintf(`{"type":%q,"kind":%q}`, result.Type, result.Kind),
		Inferrer:   "typescript",
	}
}

// analyzeWithGo performs contract analysis using Go-based analyzer.
func (s *server) analyzeWithGo(w http.ResponseWriter, code string) {
	c, err := s.contractAnalyzer.Analyze(code)
	if err != nil {
		_ = json.NewEncoder(w).Encode(contractResponse{Error: err.Error()})
		return
	}

	jsonSchema, _ := c.ToJSONSchemaJSON()
	contractJSON, _ := c.ToJSON()

	_ = json.NewEncoder(w).Encode(contractResponse{
		TypeScript: c.ToTypeScript(),
		JSONSchema: string(jsonSchema),
		Contract:   string(contractJSON),
		Inferrer:   "go",
	})
}

// sampleResponse represents a sample file response.
type sampleResponse struct {
	Context string `json:"context"`
	Code    string `json:"code"`
}

// handleSample serves sample files with context/code separation.
func (s *server) handleSample(w http.ResponseWriter, r *http.Request) {
	sampleID := strings.TrimPrefix(r.URL.Path, "/sample/")
	if sampleID == "" {
		http.Error(w, "Sample ID required", http.StatusBadRequest)
		return
	}

	filename := "samples/" + sampleID + ".ts"
	data, err := content.ReadFile(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	contextCode, mainCode := splitSample(string(data))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sampleResponse{
		Context: contextCode,
		Code:    mainCode,
	})
}

// splitSample separates context and main code from a sample file.
func splitSample(content string) (context, main string) {
	parts := strings.SplitN(content, codeSeparator, 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(content)
}

// combineCode merges context and main code.
func combineCode(context, main string) string {
	if context == "" {
		return main
	}
	return context + "\n\n" + main
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func main() {
	pool.init()

	srv := newServer()

	http.HandleFunc("/", srv.serveStatic)
	http.HandleFunc("/execute", srv.handleExecute)
	http.HandleFunc("/contract", srv.handleContract)
	http.HandleFunc("/sample/", srv.handleSample)
	http.Handle("/monaco/", http.StripPrefix("/monaco", srv.monacoHandler))

	fmt.Printf("Monaco editor available at http://%s\n", defaultAddr)
	fmt.Println("Select a sample to load context and code")
	fmt.Println("\nPress Ctrl+C to stop")

	log.Fatal(http.ListenAndServe(defaultAddr, nil))
}
