// Package typeinfer provides TypeScript type inference using the TypeScript Compiler API.
//
// It uses a pool of persistent Bun worker processes for efficient inference.
// The TypeScript Compiler API provides accurate type information that would be
// difficult to achieve through static analysis alone.
package typeinfer

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Embedded Worker Script
// ============================================================================

//go:embed worker.ts
var workerScript string

// ============================================================================
// Types
// ============================================================================

// InferenceResult represents the result of type inference
type InferenceResult struct {
	Type        string     `json:"type"`
	Kind        string     `json:"kind"` // primitive, object, array, union, function, literal, any
	Properties  []Property `json:"properties,omitempty"`
	ElementType string     `json:"elementType,omitempty"`
	ReturnType  string     `json:"returnType,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// Property represents a property in an object type
type Property struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
}

// ============================================================================
// Inferrer
// ============================================================================

// Inferrer provides TypeScript type inference using the TS Compiler API.
// It maintains a pool of Bun worker processes for efficient inference.
type Inferrer struct {
	pool       *workerPool
	timeout    time.Duration
	available  bool
	workerPath string
	tempDir    string
	closed     atomic.Bool
	mu         sync.Mutex
}

// NewInferrer creates a new type inferrer with a worker pool
func NewInferrer() *Inferrer {
	return NewInferrerWithPoolSize(2)
}

// NewInferrerWithPoolSize creates a new type inferrer with specified pool size
func NewInferrerWithPoolSize(poolSize int) *Inferrer {
	if poolSize < 1 {
		poolSize = 1
	}
	if poolSize > 8 {
		poolSize = 8
	}

	i := &Inferrer{
		timeout:   5 * time.Second,
		available: false,
	}

	// Check if bun is available
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		return i
	}

	// Set up worker script
	workerPath, tempDir, err := setupWorkerScript()
	if err != nil {
		return i
	}

	i.workerPath = workerPath
	i.tempDir = tempDir
	i.available = true

	// Create worker pool
	i.pool = newWorkerPool(poolSize, bunPath, workerPath)

	return i
}

// setupWorkerScript writes the embedded worker script to a temp file
func setupWorkerScript() (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "typeinfer-*")
	if err != nil {
		return "", "", err
	}

	path := filepath.Join(tmpDir, "infer_worker.ts")
	if err := os.WriteFile(path, []byte(workerScript), 0600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", err
	}

	return path, tmpDir, nil
}

// WithTimeout sets the timeout for inference operations
func (i *Inferrer) WithTimeout(d time.Duration) *Inferrer {
	i.timeout = d
	return i
}

// InferDefaultExport infers the type of the default export in TypeScript code
func (i *Inferrer) InferDefaultExport(ctx context.Context, code string) (*InferenceResult, error) {
	if i.closed.Load() {
		return nil, errors.New("inferrer is closed")
	}

	if !i.available {
		return nil, errors.New("inferrer not available (bun not found)")
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()

	// Acquire a worker from pool
	worker, release, err := i.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire worker: %w", err)
	}
	defer release()

	// Send inference request
	result, err := worker.infer(ctx, code)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Close releases all resources
func (i *Inferrer) Close() error {
	if !i.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error

	if i.pool != nil {
		i.pool.close()
	}

	if i.tempDir != "" {
		if err := os.RemoveAll(i.tempDir); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// IsBunAvailable checks if Bun runtime is available
func IsBunAvailable() bool {
	_, err := exec.LookPath("bun")
	return err == nil
}

// ============================================================================
// Worker Pool
// ============================================================================

type workerPool struct {
	workers    []*worker
	bunPath    string
	workerPath string
	mu         sync.RWMutex
	closed     atomic.Bool
	nextIdx    uint64
}

type worker struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdinMu   sync.Mutex
	stdout    *bufio.Reader
	pending   map[string]chan *rpcResponse
	mu        sync.Mutex
	ready     bool
	failures  int32
	requestID int64
}

type rpcRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Code   string `json:"code,omitempty"`
}

type rpcResponse struct {
	ID     string           `json:"id"`
	Result *InferenceResult `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
		Stack   string `json:"stack,omitempty"`
	} `json:"error,omitempty"`
}

func newWorkerPool(size int, bunPath, workerPath string) *workerPool {
	p := &workerPool{
		workers:    make([]*worker, size),
		bunPath:    bunPath,
		workerPath: workerPath,
	}

	// Start initial workers
	for i := 0; i < size; i++ {
		w, err := p.startWorker()
		if err != nil {
			continue
		}
		p.workers[i] = w
	}

	return p
}

