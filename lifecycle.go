package main

import "sync"

// appCleanup releases acquired resources in reverse order on native shutdown
// as well as startup errors, where the Wails run loop may never return.
type appCleanup struct {
	once  sync.Once
	steps []func()
}

func (c *appCleanup) add(step func()) { c.steps = append(c.steps, step) }

func (c *appCleanup) close() {
	c.once.Do(func() {
		for i := len(c.steps) - 1; i >= 0; i-- {
			c.steps[i]()
		}
	})
}

// quitCoordinator keeps the native thread pumping until updater UI work drains.
type quitCoordinator struct {
	mu       sync.Mutex
	stopping bool
	ready    bool
	stop     func()
	quit     func()
}

func (q *quitCoordinator) shouldQuit() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ready {
		return true
	}
	if q.stopping {
		return false
	}
	q.stopping = true
	go func() {
		q.stop()
		q.mu.Lock()
		q.ready = true
		q.mu.Unlock()
		q.quit()
	}()
	return false
}
