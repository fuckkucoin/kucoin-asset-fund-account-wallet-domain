package outboxonce_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/outboxonce"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// ── 停机世代 FSM 单元 ─────────────────────────────────────────────────────

func TestHaltGenMgr_GenerationMonotone(t *testing.T) {
	now := time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC)
	m := outboxonce.NewHaltGenMgr(fixedClock(now))
	if m.CurrentGen() != 0 {
		t.Fatalf("initial gen want 0, got %d", m.CurrentGen())
	}
	lease1, _ := m.SetHalt(true, "actor-A")
	if lease1.Generation != 1 {
		t.Errorf("after SetHalt(true) gen want 1, got %d", lease1.Generation)
	}
	if lease1.State != outboxonce.HaltStateBlocking {
		t.Errorf("after SetHalt(true) state want blocking, got %s", lease1.State)
	}
	lease2, _ := m.SetHalt(false, "actor-A")
	if lease2.Generation != 2 {
		t.Errorf("after SetHalt(false) gen want 2, got %d", lease2.Generation)
	}
	if lease2.State != outboxonce.HaltStateCleared {
		t.Errorf("after SetHalt(false) state want cleared, got %s", lease2.State)
	}
	lease3, _ := m.SetHalt(true, "actor-B")
	if lease3.Generation != 3 {
		t.Errorf("after second SetHalt(true) gen want 3, got %d", lease3.Generation)
	}
	if m.CurrentGen() != 3 {
		t.Errorf("CurrentGen want 3, got %d", m.CurrentGen())
	}
}

func TestHaltGenMgr_IsHaltedAtomic(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	if m.IsHalted() {
		t.Error("initial IsHalted want false")
	}
	m.SetHalt(true, "")
	if !m.IsHalted() {
		t.Error("after SetHalt(true) IsHalted want true")
	}
	m.SetHalt(false, "")
	if m.IsHalted() {
		t.Error("after SetHalt(false) IsHalted want false")
	}
}

func TestHaltGenMgr_LockDeliver_RejectsWhenHalted(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	m.SetHalt(true, "")
	_, err := m.LockDeliver("key-1", "ev-1", 1, "tok-A")
	if err != outboxonce.ErrHalt {
		t.Errorf("want ErrHalt after SetHalt(true), got %v", err)
	}
}

func TestHaltGenMgr_LockDeliver_RejectsStaleFence(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	// LockDeliver → SetHalt(true) → AbortDeliver（记录吊销 fence）。
	dg, err := m.LockDeliver("key-2", "ev-2", 1, "tok-B")
	if err != nil {
		t.Fatalf("LockDeliver unexpected err: %v", err)
	}
	m.SetHalt(true, "")
	m.AbortDeliver(dg)
	// Halt 解除后，旧 (epoch=1, tok-B) 不得重用。
	m.SetHalt(false, "")
	_, err2 := m.LockDeliver("key-2", "ev-2", 1, "tok-B")
	if err2 != outboxonce.ErrStaleFenceRevive {
		t.Errorf("want ErrStaleFenceRevive for revoked fence, got %v", err2)
	}
}

func TestHaltGenMgr_CommitDeliver_CASFail(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	dg, err := m.LockDeliver("key-3", "ev-3", 1, "tok-C")
	if err != nil {
		t.Fatalf("LockDeliver err: %v", err)
	}
	// SetHalt(true) 在 CommitDeliver 之前发生（夹缝）。
	m.SetHalt(true, "ops")
	commitErr := m.CommitDeliver(dg)
	if commitErr != outboxonce.ErrHaltGenCAS {
		t.Errorf("want ErrHaltGenCAS, got %v", commitErr)
	}
	// 应产出 OnceHaltCase。
	cases := m.Cases()
	if len(cases) == 0 {
		t.Error("expected at least one OnceHaltCase after CAS fail")
	}
	// 吊销 fence 应被记录（来自 SetHalt 吊销扫描或 CommitDeliver）。
	if m.RevokedFenceCount() == 0 {
		t.Error("expected revokedFences > 0")
	}
}

func TestHaltGenMgr_CommitDeliver_OK(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	dg, err := m.LockDeliver("key-4", "ev-4", 1, "tok-D")
	if err != nil {
		t.Fatalf("LockDeliver err: %v", err)
	}
	if err := m.CommitDeliver(dg); err != nil {
		t.Errorf("CommitDeliver want nil, got %v", err)
	}
	if m.InFlightCount() != 0 {
		t.Errorf("InFlight want 0, got %d", m.InFlightCount())
	}
}

