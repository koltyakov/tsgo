// Package bun provides a Bun-based TypeScript execution engine.
package bun

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koltyakov/tsgo/internal/types"
)

//go:embed worker.ts
var embeddedWorker []byte

// Config configures the Bun execution engine.
type Config struct {
	// PoolSize sets the number of pre-warmed Bun processes.
	PoolSize int
	// ExecutablePath overrides the path to the Bun executable.
	ExecutablePath string
	// WorkerScript provides a custom worker script (empty = use embedded).
	WorkerScript string
	// HealthCheckInterval sets the interval for process health checks.
	HealthCheckInterval time.Duration
}

// Engine executes TypeScript using external Bun processes.
type Engine struct {
	config     Config
	pool       *pool
	workerPath string
	available  bool
}

// New creates a new Bun execution engine.
func New(cfg Config) (*Engine, error) {
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 4
	}
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = 5 * time.Second
	}

	// Find Bun executable
	bunPath := cfg.ExecutablePath
	if bunPath == "" {
		var err error
		bunPath, err = exec.LookPath("bun")
		if err != nil {
			return &Engine{
				config:    cfg,
				available: false,
			}, nil
		}
	} else {
		// Verify the specified path exists
		if _, err := os.Stat(bunPath); err != nil {
			return &Engine{
				config:    cfg,
				available: false,
			}, nil
		}
	}

	// Set up worker script
	workerPath, err := setupWorkerScript(cfg.WorkerScript)
	if err != nil {
		return nil, fmt.Errorf("failed to setup worker script: %w", err)
	}

	engine := &Engine{
		config:     cfg,
		workerPath: workerPath,
		available:  true,
	}

	// Create process pool
	engine.pool = newPool(cfg.PoolSize, bunPath, workerPath)

	// Start health checker
	go engine.pool.healthChecker(cfg.HealthCheckInterval)

	return engine, nil
}

// setupWorkerScript sets up the worker script, either from config or embedded.
func setupWorkerScript(customScript string) (string, error) {
	if customScript != "" {
		// Use custom script - write to temp file
		tmpFile, err := os.CreateTemp("", "tsgo-worker-*.ts")
		if err != nil {
			return "", err
		}
		if _, err := tmpFile.WriteString(customScript); err != nil {
			tmpFile.Close()
			return "", err
		}
		tmpFile.Close()
		return tmpFile.Name(), nil
	}

	// Use embedded worker
	content := embeddedWorker

	// Write to temp directory
	tmpDir, err := os.MkdirTemp("", "tsgo-worker")
	if err != nil {
		return "", err
	}

	workerPath := filepath.Join(tmpDir, "bun_worker.ts")
	if err := os.WriteFile(workerPath, content, 0644); err != nil {
		return "", err
	}

	return workerPath, nil
}

// Execute runs TypeScript code in a Bun process.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	if !e.available {
		return nil, fmt.Errorf("bun engine is not available")
	}

	start := time.Now()

	// Get process from pool
	proc, release, err := e.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire bun process: %w", err)
	}
	defer release()

	// Prepare context
	context := make(map[string]any)
	for k, v := range globals {
		context[k] = v
	}

	// Send execute request
	resp, err := proc.execute(ctx, code, context)
	if err != nil {
		return nil, err
	}

	result := &types.Result{
		Metrics: types.ExecutionMetrics{
			ExecutionTime: time.Since(start),
			TotalTime:     time.Since(start),
		},
		EngineUsed: types.EngineBun,
	}

	if resp.Error != nil {
		return nil, &types.ExecutionError{
			Message: resp.Error.Message,
			Stack:   resp.Error.Stack,
		}
	}

	result.Value = resp.Result

	if resp.Metrics != nil {
		result.Metrics.ExecutionTime = time.Duration(resp.Metrics.ExecutionTimeMs * float64(time.Millisecond))
	}

	return result, nil
}

// Close releases engine resources.
func (e *Engine) Close() error {
	if e.pool != nil {
		e.pool.close()
	}
	return nil
}

// IsAvailable returns true if Bun is installed and available.
func (e *Engine) IsAvailable() bool {
	return e.available
}

// pool manages a pool of Bun processes.
type pool struct {
	processes  []*process
	bunPath    string
	workerPath string
	mu         sync.Mutex
	closed     bool
	stopCh     chan struct{}
	nextIdx    uint64 // for round-robin selection
}

type process struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdinMu   sync.Mutex // protects stdin writes
	stdout    *bufio.Reader
	pending   map[string]chan *response
	mu        sync.Mutex
	ready     bool
	lastUsed  time.Time
	failures  int32
	requestID int64
}

