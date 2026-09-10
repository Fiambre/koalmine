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
	shown := make(chan struct{}, 1)
	primary := Acquire(func() { shown <- struct{}{} })
	if !primary {
		t.Fatal("expected the first Acquire to become the primary instance")
	}

	secondary := Acquire(func() { t.Error("onShow should not be invoked on the secondary instance") })
	if secondary {
		t.Fatal("expected the second Acquire to fail to acquire the lock")
	}

	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("primary instance's onShow was not invoked after a secondary Acquire")
	}
}