func TestHaltGenMgr_RevokeOnSetHalt(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	// 启动两个在途 Deliver。
	dg1, _ := m.LockDeliver("key-5", "ev-5", 1, "tok-E1")
	dg2, _ := m.LockDeliver("key-6", "ev-6", 1, "tok-E2")
	_ = dg1
	_ = dg2
	lease, revoked := m.SetHalt(true, "")
	if len(revoked) != 2 {
		t.Errorf("want 2 revoked, got %d", len(revoked))
	}
	if lease.RevokedCount != 2 {
		t.Errorf("lease.RevokedCount want 2, got %d", lease.RevokedCount)
	}
	cases := m.Cases()
	if len(cases) != 2 {
		t.Errorf("want 2 OnceHaltCases, got %d", len(cases))
	}
	for _, c := range cases {
		if c.Kind != outboxonce.CaseKindHaltRevoked {
			t.Errorf("case kind want halt_revoked, got %s", c.Kind)
		}
	}
}

func TestHaltGenMgr_DumpRestore(t *testing.T) {
	m := outboxonce.NewHaltGenMgr(nil)
	m.SetHalt(true, "actor-dump")
	dg, _ := m.LockDeliver("key-7", "ev-7", 1, "tok-F")
	_ = dg
	m.SetHalt(false, "actor-clear")
	d := m.dump()
	m2 := outboxonce.NewHaltGenMgr(nil)
	m2.restore(d)
	if m2.CurrentGen() != m.CurrentGen() {
		t.Errorf("gen mismatch: want %d got %d", m.CurrentGen(), m2.CurrentGen())
	}
	if m2.IsHalted() != m.IsHalted() {
		t.Errorf("halted mismatch")
	}
}

// ── Service 集成 ──────────────────────────────────────────────────────────

type serviceTestCase struct {
	name	string
	run	func(t *testing.T,
		s *outboxonce.Service,)
}

var serviceTests = []serviceTestCase{
	{
		name:	"enqueue_ok",
		run: func(t *testing.T, s *outboxonce.Service,) {
			ev, err := s.Enqueue(context.Background(), "k1", "transfer", `{"a":1}`)
			if err != nil {
				t.Fatalf("Enqueue err: %v", err)
			}
			if ev.Key != "k1" {
				t.Errorf("key want k1, got %s", ev.Key)
			}
			if ev.Epoch != 1 {
				t.Errorf("epoch want 1, got %d", ev.Epoch)
			}
		},
	},
	{
		name:	"enqueue_halt_blocked",
		run: func(t *testing.T, s *outboxonce.Service,) {
			s.SetHalt(true)
			_, err := s.Enqueue(context.Background(), "k2", "transfer", `{"a":2}`)
			if err != outboxonce.ErrHalt {
				t.Errorf("want ErrHalt, got %v", err)
			}
		},
	},
	{
		name:	"deliver_halt_blocked",
		run: func(t *testing.T, s *outboxonce.Service,) {
			s.Enqueue(context.Background(), "k3", "transfer", `{"a":3}`)
			s.SetHalt(true)
			_, err := s.Deliver(context.Background(), "k3", 1, "any")
			if err != outboxonce.ErrHalt {
				t.Errorf("want ErrHalt, got %v", err)
			}
		},
	},
	{
		name:	"deliver_stale_fence_revive_blocked",
		run: func(t *testing.T, s *outboxonce.Service,) {
			ev, _ := s.Enqueue(context.Background(), "k4", "transfer", `{"a":4}`)
			// 世代=0，Deliver 开始（LockDeliver）后 Halt 切入，吊销该 fence。
			// 此处直接往 hgm.revokedFences 注入。
			s.hgm.mu.Lock()
			s.hgm.revokedFences[outboxonce.FenceKey(ev.Epoch, ev.Token)] = true
			s.hgm.mu.Unlock()
			_, err := s.Deliver(context.Background(), "k4", ev.Epoch, ev.Token)
			if err != outboxonce.ErrStaleFenceRevive {
				t.Errorf("want ErrStaleFenceRevive, got %v", err)
			}
		},
	},
	{
		name:	"halt_gen_monotone_across_set_halt",
		run: func(t *testing.T, s *outboxonce.Service,) {
			if s.CurrentHaltGen() != 0 {
				t.Errorf("initial gen want 0")
			}
			s.SetHalt(true)
			if s.CurrentHaltGen() != 1 {
				t.Errorf("gen after SetHalt(true) want 1, got %d", s.CurrentHaltGen())
			}
			s.SetHalt(false)
			if s.CurrentHaltGen() != 2 {
				t.Errorf("gen after SetHalt(false) want 2, got %d", s.CurrentHaltGen())
			}
		},
	},
	{
		name:	"halt_gen_view_halted",
		run: func(t *testing.T, s *outboxonce.Service,) {
			s.SetHalt(true)
			v := s.HaltGenView()
			if !v.Halted {
				t.Error("view.Halted want true")
			}
			if v.State != outboxonce.HaltStateBlocking {
				t.Errorf("view.State want blocking, got %s", v.State)
			}
		},
	},
	{
		name:	"export_proof_empty",
		run: func(t *testing.T, s *outboxonce.Service,) {
			p := s.ExportProof()
			if p.Total != 0 {
				t.Errorf("total want 0, got %d", p.Total)
			}
		},
	},
	{
		name:	"cases_after_halt_revoke",
		run: func(t *testing.T, s *outboxonce.Service,) {
			// 入箱后在途期间切换 Halt，产出 halt_revoked case。
			ev, _ := s.Enqueue(context.Background(), "k5", "transfer", `{"a":5}`)
			// 先 LockDeliver 然后 SetHalt（夹缝）。
			dg, err := s.hgm.LockDeliver("k5", ev.ID, ev.Epoch, ev.Token)
			if err != nil {
				t.Fatalf("LockDeliver err: %v", err)
			}
			s.SetHalt(true)	// 吊销 dg
			s.hgm.AbortDeliver(dg)
			cases := s.Cases()
			if len(cases) == 0 {
				t.Error("want at least 1 case after halt revoke")
			}
		},
	},
}

