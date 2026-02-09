// Package rpc provides shared line-delimited JSON-RPC transport utilities.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

var (
	// ErrTransportClosed is returned when the transport is no longer writable/readable.
	ErrTransportClosed = errors.New("rpc transport is closed")
)

type transportResult struct {
	data []byte
	err  error
}

// LineClient provides request/response correlation over line-delimited JSON.
type LineClient struct {
	stdin  io.WriteCloser
	stdout *bufio.Reader

	stdinMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan transportResult
	closed  bool
}

// NewLineClient creates a transport over process stdin/stdout streams.
func NewLineClient(stdin io.WriteCloser, stdout io.Reader) *LineClient {
	return &LineClient{
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[string]chan transportResult),
	}
}

// SendRequest sends a JSON request and waits for the response with matching ID.
func (c *LineClient) SendRequest(ctx context.Context, id string, req any) ([]byte, error) {
	respCh := make(chan transportResult, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrTransportClosed
	}
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.stdinMu.Lock()
	if c.stdin == nil {
		c.stdinMu.Unlock()
		return nil, ErrTransportClosed
	}
	_, err = c.stdin.Write(append(data, '\n'))
	c.stdinMu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		return resp.data, resp.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReadResponses continuously reads and routes responses by ID.
// It should run in a dedicated goroutine per client.
func (c *LineClient) ReadResponses() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.failAllPending(err)
			return
		}

		var envelope struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil || envelope.ID == "" {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[envelope.ID]
		c.mu.Unlock()
		if !ok {
			continue
		}

		select {
		case ch <- transportResult{data: line}:
		default:
		}
	}
}

// WriteRaw writes bytes to stdin using the same serialization lock as SendRequest.
func (c *LineClient) WriteRaw(data []byte) error {
	c.stdinMu.Lock()
	defer c.stdinMu.Unlock()

	if c.stdin == nil {
		return ErrTransportClosed
	}
	_, err := c.stdin.Write(data)
	return err
}

// CloseInput closes the writable input stream.
func (c *LineClient) CloseInput() error {
	c.stdinMu.Lock()
	stdin := c.stdin
	c.stdin = nil
	c.stdinMu.Unlock()

	c.failAllPending(ErrTransportClosed)

	if stdin == nil {
		return nil
	}
	return stdin.Close()
}

func (c *LineClient) failAllPending(err error) {
	if err == nil {
		err = ErrTransportClosed
	}

	c.mu.Lock()
	c.closed = true
	pending := make([]chan transportResult, 0, len(c.pending))
	for _, ch := range c.pending {
		pending = append(pending, ch)
	}
	c.mu.Unlock()

	for _, ch := range pending {
		select {
		case ch <- transportResult{err: err}:
		default:
		}
	}
}
