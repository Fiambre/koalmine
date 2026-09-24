package singleinstance

import (
	"testing"
	"time"
)

// TestAcquireSecondCallTriggersOnShow verifies the core contract: the first
// Acquire becomes primary, and a second Acquire from what would be another
// process (a should-be-impossible-in-real-life second Acquire from the same
// process, but the TCP-port mechanism can't tell the difference) fails and
// wakes the primary's onShow callback instead of also succeeding.
func TestAcquireSecondCallTriggersOnShow(t *testing.T) {
	// A real Koalmine instance may already be running on the dev machine and
	// holding the production port — use a dedicated one instead so this
	// test's outcome reflects its own logic, not that coincidence.
	original := addr
	addr = "127.0.0.1:58744"
	t.Cleanup(func() { addr = original })

	shown := make(chan struct{}, 1)
	primary, release := Acquire(func() { shown <- struct{}{} })
	if !primary {
		t.Fatal("expected the first Acquire to become the primary instance")
	}
	t.Cleanup(release)

	secondary, _ := Acquire(func() { t.Error("onShow should not be invoked on the secondary instance") })
	if secondary {
		t.Fatal("expected the second Acquire to fail to acquire the lock")
	}

	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("primary instance's onShow was not invoked after a secondary Acquire")
	}
}
