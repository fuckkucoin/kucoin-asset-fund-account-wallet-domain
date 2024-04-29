package holdopsgate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func TestTokenMutexReleaseClose(t *testing.T) {
	m := holdopsgate.NewTokenMutex()
	if err := m.TryHold(holdopsgate.OpsRelease, "tok-1", "gap-1", "t-a"); err != nil {
		t.Fatal(err)
	}
	if err := m.TryHold(holdopsgate.OpsGapClose, "tok-1", "gap-1", "t-b"); err != holdopsgate.ErrReleaseClose {
		t.Fatalf("err=%v", err)
	}
	if err := m.TryHold(holdopsgate.OpsRelease, "tok-1", "gap-1", "t-c"); err != holdopsgate.ErrDualRelease {
		t.Fatalf("err=%v", err)
	}
	m.Release(holdopsgate.OpsRelease, "tok-1", "gap-1", "t-a")
	if err := m.TryHold(holdopsgate.OpsGapClose, "tok-1", "gap-1", "t-b"); err != nil {
		t.Fatal(err)
	}
}

func TestAckDoesNotShareReleaseKey(t *testing.T) {
	m := holdopsgate.NewTokenMutex()
	if err := m.TryHold(holdopsgate.OpsRelease, "tok-2", "gap-2", "t-r"); err != nil {
		t.Fatal(err)
	}
	if err := m.TryHold(holdopsgate.OpsGapAck, "tok-2", "gap-2", "t-ack"); err != nil {
		t.Fatal(err)
	}
}
