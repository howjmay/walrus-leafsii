package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
type Group struct {
	mu sync.Mutex       // protects m
	m  map[string]*call // lazily initialized
}

// call is an in-flight or completed singleflight.Do call
type call struct {
	wg sync.WaitGroup

	// These fields are written once before the WaitGroup is done
	// and are only read after the WaitGroup is done.
	val interface{}
	err error

	// These fields are read and written with the singleflight
	// mutex held before the WaitGroup is done, and are read but
	// not written after the WaitGroup is done.
	dups      int
	chans     []chan<- Result
	forgotten bool
}

// Result holds the results of Do, so they can be passed
// on a channel.
type Result struct {
	Val    interface{}
	Err    error
	Shared bool
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in, the duplicate caller waits for the
// original to complete and receives the same results.
// The return value shared indicates whether v was given to multiple callers.
func (g *Group) Do(key string, fn func() (interface{}, error)) (v interface{}, err error, shared bool) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		g.mu.Unlock()
		c.wg.Wait()

		if e, ok := c.err.(*panicError); ok {
			panic(e)
		} else if c.err == errGoexit {
			runtime.Goexit()
		}
		return c.val, c.err, true
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	g.doCall(c, key, fn)
	return c.val, c.err, c.dups > 0
}

// DoChan is like Do but returns a channel that will receive the
// results when they are ready.
//
// The returned channel will not be closed.
func (g *Group) DoChan(key string, fn func() (interface{}, error)) <-chan Result {
	ch := make(chan Result, 1)
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		c.chans = append(c.chans, ch)
		g.mu.Unlock()
		return ch
	}
	c := &call{chans: []chan<- Result{ch}}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	go g.doCall(c, key, fn)

	return ch
}

// doCall handles the single call for a key.
func (g *Group) doCall(c *call, key string, fn func() (interface{}, error)) {
	normalReturn := false
	recovered := false

	// use double-defer to distinguish panic from runtime.Goexit,
	// more details see https://golang.org/cl/134395
	defer func() {
		// the order of the following lines is important
		// if the panic happens before the recover, then the defer panic could not be covered
		// defer func() {
		//   if r := recover(); r != nil {
		//     panic(r)
		//   }
		// }()
		// defer func() {
		//   recover()
		// }()
		if !normalReturn {
			// Ideally, we would wait to take a stack trace until we've determined
			// whether this is a panic or a runtime.Goexit.
			//
			// Unfortunately, the only way we can distinguish the two is to see
			// whether recover returns nil (which stacktrace does not save correctly)
			// or to see whether the recovered value is a function (see below)
			// which must be caught before the stack is unwound
			// which seems to require an inline deferred function.
			c.err = newPanicError(recover())
		}
	}()

	func() {
		defer func() {
			if !recovered {
				// Normal case: recover the result
				if r := recover(); r != nil {
					c.err = newPanicError(r)
					recovered = true
				}
			}
		}()

		c.val, c.err = fn()
		normalReturn = true
	}()

	if !normalReturn && recovered {
		recovered = false
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if !c.forgotten {
		delete(g.m, key)
	}

	if e, ok := c.err.(*panicError); ok {
		// In order to prevent the waiting channels from being blocked forever,
		// needs to ensure that this panic cannot be recovered.
		if len(c.chans) > 0 {
			go panic(e)
			select {} // Keep this goroutine around so that it will appear in the crash dump.
		} else {
			panic(e)
		}
	} else if c.err == errGoexit {
		// Already in the process of goexit, no need to call again.
	} else {
		// Normal return
		for _, ch := range c.chans {
			ch <- Result{c.val, c.err, c.dups > 0}
		}
	}
}

// Forget tells the singleflight to forget about a key.  Future calls
// to Do for this key will call the function rather than waiting for
// an earlier call to complete.
func (g *Group) Forget(key string) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		c.forgotten = true
	}
	delete(g.m, key)
	g.mu.Unlock()
}

func newPanicError(v interface{}) error {
	stack := debug.Stack()

	// The first line of the stack trace is of the form "goroutine N [status]:"
	// but by the time the panic reaches Do the goroutine may no longer exist
	// and its status will have changed. Trim out the misleading line.
	if line := bytes.IndexByte(stack, '\n'); line >= 0 {
		stack = stack[line+1:]
	}
	return &panicError{value: v, stack: stack}
}

// panicError is an arbitrary value recovered from a panic
// with the stack trace during the execution of given function.
type panicError struct {
	value interface{}
	stack []byte
}

// Error implements error interface.
func (p *panicError) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}

func (p *panicError) Unwrap() error {
	err, ok := p.value.(error)
	if !ok {
		return nil
	}

	return err
}

// errGoexit indicates the runtime.Goexit was called in
// the user given function.
var errGoexit = errors.New("runtime.Goexit was called")

// WithContext wraps a function to add context support with timeout
func (g *Group) DoWithContext(ctx context.Context, key string, fn func(ctx context.Context) (interface{}, error)) (interface{}, error, bool) {
	type result struct {
		val    interface{}
		err    error
		shared bool
	}

	ch := make(chan result, 1)

	go func() {
		val, err, shared := g.Do(key, func() (interface{}, error) {
			return fn(ctx)
		})
		ch <- result{val, err, shared}
	}()

	select {
	case r := <-ch:
		return r.val, r.err, r.shared
	case <-ctx.Done():
		return nil, ctx.Err(), false
	}
}

// DoWithTimeout is a convenience method that adds timeout support
func (g *Group) DoWithTimeout(key string, timeout time.Duration, fn func() (interface{}, error)) (interface{}, error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return g.DoWithContext(ctx, key, func(ctx context.Context) (interface{}, error) {
		return fn()
	})
}
