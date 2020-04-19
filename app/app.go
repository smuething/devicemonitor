package app

import (
	"context"
	"sync"
	"time"
)

var wg *sync.WaitGroup = &sync.WaitGroup{}
var backgroundCtx context.Context
var ctx context.Context
var cancel context.CancelFunc

func init() {
	backgroundCtx = context.Background()
	ctx, cancel = context.WithCancel(backgroundCtx)
}

func Go(f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func GoWithError(f func() error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := f()
		if err != nil {
			panic(err)
		}
	}()
}

func Shutdown(ctx context.Context) bool {

	cancel()

	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		return true
	case <-ctx.Done():
		return false
	}
}

func Context() context.Context {
	return ctx
}

func ContextWithTimeout(timeout time.Duration, nested bool) (context.Context, context.CancelFunc) {
	if nested {
		return context.WithTimeout(ctx, timeout)
	} else {
		return context.WithTimeout(backgroundCtx, timeout)
	}
}
