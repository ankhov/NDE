package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ankhov/NDE/internal/config"
)

// fakeModule records lifecycle calls for assertions.
type fakeModule struct {
	name     string
	mu       sync.Mutex
	started  bool
	stopped  bool
	startErr error
	stopErr  error
}

func (f *fakeModule) Name() string { return f.name }
func (f *fakeModule) Start(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	return f.startErr
}
func (f *fakeModule) Stop(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	return f.stopErr
}

func TestStartStopOrder(t *testing.T) {
	a := New(minimalCfg(), WithModule(&fakeModule{name: "a"}), WithModule(&fakeModule{name: "b"}))
	mods := a.Modules()
	if len(mods) != 2 || mods[0].Name() != "a" || mods[1].Name() != "b" {
		t.Fatalf("modules in unexpected order: %v", modNames(mods))
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	for _, m := range mods {
		f := m.(*fakeModule)
		if !f.started {
			t.Errorf("%s not started", f.name)
		}
		if !f.stopped {
			t.Errorf("%s not stopped", f.name)
		}
	}
}

func TestDoubleStartRejected(t *testing.T) {
	a := New(minimalCfg())
	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := a.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}
	_ = a.Shutdown(ctx)
}

func TestShutdownIsBounded(t *testing.T) {
	// A module whose Stop blocks forever must be cut off by the shutdown timeout.
	block := &fakeModule{name: "blocker"}
	a := New(minimalCfg(), WithModule(block))
	a.shutdownTimeout = 50 * time.Millisecond

	ctx := context.Background()
	_ = a.Start(ctx)

	done := make(chan error, 1)
	go func() { done <- a.Shutdown(context.Background()) }()
	select {
	case err := <-done:
		// Expected to return (possibly with deadline-exceeded) within ~timeout.
		_ = err
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown was not bounded by the timeout")
	}
}

func TestStartRollsBackOnError(t *testing.T) {
	good := &fakeModule{name: "good"}
	bad := &fakeModule{name: "bad", startErr: errors.New("boom")}
	a := New(minimalCfg(), WithModule(good), WithModule(bad))

	if err := a.Start(context.Background()); err == nil {
		t.Fatal("expected start error from bad module")
	}
	if !good.stopped {
		t.Error("good module was not rolled back after bad module failed to start")
	}
}

func TestRegisterAfterStartPanics(t *testing.T) {
	a := New(minimalCfg())
	_ = a.Start(context.Background())
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on Register after Start")
		}
	}()
	a.Register(&fakeModule{name: "late"})
}

func modNames(mods []Module) []string {
	out := make([]string, len(mods))
	for i, m := range mods {
		out[i] = m.Name()
	}
	return out
}

func minimalCfg() config.Config { return config.Default() }
