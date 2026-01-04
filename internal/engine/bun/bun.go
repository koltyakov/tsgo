// Package bun provides a Bun-based TypeScript execution engine.
//
// Bun is a high-performance JavaScript/TypeScript runtime that provides
// native async/await support, fetch API, and other Web APIs that aren't
// available in pure-Go runtimes like GOJA.
package bun

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
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koltyakov/tsgo/internal/types"
)

//go:embed worker.ts
var embeddedWorker []byte

// ============================================================================
// Configuration
// ============================================================================

// Pool size limits.
const (
	MinPoolSize     = 2
	MaxPoolSize     = 8
	DefaultPoolSize = 0 // 0 means auto-detect based on CPU count
)

// Default timing values.
const (
	DefaultHealthCheckInterval = 5 * time.Second
	ProcessStartupTimeout      = 2 * time.Second
	ProcessShutdownTimeout     = 2 * time.Second
	HealthCheckTimeout         = 500 * time.Millisecond
)

// Config configures the Bun execution engine.
type Config struct {
	// PoolSize sets the maximum number of Bun processes.
	PoolSize int
	// MinPoolSize sets the minimum number of pre-warmed processes (lazy mode).
	// If 0, all PoolSize processes are started immediately (eager mode).
	MinPoolSize int
	// ExecutablePath overrides the path to the Bun executable.
	ExecutablePath string
	// WorkerScript provides a custom worker script (empty = use embedded).
	WorkerScript string
	// HealthCheckInterval sets the interval for process health checks.
	HealthCheckInterval time.Duration
	// MaxRequestsPerProcess sets how many requests a process handles before recycling.
	// If 0, processes are never recycled (default for short-lived usage).
	MaxRequestsPerProcess int64
	// MaxProcessAge sets the maximum age before a process is recycled.
	// If 0, processes are never recycled by age.
	MaxProcessAge time.Duration
	// QueueSize sets the maximum number of queued requests when all processes are busy.
	// If 0, requests block until a process is available.
	QueueSize int
	// BackgroundWarmup starts processes in background goroutines.
	// New() returns immediately; first request may wait for process startup.
	// This reduces cold start from ~120ms to <1ms.
	BackgroundWarmup bool
}

// ============================================================================
// Engine
// ============================================================================

// Engine executes TypeScript using external Bun processes.
type Engine struct {
	config     Config
	pool       *pool
	workerPath string
	tempDir    string
	available  bool
	closed     atomic.Bool
}

// New creates a new Bun execution engine.
func New(cfg Config) (*Engine, error) {
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = runtime.NumCPU() / 2 // Bun processes are heavier
		if poolSize < MinPoolSize {
			poolSize = MinPoolSize
		}
		if poolSize > MaxPoolSize {
			poolSize = MaxPoolSize
		}
	}

	healthInterval := cfg.HealthCheckInterval
	if healthInterval <= 0 {
		healthInterval = DefaultHealthCheckInterval
	}

	// Find Bun executable
	bunPath, err := findBunExecutable(cfg.ExecutablePath)
	if err != nil {
		return &Engine{config: cfg, available: false}, nil
	}

	// Set up worker script
	workerPath, tempDir, err := setupWorkerScript(cfg.WorkerScript)
	if err != nil {
		return nil, fmt.Errorf("failed to setup worker script: %w", err)
	}

	engine := &Engine{
		config:     cfg,
		workerPath: workerPath,
		tempDir:    tempDir,
		available:  true,
	}

	// Create process pool
	engine.pool = newPool(poolConfig{
		size:               poolSize,
		minSize:            cfg.MinPoolSize,
		maxRequestsPerProc: cfg.MaxRequestsPerProcess,
		maxProcessAge:      cfg.MaxProcessAge,
		queueSize:          cfg.QueueSize,
		backgroundWarmup:   cfg.BackgroundWarmup,
	}, bunPath, workerPath)

	// Start health checker
	go engine.pool.healthChecker(healthInterval)

	return engine, nil
}

// findBunExecutable locates the Bun executable.
func findBunExecutable(customPath string) (string, error) {
	if customPath != "" {
		if _, err := os.Stat(customPath); err != nil {
			return "", err
		}
		return customPath, nil
	}
	return exec.LookPath("bun")
}

