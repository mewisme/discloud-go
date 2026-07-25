package ui

import (
	"context"
	"os"
	"os/signal"
	"sync"
)

// ContextWithInterrupt cancels when the user presses Ctrl+C.
// On Windows it also watches the console input buffer so ConPTY sessions
// still cancel when os/signal does not deliver Interrupt.
func ContextWithInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	var once sync.Once
	fire := func() { once.Do(cancel) }

	sigCtx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt)
	go func() {
		select {
		case <-sigCtx.Done():
			fire()
		case <-ctx.Done():
		}
		stopSig()
	}()

	stopCon := startConsoleInterruptWatch(ctx, fire)
	return ctx, func() {
		stopCon()
		stopSig()
		fire()
	}
}