func (p *workerPool) startWorker() (*worker, error) {
	cmd := exec.Command(p.bunPath, "run", p.workerPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}

	w := &worker{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[string]chan *rpcResponse),
	}

	// Start response reader
	go w.readResponses()

	// Wait for ready signal
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := w.sendRequest(ctx, &rpcRequest{
		ID:     "0",
		Method: "ping",
	})
	if err != nil {
		w.close()
		return nil, fmt.Errorf("worker not ready: %w", err)
	}

	if resp.Result == nil || resp.Result.Type != "pong" {
		w.close()
		return nil, fmt.Errorf("unexpected ready response: %v", resp)
	}

	w.ready = true
	return w, nil
}

func (p *workerPool) acquire(ctx context.Context) (*worker, func(), error) {
	if p.closed.Load() {
		return nil, nil, errors.New("pool is closed")
	}

	p.mu.RLock()

	numWorkers := len(p.workers)
	startIdx := int(atomic.AddUint64(&p.nextIdx, 1) % uint64(numWorkers))

	// Find a ready worker
	for i := 0; i < numWorkers; i++ {
		idx := (startIdx + i) % numWorkers
		w := p.workers[idx]

		if w == nil {
			continue
		}

		// Check for too many failures
		if atomic.LoadInt32(&w.failures) >= 3 {
			p.mu.RUnlock()
			p.mu.Lock()
			if p.workers[idx] == w && atomic.LoadInt32(&w.failures) >= 3 {
				p.workers[idx] = nil
				go p.replaceWorkerAsync(w, idx)
			}
			p.mu.Unlock()
			p.mu.RLock()
			continue
		}

		w.mu.Lock()
		if w.ready {
			w.mu.Unlock()
			p.mu.RUnlock()
			return w, func() {}, nil
		}
		w.mu.Unlock()
	}
	p.mu.RUnlock()

	// No ready workers - start a temporary one
	w, err := p.startWorker()
	if err != nil {
		return nil, nil, err
	}

	return w, func() { w.close() }, nil
}

func (p *workerPool) replaceWorkerAsync(oldWorker *worker, idx int) {
	oldWorker.close()

	newWorker, err := p.startWorker()
	if err != nil || p.closed.Load() {
		if newWorker != nil {
			newWorker.close()
		}
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed.Load() && p.workers[idx] == nil {
		p.workers[idx] = newWorker
	} else {
		newWorker.close()
	}
}

func (p *workerPool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, w := range p.workers {
		if w != nil {
			w.close()
			p.workers[i] = nil
		}
	}
}

// ============================================================================
// Worker Methods
// ============================================================================

func (w *worker) infer(ctx context.Context, code string) (*InferenceResult, error) {
	req := &rpcRequest{
		ID:     fmt.Sprintf("%d", atomic.AddInt64(&w.requestID, 1)),
		Method: "infer",
		Code:   code,
	}

	resp, err := w.sendRequest(ctx, req)
	if err != nil {
		atomic.AddInt32(&w.failures, 1)
		return nil, err
	}

	// Reset failures on success
	atomic.StoreInt32(&w.failures, 0)

	if resp.Error != nil {
		return nil, fmt.Errorf("inference error: %s", resp.Error.Message)
	}

	if resp.Result == nil {
		return nil, errors.New("no result returned")
	}

	return resp.Result, nil
}

func (w *worker) sendRequest(ctx context.Context, req *rpcRequest) (*rpcResponse, error) {
	// Create response channel
	respCh := make(chan *rpcResponse, 1)

	w.mu.Lock()
	w.pending[req.ID] = respCh
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.pending, req.ID)
		w.mu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	w.stdinMu.Lock()
	_, err = w.stdin.Write(append(data, '\n'))
	w.stdinMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *worker) readResponses() {
	for {
		line, err := w.stdout.ReadBytes('\n')
		if err != nil {
			// Worker died
			w.mu.Lock()
			w.ready = false
			// Signal all pending requests
			for id, ch := range w.pending {
				ch <- &rpcResponse{
					ID: id,
					Error: &struct {
						Message string `json:"message"`
						Stack   string `json:"stack,omitempty"`
					}{Message: "worker died"},
				}
			}
			w.mu.Unlock()
			return
		}

		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		w.mu.Lock()
		if ch, ok := w.pending[resp.ID]; ok {
			ch <- &resp
		}
		w.mu.Unlock()
	}
}

func (w *worker) close() {
	w.mu.Lock()
	w.ready = false
	w.mu.Unlock()

	if w.stdin != nil {
		_ = w.stdin.Close()
	}

	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
	}
}