func TestService_TableDriven(t *testing.T) {
	for _, tc := range serviceTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := outboxonce.New()
			tc.run(t, s)
		})
	}
}

// ── 并发竞态 ──────────────────────────────────────────────────────────────

func TestService_ConcurrentSetHalt_MonotoneGen(t *testing.T) {
	s := outboxonce.New()
	var wg sync.WaitGroup
	N := 100
	wg.Add(N)
	for i := 0; i < N; i++ {
		on := i%2 == 0
		go func(on bool) {
			defer wg.Done()
			s.SetHalt(on)
		}(on)
	}
	wg.Wait()
	gen := s.CurrentHaltGen()
	if gen != int64(N) {
		t.Errorf("gen after %d concurrent SetHalt want %d, got %d", N, N, gen)
	}
}

func TestService_ConcurrentEnqueue_SamePayloadIdempotent(t *testing.T) {
	s := outboxonce. // 同 key + 同 payload：所有并发 Enqueue 均应成功（幂等回放）。
	New()

	ok, conflict := outboxonce.RaceEnqueue(context.Background(), s, 20, "race-key", "transfer", `{"x":1}`)
	if conflict != 0 {
		t.Errorf("want 0 conflict for same payload, got %d", conflict)
	}
	if ok != 20 {
		t.Errorf("want 20 ok for idempotent enqueue, got %d", ok)
	}
}

func TestService_HaltMidDeliverRace(t *testing.T) {
	s := outboxonce.New()
	ev, err := s.Enqueue(context.Background(), "race-deliver", "transfer", `{"y":2}`)
	if err != nil {
		t.Fatalf("enqueue err: %v", err)
	}
	var wg sync.WaitGroup
	deliverResults := make(chan error,
		20)
	haltResults := make(chan *outboxonce.HaltLease, 5)
	// 启动 20 个 Deliver goroutine 与 5 个 SetHalt goroutine 同时竞争。
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Deliver(context.Background(), ev.Key, ev.Epoch, ev.Token)
			deliverResults <- err
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		on := i%2 == 0
		go func(on bool) {
			defer wg.Done()
			lease, _ := s.SetHaltByActor(on, fmt.Sprintf("actor-%v", on))
			haltResults <- lease
		}(on)
	}
	wg.Wait()
	close(deliverResults)
	close(haltResults)
	// 统计成功 Deliver 数量：最多一次。
	successCount := 0
	for err := range deliverResults {
		if err == nil {
			successCount++
		}
	}
	if successCount > 1 {
		t.Errorf("at most 1 deliver should succeed, got %d", successCount)
	}
}

func TestService_SaveLoad_PreservesHaltGen(t *testing.T) {
	ctx := context.Background()
	s := outboxonce.New()
	s.Enqueue(ctx, "persist-key", "transfer", `{"z":1}`)
	s.SetHalt(true)
	s.SetHalt(false)
	gen := s.CurrentHaltGen()
	// Save 到内存 KV。
	kv := persist.NewMemKV()
	if err := s.Save(ctx, kv, "test-slot"); err != nil {
		t.Fatalf("Save err: %v", err)
	}
	s2, err := outboxonce.Load(ctx, kv, "test-slot")
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if s2.CurrentHaltGen() != gen {
		t.Errorf("loaded gen want %d, got %d", gen, s2.CurrentHaltGen())
	}
}
