package main

import (
	"context"
	"log"
	"sync"

	"github.com/koltyakov/tsgo"
)

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
		p.auto = newPlaygroundExecutor(tsgo.EngineAuto)
		p.goja = newPlaygroundExecutor(tsgo.EngineGOJA)
		p.bun = newPlaygroundExecutor(tsgo.EngineBun)
		go p.warmup()
	})
}

func newPlaygroundExecutor(engine tsgo.EngineType) *tsgo.Executor {
	return tsgo.New(
		tsgo.WithEngine(engine),
		tsgo.WithTimeout(executorTimeout),
		tsgo.WithSecurity(playgroundSecurityPolicy()),
	)
}

func playgroundSecurityPolicy() tsgo.SecurityPolicy {
	return tsgo.SecurityPolicy{
		NetworkAccess:  true,
		AllowedGlobals: []string{"fetch", "process", "eval"},
	}
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