// setupWorkerScript sets up the worker script, either from config or embedded.
// Returns the worker path and temp directory (for cleanup).
func setupWorkerScript(customScript string) (workerPath string, tempDir string, err error) {
	if customScript != "" {
		// Use custom script - write to temp file
		tmpDir, err := os.MkdirTemp("", "tsgo-worker")
		if err != nil {
			return "", "", err
		}
		path := filepath.Join(tmpDir, "custom_worker.ts")
		if err := os.WriteFile(path, []byte(customScript), 0600); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", "", err
		}
		return path, tmpDir, nil
	}

	// Use embedded worker
	content := embeddedWorker

	// Write to temp directory
	tmpDir, err := os.MkdirTemp("", "tsgo-worker")
	if err != nil {
		return "", "", err
	}

	path := filepath.Join(tmpDir, "bun_worker.ts")
	if err := os.WriteFile(path, content, 0600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", err
	}

	return path, tmpDir, nil
}

// ============================================================================
// Execution
// ============================================================================

// Execute runs TypeScript code in a Bun process.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	if e.closed.Load() {
		return nil, errors.New("bun engine is closed")
	}
	if !e.available {
		return nil, errors.New("bun engine is not available")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now()

	// Get process from pool
	proc, release, err := e.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire bun process: %w", err)
	}
	defer release()

	// Send execute request
	resp, err := proc.execute(ctx, code, globals)
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

// ============================================================================
// Lifecycle
// ============================================================================

// Close releases engine resources.
// It is idempotent and safe to call multiple times.
func (e *Engine) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error

	if e.pool != nil {
		e.pool.close()
	}

	if e.tempDir != "" {
		if err := os.RemoveAll(e.tempDir); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up temp dir: %w", err))
		}
	}

	return errors.Join(errs...)
}

// IsAvailable returns true if Bun is installed and available.
func (e *Engine) IsAvailable() bool {
	return e.available
}

// ============================================================================
// Process Pool
// ============================================================================

// pool manages a pool of Bun processes.
type pool struct {
	processes  []*process
	bunPath    string
	workerPath string
	mu         sync.RWMutex
	closed     atomic.Bool
	stopCh     chan struct{}
	nextIdx    uint64 // For round-robin selection

	// Service mode settings
	minSize            int
	maxRequestsPerProc int64
	maxProcessAge      time.Duration

	// Request queue for backpressure
	queueSem    chan struct{}
	activeCount int32
}

// poolConfig holds service mode settings for the pool.
type poolConfig struct {
	size               int
	minSize            int
	maxRequestsPerProc int64
	maxProcessAge      time.Duration
	queueSize          int
	backgroundWarmup   bool
}

// ============================================================================
// Process Types
// ============================================================================

// process represents a single Bun worker process.
type process struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdinMu      sync.Mutex
	stdout       *bufio.Reader
	pending      map[string]chan *response
	mu           sync.Mutex
	ready        bool
	lastUsed     time.Time
	failures     int32
	requestID    int64
	requestCount int64
	startTime    time.Time
}

