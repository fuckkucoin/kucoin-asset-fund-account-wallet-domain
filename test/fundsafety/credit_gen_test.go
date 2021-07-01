package fundsafety_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundsafety"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock 实现
// ─────────────────────────────────────────────────────────────────────────────

type recordingNotifier struct {
	mu	sync.Mutex
	flips	[]flipEvent
}

type flipEvent struct {
	boardKey	string
	laneFlip	bool
	close		bool
	operator	string
}

func (r *recordingNotifier) OnBoardFlip(boardKey string, laneFlip, close bool, operator string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flips = append(r.flips, flipEvent{boardKey, laneFlip, close, operator})
}

func (r *recordingNotifier) Flips() []flipEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]flipEvent, len(r.flips))
	copy(out, r.flips)
	return out
}

type recordingHook struct {
	mu	sync.Mutex
	events	[]enqueueEvent
}

type enqueueEvent struct {
	boardKey	string
	creditKey	string
	epoch		int64
	token		string
	amount		string
}

func (h *recordingHook) OnEnqueue(boardKey, creditKey string, epoch int64, token, amount string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, enqueueEvent{boardKey, fundsafety.CreditKey, epoch, token, amount})
}

func (h *recordingHook) Events() []enqueueEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]enqueueEvent, len(h.events))
	copy(out, h.events)
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────

func TestBindCreditGenNotifier_Single(t *testing.T) {
	svc := fundsafety.New()
	n := &recordingNotifier{}
	svc.BindCreditGenNotifier(n)
	svc.notifyBoardFlip("ETH|USDT", true, true, "ops")
	flips := n.Flips()
	if len(flips) != 1 {
		t.Fatalf("expected 1 flip event, got %d", len(flips))
	}
	if flips[0].boardKey != "ETH|USDT" {
		t.Errorf("boardKey: want ETH|USDT, got %s", flips[0].boardKey)
	}
	if !flips[0].laneFlip {
		t.Error("laneFlip should be true")
	}
	if !flips[0].close {
		t.Error("close should be true")
	}
	if flips[0].operator != "ops" {
		t.Errorf("operator: want ops, got %s", flips[0].operator)
	}
}

func TestBindCreditGenNotifier_Multiple(t *testing.T) {
	svc := fundsafety.New()
	n1 := &recordingNotifier{}
	n2 := &recordingNotifier{}
	svc.BindCreditGenNotifier(n1)
	svc.BindCreditGenNotifier(n2)
	svc.notifyBoardFlip("BTC|BTC", false, false, "ops2")
	for _, n := range []*recordingNotifier{n1, n2} {
		flips := n.Flips()
		if len(flips) != 1 {
			t.Fatalf("expected 1 flip event per notifier, got %d", len(flips))
		}
		if flips[0].boardKey != "BTC|BTC" {
			t.Errorf("boardKey: want BTC|BTC, got %s", flips[0].boardKey)
		}
	}
}

func TestBindCreditEnqueueHook(t *testing.T) {
	svc := fundsafety.New()
	h := &recordingHook{}
	svc.BindCreditEnqueueHook(h)
	svc.notifyEnqueue("ETH|USDT", "key1", 1, "tok1", "100")
	events := h.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 enqueue event, got %d", len(events))
	}
	if events[0].creditKey != "key1" {
		t.Errorf("creditKey: want key1, got %s", events[0].creditKey)
	}
	if events[0].epoch != 1 {
		t.Errorf("epoch: want 1, got %d", events[0].epoch)
	}
}

func TestNotifyBoardFlip_NilNotifier(t *testing.T) {
	svc := fundsafety.
	// nil 通知器不崩溃。
	New()

	svc.BindCreditGenNotifier(nil)
	svc.notifyBoardFlip("ETH|USDT", false, false, "ops")
}

func TestGuardBoardLaneWithNotify_Denied(t *testing.T) {
	svc := fundsafety.New()
	n := &recordingNotifier{}
	svc.BindCreditGenNotifier(n)
	// Halt 状态下 GuardBoardLane 应拒绝，触发通知。
	svc.SetHalt(true)
	err := svc.GuardBoardLaneWithNotify(context.Background(), "deposit", "user1", "ETH", "USDT", "ops-test")
	if err == nil {
		t.Fatal("expected error when halted")
	}
	// 等待异步通知（goroutine）。
	time.Sleep(10 * time.Millisecond)
	flips := n.Flips()
	if len(flips) == 0 {
		t.Error("expected flip notification when GuardBoardLane denied")
	}
}

func TestBoardKeyOf(t *testing.T) {
	svc := fundsafety.New()
	tests := []struct {
		chain, asset	string
		want		string
	}{
		{"eth", "usdt", "ETH|USDT"},
		{"BTC", "BTC", "BTC|BTC"},
		{" eth ", " usdt ", "ETH|USDT"},
		{"TRX", "usdt", "TRX|USDT"},
	}
	for _, tc := range tests {
		got := svc.BoardKeyOf(tc.chain, tc.asset)
		if got != tc.want {
			t.Errorf("BoardKeyOf(%q,%q): want %s, got %s", tc.chain, tc.asset, tc.want, got)
		}
	}
}

func TestCreditBoardKeyFrom(t *testing.T) {
	tests := []struct {
		chain, asset	string
		want		string
	}{
		{"eth", "usdt", "ETH|USDT"},
		{"BTC", "BTC", "BTC|BTC"},
		{"  sol  ", "  usdc  ", "SOL|USDC"},
	}
	for _, tc := range tests {
		got := fundsafety.CreditBoardKeyFrom(tc.chain, tc.asset)
		if got != tc.want {
			t.Errorf("creditBoardKeyFrom(%q,%q): want %s, got %s", tc.chain, tc.asset, tc.want, got)
		}
	}
}

func TestBindCreditEnqueueHook_Nil(t *testing.T) {
	svc := fundsafety. // nil 钩子不崩溃。
	New()

	svc.BindCreditEnqueueHook(nil)
	svc.notifyEnqueue("ETH|USDT", "key1", 1, "tok1", "100")
}