type request struct {
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Code    string         `json:"code,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

type response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
		Stack   string `json:"stack,omitempty"`
	} `json:"error,omitempty"`
	Metrics *struct {
		ExecutionTimeMs float64 `json:"executionTimeMs"`
	} `json:"metrics,omitempty"`
}

func newPool(size int, bunPath, workerPath string) *pool {
	p := &pool{
		processes:  make([]*process, size),
		bunPath:    bunPath,
		workerPath: workerPath,
		stopCh:     make(chan struct{}),
	}

	// Start initial processes
	for i := 0; i < size; i++ {
		proc, err := p.startProcess()
		if err != nil {
			// Log error but continue - we can start processes on demand
			continue
		}
		p.processes[i] = proc
	}

	return p
}

func (p *pool) startProcess() (*process, error) {
	cmd := exec.Command(p.bunPath, "run", p.workerPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}

	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}

	proc := &process{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   bufio.NewReader(stdout),
		pending:  make(map[string]chan *response),
		lastUsed: time.Now(),
	}

	// Start response reader
	go proc.readResponses()

	// Wait for ready signal
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := proc.sendRequest(ctx, &request{
		ID:     "0",
		Method: "ping",
	})
	if err != nil {
		proc.close()
		return nil, fmt.Errorf("process not ready: %w", err)
	}

	if resp.Result != "pong" && resp.Result != "ready" {
		proc.close()
		return nil, fmt.Errorf("unexpected ready response: %v", resp.Result)
	}

	proc.ready = true
	return proc, nil
}

func (p *pool) acquire(ctx context.Context) (*process, func(), error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, nil, fmt.Errorf("pool is closed")
	}

	// Round-robin selection - each process can handle multiple concurrent requests
	numProcs := len(p.processes)
	startIdx := int(atomic.AddUint64(&p.nextIdx, 1) % uint64(numProcs))

	// Try to find a ready process
	for i := 0; i < numProcs; i++ {
		idx := (startIdx + i) % numProcs
		proc := p.processes[idx]

		if proc == nil {
			continue
		}

		// Check if process needs restart due to failures
		if atomic.LoadInt32(&proc.failures) >= 3 {
			p.processes[idx] = nil
			go func(oldProc *process, pidx int) {
				oldProc.close()
				newProc, err := p.startProcess()
				if err == nil {
					p.mu.Lock()
					if !p.closed {
						p.processes[pidx] = newProc
					}
					p.mu.Unlock()
				}
			}(proc, idx)
			continue
		}

		proc.mu.Lock()
		if proc.ready {
			proc.lastUsed = time.Now()
			proc.mu.Unlock()
			p.mu.Unlock()

			// No release function needed - process handles concurrent requests
			return proc, func() {}, nil
		}
		proc.mu.Unlock()
	}
	p.mu.Unlock()

	// No ready processes, start a new one (this should be rare)
	proc, err := p.startProcess()
	if err != nil {
		return nil, nil, err
	}

	return proc, func() {
		proc.close()
	}, nil
}

func (p *pool) healthChecker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			for i, proc := range p.processes {
				if proc == nil {
					// Start new process
					newProc, err := p.startProcess()
					if err == nil {
						p.processes[i] = newProc
					}
					continue
				}

				// Ping check - processes can handle concurrent requests
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				_, err := proc.sendRequest(ctx, &request{
					ID:     fmt.Sprintf("health-%d", time.Now().UnixNano()),
					Method: "ping",
				})
				cancel()

				if err != nil {
					atomic.AddInt32(&proc.failures, 1)
				} else {
					atomic.StoreInt32(&proc.failures, 0)
				}
			}
			p.mu.Unlock()
		}
	}
}

func (p *pool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	p.closed = true
	close(p.stopCh)

	for _, proc := range p.processes {
		if proc != nil {
			proc.close()
		}
	}
}

func (proc *process) execute(ctx context.Context, code string, context map[string]any) (*response, error) {
	id := fmt.Sprintf("exec-%d", atomic.AddInt64(&proc.requestID, 1))

	return proc.sendRequest(ctx, &request{
		ID:      id,
		Method:  "execute",
		Code:    code,
		Context: context,
	})
}

func (proc *process) sendRequest(ctx context.Context, req *request) (*response, error) {
	respCh := make(chan *response, 1)

	proc.mu.Lock()
	proc.pending[req.ID] = respCh
	proc.mu.Unlock()

	defer func() {
		proc.mu.Lock()
		delete(proc.pending, req.ID)
		proc.mu.Unlock()
	}()

	// Send request - protect stdin writes
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	proc.stdinMu.Lock()
	_, err = proc.stdin.Write(append(data, '\n'))
	proc.stdinMu.Unlock()

	if err != nil {
		atomic.AddInt32(&proc.failures, 1)
		return nil, err
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (proc *process) readResponses() {
	for {
		line, err := proc.stdout.ReadBytes('\n')
		if err != nil {
			// Process died
			atomic.AddInt32(&proc.failures, 1)
			return
		}

		var resp response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		proc.mu.Lock()
		if ch, ok := proc.pending[resp.ID]; ok {
			select {
			case ch <- &resp:
			default:
			}
		}
		proc.mu.Unlock()
	}
}

func (proc *process) close() {
	proc.mu.Lock()
	defer proc.mu.Unlock()

	if proc.stdin != nil {
		// Send shutdown request
		req := &request{ID: "shutdown", Method: "shutdown"}
		data, _ := json.Marshal(req)
		proc.stdin.Write(append(data, '\n'))
		proc.stdin.Close()
	}

	if proc.cmd != nil && proc.cmd.Process != nil {
		// Give it a moment to shutdown gracefully
		done := make(chan error, 1)
		go func() { done <- proc.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			proc.cmd.Process.Kill()
		}
	}
}