// request represents a JSON-RPC request to the worker.
type request struct {
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Code    string         `json:"code,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

// response represents a JSON-RPC response from the worker.
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

// ============================================================================
// Pool Management
// ============================================================================

func newPool(cfg poolConfig, bunPath, workerPath string) *pool {
	p := &pool{
		processes:          make([]*process, cfg.size),
		bunPath:            bunPath,
		workerPath:         workerPath,
		stopCh:             make(chan struct{}),
		minSize:            cfg.minSize,
		maxRequestsPerProc: cfg.maxRequestsPerProc,
		maxProcessAge:      cfg.maxProcessAge,
	}

	if cfg.queueSize > 0 {
		p.queueSem = make(chan struct{}, cfg.queueSize)
	}

	// Determine initial size (lazy mode starts minSize, eager starts all)
	initialSize := cfg.size
	if cfg.minSize > 0 && cfg.minSize < cfg.size {
		initialSize = cfg.minSize
	}

	if cfg.backgroundWarmup {
		go p.warmupBackground(initialSize)
	} else {
		p.warmupBlocking(initialSize)
	}

	return p
}

// warmupBackground starts processes in the background.
func (p *pool) warmupBackground(count int) {
	for i := 0; i < count; i++ {
		if p.closed.Load() {
			return
		}
		proc, err := p.startProcess()
		if err != nil {
			continue
		}
		p.mu.Lock()
		if !p.closed.Load() {
			p.processes[i] = proc
			atomic.AddInt32(&p.activeCount, 1)
		} else {
			proc.close()
		}
		p.mu.Unlock()
	}
}

// warmupBlocking starts processes synchronously.
func (p *pool) warmupBlocking(count int) {
	for i := 0; i < count; i++ {
		proc, err := p.startProcess()
		if err != nil {
			continue
		}
		p.processes[i] = proc
		atomic.AddInt32(&p.activeCount, 1)
	}
}

// ============================================================================
// Process Lifecycle
// ============================================================================

func (p *pool) startProcess() (*process, error) {
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

	proc := &process{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		pending:   make(map[string]chan *response),
		lastUsed:  time.Now(),
		startTime: time.Now(),
	}

	go proc.readResponses()

	// Wait for ready signal
	if err := proc.waitForReady(); err != nil {
		proc.close()
		return nil, err
	}

	proc.ready = true
	return proc, nil
}

// waitForReady waits for the process to signal readiness.
func (proc *process) waitForReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), ProcessStartupTimeout)
	defer cancel()

	resp, err := proc.sendRequest(ctx, &request{
		ID:     "0",
		Method: "ping",
	})
	if err != nil {
		return fmt.Errorf("process not ready: %w", err)
	}

	if resp.Result != "pong" && resp.Result != "ready" {
		return fmt.Errorf("unexpected ready response: %v", resp.Result)
	}

	return nil
}

// ============================================================================
// Pool Acquire/Release
// ============================================================================

func (p *pool) acquire(ctx context.Context) (*process, func(), error) {
	if p.closed.Load() {
		return nil, nil, errors.New("pool is closed")
	}

	// If queue is configured, try to acquire a slot (backpressure)
	if p.queueSem != nil {
		select {
		case p.queueSem <- struct{}{}:
			// Got a slot
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	p.mu.RLock()

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
			p.mu.RUnlock()
			p.mu.Lock()
			// Double-check after acquiring write lock
			if p.processes[idx] == proc && atomic.LoadInt32(&proc.failures) >= 3 {
				p.processes[idx] = nil
				atomic.AddInt32(&p.activeCount, -1)
				p.replaceProcessAsync(proc, idx, true)
			}
			p.mu.Unlock()
			p.mu.RLock()
			continue
		}

		// Check if process needs recycling (request count or age)
		if p.needsRecycle(proc) {
			p.mu.RUnlock()
			p.mu.Lock()
			if p.processes[idx] == proc && p.needsRecycle(proc) {
				p.processes[idx] = nil
				atomic.AddInt32(&p.activeCount, -1)
				p.replaceProcessAsync(proc, idx, true)
			}
			p.mu.Unlock()
			p.mu.RLock()
			continue
		}

		proc.mu.Lock()
		if proc.ready {
			proc.lastUsed = time.Now()
			proc.mu.Unlock()
			p.mu.RUnlock()

			// Release queue slot when done
			return proc, p.releaseQueueSlot, nil
		}
		proc.mu.Unlock()
	}
	p.mu.RUnlock()

	// No ready processes - try to scale up (lazy mode)
	proc, _, err := p.tryScaleUp()
	if err == nil {
		return proc, p.releaseQueueSlot, nil
	}

	// All slots full, start temporary process
	proc, err = p.startProcess()
	if err != nil {
		p.releaseQueueSlot()
		return nil, nil, err
	}

	return proc, func() {
		proc.close()
		p.releaseQueueSlot()
	}, nil
}

// ============================================================================
// Process Recycling
// ============================================================================

// needsRecycle checks if a process should be recycled based on config.
func (p *pool) needsRecycle(proc *process) bool {
	if p.maxRequestsPerProc > 0 && atomic.LoadInt64(&proc.requestCount) >= p.maxRequestsPerProc {
		return true
	}
	if p.maxProcessAge > 0 && time.Since(proc.startTime) >= p.maxProcessAge {
		return true
	}
	return false
}

// replaceProcessAsync closes the old process and starts a new one asynchronously.
func (p *pool) replaceProcessAsync(oldProc *process, idx int, restartAlways bool) {
	go func() {
		oldProc.close()
		if !restartAlways && p.minSize > 0 && int(atomic.LoadInt32(&p.activeCount)) >= p.minSize {
			return
		}
		newProc, err := p.startProcess()
		if err != nil || p.closed.Load() {
			if newProc != nil {
				newProc.close()
			}
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.closed.Load() && p.processes[idx] == nil {
			p.processes[idx] = newProc
			atomic.AddInt32(&p.activeCount, 1)
		} else {
			newProc.close()
		}
	}()
}

// releaseQueueSlot releases a slot in the queue semaphore.
func (p *pool) releaseQueueSlot() {
	if p.queueSem != nil {
		select {
		case <-p.queueSem:
		default:
		}
	}
}

// tryScaleUp attempts to start a new process in an empty slot (lazy scaling).
func (p *pool) tryScaleUp() (*process, int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return nil, -1, errors.New("pool is closed")
	}

	for i, proc := range p.processes {
		if proc == nil {
			newProc, err := p.startProcess()
			if err != nil {
				return nil, -1, err
			}
			p.processes[i] = newProc
			atomic.AddInt32(&p.activeCount, 1)
			return newProc, i, nil
		}
	}

	return nil, -1, errors.New("no empty slots")
}

// ============================================================================
// Health Checking
// ============================================================================

// healthChecker runs periodic health checks on pooled processes.
// It monitors process health via ping requests and handles:
// - Starting new processes when pool is below minimum size
// - Recycling processes that have exceeded their usage limits
// - Tracking failure counts for unhealthy processes
func (p *pool) healthChecker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			if p.closed.Load() {
				return
			}

			p.mu.RLock()
			processes := make([]*process, len(p.processes))
			copy(processes, p.processes)
			p.mu.RUnlock()

			for i, proc := range processes {
				if proc == nil {
					// Only start new process if below minimum pool size
					if int(atomic.LoadInt32(&p.activeCount)) < p.minSize || p.minSize == 0 {
						newProc, err := p.startProcess()
						if err == nil {
							p.mu.Lock()
							if !p.closed.Load() && p.processes[i] == nil {
								p.processes[i] = newProc
								atomic.AddInt32(&p.activeCount, 1)
							} else {
								newProc.close()
							}
							p.mu.Unlock()
						}
					}
					continue
				}

				// Check if process needs recycling (proactive cleanup during idle)
				if p.needsRecycle(proc) {
					p.mu.Lock()
					if p.processes[i] == proc {
						p.processes[i] = nil
						atomic.AddInt32(&p.activeCount, -1)
						// Respect minSize when restarting during health check
						p.replaceProcessAsync(proc, i, false)
					}
					p.mu.Unlock()
					continue
				}

				// Ping check - processes can handle concurrent requests
				ctx, cancel := context.WithTimeout(context.Background(), HealthCheckTimeout)
				_, err := proc.sendRequest(ctx, &request{
					ID:     "health-" + strconv.FormatInt(time.Now().UnixNano(), 10),
					Method: "ping",
				})
				cancel()

				if err != nil {
					atomic.AddInt32(&proc.failures, 1)
				} else {
					atomic.StoreInt32(&proc.failures, 0)
				}
			}
		}
	}
}

// ============================================================================
// Pool Shutdown
// ============================================================================

// close shuts down the pool and all its processes gracefully.
// It stops the health checker and closes all active processes.
// This method is idempotent - multiple calls are safe.
func (p *pool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return // Already closed
	}

	close(p.stopCh)

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, proc := range p.processes {
		if proc != nil {
			proc.close()
		}
	}
}

// ============================================================================
// Process Communication
// ============================================================================

// execute sends code to the Bun process for execution and waits for the result.
// It tracks request counts for process recycling decisions.
func (proc *process) execute(ctx context.Context, code string, context map[string]any) (*response, error) {
	// Use strconv for faster ID generation
	id := "exec-" + strconv.FormatInt(atomic.AddInt64(&proc.requestID, 1), 10)

	// Increment request count for recycling tracking
	atomic.AddInt64(&proc.requestCount, 1)

	return proc.sendRequest(ctx, &request{
		ID:      id,
		Method:  "execute",
		Code:    code,
		Context: context,
	})
}

// sendRequest sends a JSON-RPC style request to the Bun process.
// It handles request/response correlation via unique IDs and supports
// context cancellation for timeout handling.
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

// readResponses runs a continuous loop reading JSON responses from the Bun process.
// It dispatches responses to waiting callers via the pending channel map.
// This goroutine exits when the process stdout is closed.
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

// ============================================================================
// Process Shutdown
// ============================================================================

// close gracefully shuts down the Bun process.
// It sends a shutdown request to allow clean termination, then
// waits briefly before forcefully killing if necessary.
func (proc *process) close() {
	proc.mu.Lock()
	defer proc.mu.Unlock()

	if proc.stdin != nil {
		// Send shutdown request
		req := &request{ID: "shutdown", Method: "shutdown"}
		data, _ := json.Marshal(req)
		_, _ = proc.stdin.Write(append(data, '\n'))
		_ = proc.stdin.Close()
	}

	if proc.cmd != nil && proc.cmd.Process != nil {
		// Give it a moment to shutdown gracefully
		done := make(chan error, 1)
		go func() { done <- proc.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(ProcessShutdownTimeout):
			_ = proc.cmd.Process.Kill()
		}
	}
}
