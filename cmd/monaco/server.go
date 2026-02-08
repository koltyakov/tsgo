package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/koltyakov/tsgo"
)

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

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveStatic)
	mux.HandleFunc("/execute", s.handleExecute)
	mux.HandleFunc("/contract", s.handleContract)
	mux.HandleFunc("/sample/", s.handleSample)
	mux.Handle("/monaco/", http.StripPrefix("/monaco", s.monacoHandler))
	return mux
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req executeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	exec := pool.get(req.Engine)
	fullCode := combineCode(req.ContextCode, req.Code)

	result, err := exec.Execute(r.Context(), fullCode)

	if err != nil {
		writeJSON(w, http.StatusOK, executeResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, executeResponse{
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req contractRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	fullCode := combineCode(req.ContextCode, req.Code)

	// Try TypeScript Compiler API first (more accurate)
	if s.useTSInferrer {
		if resp := s.inferWithTypeScript(r.Context(), fullCode); resp != nil {
			writeJSON(w, http.StatusOK, resp)
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
		writeJSON(w, http.StatusOK, contractResponse{Error: err.Error()})
		return
	}

	jsonSchema, _ := c.ToJSONSchemaJSON()
	contractJSON, _ := c.ToJSON()

	writeJSON(w, http.StatusOK, contractResponse{
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

	writeJSON(w, http.StatusOK, sampleResponse{
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
