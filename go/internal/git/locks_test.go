package git

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepoLocksSameRepoSerializes(t *testing.T) {
	// Arrange
	locks := NewRepoLocks()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var activeCount int32
	var maxActiveCount int32
	var secondCompletedBeforeRelease int32
	var wg sync.WaitGroup
	wg.Add(2)

	// Act
	go func() {
		defer wg.Done()
		err := locks.WithRepo("github:acme/api", func() error {
			currentActive := atomic.AddInt32(&activeCount, 1)
			updateMaxInt32(&maxActiveCount, currentActive)
			close(firstStarted)
			<-releaseFirst
			atomic.AddInt32(&activeCount, -1)
			return nil
		})
		if err != nil {
			t.Errorf("WithRepo returned error: %v", err)
		}
	}()

	<-firstStarted
	go func() {
		defer wg.Done()
		err := locks.WithRepo("github:acme/api", func() error {
			currentActive := atomic.AddInt32(&activeCount, 1)
			updateMaxInt32(&maxActiveCount, currentActive)
			atomic.AddInt32(&activeCount, -1)
			return nil
		})
		if err != nil {
			t.Errorf("WithRepo returned error: %v", err)
		}
		if atomic.LoadInt32(&activeCount) > 0 {
			atomic.StoreInt32(&secondCompletedBeforeRelease, 1)
		}
	}()

	time.Sleep(25 * time.Millisecond)
	if atomic.LoadInt32(&secondCompletedBeforeRelease) != 0 {
		t.Fatal("second repo operation completed before the first operation released the lock")
	}
	close(releaseFirst)
	wg.Wait()

	// Assert
	if got := atomic.LoadInt32(&maxActiveCount); got != 1 {
		t.Fatalf("same repo operations ran concurrently; max active = %d", got)
	}
}

func TestRepoLocksDifferentReposRunConcurrently(t *testing.T) {
	// Arrange
	locks := NewRepoLocks()
	firstStarted := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Act
	go func() {
		defer wg.Done()
		err := locks.WithRepo("github:acme/api", func() error {
			close(firstStarted)
			<-releaseFirst
			return nil
		})
		if err != nil {
			t.Errorf("WithRepo returned error: %v", err)
		}
	}()

	<-firstStarted
	go func() {
		defer wg.Done()
		err := locks.WithRepo("github:acme/web", func() error {
			close(secondEntered)
			return nil
		})
		if err != nil {
			t.Errorf("WithRepo returned error: %v", err)
		}
	}()

	select {
	case <-secondEntered:
		// expected
	case <-time.After(250 * time.Millisecond):
		t.Fatal("different repo operation did not enter while first repo operation was active")
	}
	close(releaseFirst)
	wg.Wait()
}

func TestRepoLocksMaintenanceWaitsForInFlightRepo(t *testing.T) {
	// Arrange
	locks := NewRepoLocks()
	repoStarted := make(chan struct{})
	releaseRepo := make(chan struct{})
	maintenanceEntered := make(chan struct{})
	var repoCompleted int32
	var wg sync.WaitGroup
	wg.Add(2)

	// Act
	go func() {
		defer wg.Done()
		err := locks.WithRepo("github:acme/api", func() error {
			close(repoStarted)
			<-releaseRepo
			atomic.StoreInt32(&repoCompleted, 1)
			return nil
		})
		if err != nil {
			t.Errorf("WithRepo returned error: %v", err)
		}
	}()

	<-repoStarted
	go func() {
		defer wg.Done()
		err := locks.WithMaintenance(func() error {
			close(maintenanceEntered)
			return nil
		})
		if err != nil {
			t.Errorf("WithMaintenance returned error: %v", err)
		}
	}()

	select {
	case <-maintenanceEntered:
		t.Fatal("maintenance started before in-flight repo operation finished")
	case <-time.After(25 * time.Millisecond):
		// expected
	}

	close(releaseRepo)
	select {
	case <-maintenanceEntered:
		// expected
	case <-time.After(250 * time.Millisecond):
		t.Fatal("maintenance did not start after repo operation finished")
	}
	wg.Wait()

	// Assert
	if atomic.LoadInt32(&repoCompleted) != 1 {
		t.Fatal("repo operation did not complete before maintenance started")
	}
}

func updateMaxInt32(maxValue *int32, candidate int32) {
	for {
		currentMax := atomic.LoadInt32(maxValue)
		if candidate <= currentMax {
			return
		}
		if atomic.CompareAndSwapInt32(maxValue, currentMax, candidate) {
			return
		}
	}
}
