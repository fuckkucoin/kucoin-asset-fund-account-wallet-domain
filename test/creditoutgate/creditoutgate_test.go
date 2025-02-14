package creditoutgate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/creditoutgate"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────────────────────

func newTestService(t *testing.T) *creditoutgate.Service {
	t.Helper()
	fixed := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	seq := int64(0)
	svc := creditoutgate.New().WithClock(func() time.Time {
		seq++
		return fixed.Add(time.Duration(seq) * time.Second)
	})
	return svc
}

// mockOutbox CreditOutboxReader，支持注入"外部已贷记"场景。
type mockOutbox struct {
	records map[string]struct {
		epoch		int64
		token		string
		amount		string
		delivered	bool
	}
}

func newMockOutbox() *mockOutbox {
	return &mockOutbox{records: map[string]struct {
		epoch		int64
		token		string
		amount		string
		delivered	bool
	}{}}
}

func (m *mockOutbox) CreditByKey(key string) (epoch int64, token string, amount string, delivered bool, ok bool) {
	r, exists := m.records[key]
	if !exists {
		return 0, "", "", false, false
	}
	return r.epoch, r.token, r.amount, r.delivered, true
}

func (m *mockOutbox) setDelivered(key string, epoch int64, token, amount string) {
	m.records[key] = struct {
		epoch		int64
		token		string
		amount		string
		delivered	bool
	}{epoch: epoch, token: token, amount: amount, delivered: true}
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. EnqueueCredit 绑定板卡世代
// ─────────────────────────────────────────────────────────────────────────────

func TestEnqueue(t *testing.T) {
	tests := []struct {
		name		string
		boardKey	string
		creditKey	string
		epoch		int64
		token		string
		amount		string
		wantErr		error
		wantState	creditoutgate.LeaseState
		wantReplay	bool
	}{
		{
			name:		"正常入队",
			boardKey:	"ETH|USDT",
			creditKey:	"ETH|0xabc|user1|USDT",
			epoch:		1,
			token:		"tok001",
			amount:		"100.00",
			wantState:	creditoutgate.LeaseEnqueued,
		},
		{
			name:		"空 boardKey 拒绝",
			boardKey:	"",
			creditKey:	"ETH|0xabc|user1|USDT",
			epoch:		1,
			token:		"tok001",
			amount:		"100.00",
			wantErr:	creditoutgate.ErrBoardKey,
		},
		{
			name:		"空 creditKey 拒绝",
			boardKey:	"ETH|USDT",
			creditKey:	"",
			epoch:		1,
			token:		"tok001",
			amount:		"100.00",
			wantErr:	creditoutgate.ErrCreditKey,
		},
		{
			name:		"epoch=0 拒绝",
			boardKey:	"ETH|USDT",
			creditKey:	"ETH|0xabc|user1|USDT",
			epoch:		0,
			token:		"tok001",
			amount:		"100.00",
			wantErr:	creditoutgate.ErrEpochToken,
		},
		{
			name:		"空 token 拒绝",
			boardKey:	"ETH|USDT",
			creditKey:	"ETH|0xabc|user1|USDT",
			epoch:		1,
			token:		"",
			amount:		"100.00",
			wantErr:	creditoutgate.ErrEpochToken,
		},
		{
			name:		"空 amount 拒绝",
			boardKey:	"ETH|USDT",
			creditKey:	"ETH|0xabc|user1|USDT",
			epoch:		1,
			token:		"tok001",
			amount:		"",
			wantErr:	creditoutgate.ErrAmount,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			res, err := svc.Enqueue(tc.boardKey, tc.creditKey, tc.epoch, tc.token, tc.amount)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Lease.State != tc.wantState {
				t.Errorf("lease state: want %s, got %s", tc.wantState, res.Lease.State)
			}
			if res.Replay != tc.wantReplay {
				t.Errorf("replay: want %v, got %v", tc.wantReplay, res.Replay)
			}
		})
	}
}

func TestEnqueueIdempotency(t *testing.T) {
	svc := newTestService(t)
	// 第一次入队。
	r1, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if r1.Replay {
		t.Error("first enqueue should not be replay")
	}
	// 同 key 同金额幂等。
	r2, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if !r2.Replay {
		t.Error("second enqueue with same amount should be replay")
	}
	if r2.Lease.ID != r1.Lease.ID {
		t.Error("idempotent enqueue should return same lease ID")
	}
}

func TestEnqueueAmountConflict(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// 同 key 不同金额。
	_, err = svc.Enqueue("ETH|USDT", "key1", 2, "tok2", "200")
	if err != creditoutgate.ErrAmountConflict {
		t.Fatalf("expected ErrAmountConflict, got %v", err)
	}
	// 必须有 CaseAmountConflict 证明。
	cases := svc.CasesByKey("key1")
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].Kind != creditoutgate.CaseAmountConflict {
		t.Errorf("case kind: want %s, got %s", creditoutgate.CaseAmountConflict, cases[0].Kind)
	}
}

func TestEnqueueAfterBoardClosed(t *testing.T) {
	svc := newTestService(t)
	// 关闭板卡。
	_, err := svc.CloseFence("BTC|BTC", "ops")
	if err != nil {
		t.Fatalf("close fence: %v", err)
	}
	// 关闭后 Enqueue 必须失败。
	_, err = svc.Enqueue("BTC|BTC", "key1", 1, "tok1", "1.0")
	if err != creditoutgate.ErrBoardClosed {
		t.Fatalf("expected ErrBoardClosed, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. DeliverCredit 复验世代
// ─────────────────────────────────────────────────────────────────────────────

func TestDeliver_Normal(t *testing.T) {
	svc := newTestService(t)
	r, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	res, err := svc.Deliver("key1", r.Lease.Epoch, r.Lease.Token)
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if res.Proof == nil {
		t.Fatal("proof must be set on successful deliver")
	}
	if res.Replay {
		t.Error("first deliver must not be replay")
	}
	if res.Proof.Amount != "100" {
		t.Errorf("proof amount: want 100, got %s", res.Proof.Amount)
	}
	// 检查租约状态。
	l, ok := svc.LeaseByKey("key1")
	if !ok {
		t.Fatal("lease should exist")
	}
	if l.State != creditoutgate.LeaseDelivered {
		t.Errorf("lease state after deliver: want delivered, got %s", l.State)
	}
}

func TestDeliver_Idempotent(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 第一次 Deliver。
	r1, err := svc.Deliver("key1", 1, "tok1")
	if err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	// 第二次 Deliver（幂等）。
	r2, err := svc.Deliver("key1", 1, "tok1")
	if err != nil {
		t.Fatalf("second deliver: %v", err)
	}
	if !r2.Replay {
		t.Error("second deliver must be replay")
	}
	if r2.Proof == nil {
		t.Fatal("replay deliver must return existing proof")
	}
	if r1.Proof.ID != r2.Proof.ID {
		t.Error("idempotent deliver must return same proof ID")
	}
	// 不得产生新 CreditFenceCase。
	cases := svc.AllCases()
	if len(cases) != 0 {
		t.Errorf("idempotent deliver must not produce cases, got %d", len(cases))
	}
}

func TestDeliver_StaleToken(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 旧 token 重放。
	_, err = svc.Deliver("key1", 99, "tok-stale")
	if err != creditoutgate.ErrEpochToken {
		t.Fatalf("expected ErrEpochToken, got %v", err)
	}
	// 必须落 CaseStaleReplay。
	cases := svc.CasesByKey("key1")
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].Kind != creditoutgate.CaseStaleReplay {
		t.Errorf("case kind: want stale_replay, got %s", cases[0].Kind)
	}
	// 正确 token 仍可成功。
	_, err = svc.Deliver("key1", 1, "tok1")
	if err != nil {
		t.Fatalf("deliver with correct token: %v", err)
	}
}

func TestDeliver_LeaseNotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Deliver("nonexistent", 1, "tok")
	if err != creditoutgate.ErrLeaseNotFound {
		t.Fatalf("expected ErrLeaseNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. 验收 1：EnqueueCredit 后关闭板卡护栏，Deliver 必须失败并落 CreditFenceCase
// ─────────────────────────────────────────────────────────────────────────────

func TestAcceptance_FenceCloseAfterEnqueue(t *testing.T) {
	svc := newTestService(t)
	// Step 1: Enqueue。
	r, err := svc.Enqueue("ETH|USDT", "ETH|0xabc|user1|USDT", 1, "tok001", "100.00")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if r.Lease.State != creditoutgate.LeaseEnqueued {
		t.Errorf("lease state after enqueue: want enqueued, got %s", r.Lease.State)
	}
	// Step 2: 关闭板卡护栏。
	flipRes, err := svc.CloseFence("ETH|USDT", "ops-alice")
	if err != nil {
		t.Fatalf("close fence: %v", err)
	}
	if !flipRes.NewGen.Closed {
		t.Error("board must be closed after CloseFence")
	}
	if flipRes.NewGen.WorldGen != 2 {
		t.Errorf("world gen after close: want 2, got %d", flipRes.NewGen.WorldGen)
	}
	if flipRes.RevokedCount != 1 {
		t.Errorf("revoked count: want 1, got %d", flipRes.RevokedCount)
	}
	// 吊销时须产生事故证明。
	if len(flipRes.Cases) != 1 {
		t.Fatalf("expected 1 case from FenceFlip, got %d", len(flipRes.Cases))
	}
	if flipRes.Cases[0].Kind != creditoutgate.CaseFenceClose {
		t.Errorf("case kind: want fence_close, got %s", flipRes.Cases[0].Kind)
	}
	// Step 3: 持旧 epoch/token Deliver 必须失败。
	_, err = svc.Deliver("ETH|0xabc|user1|USDT", 1, "tok001")
	if err != creditoutgate.ErrLeaseRevoked {
		t.Fatalf("expected ErrLeaseRevoked, got %v", err)
	}
	// 失败时追加新事故证明（种类：fence_close）。
	cases := svc.CasesByKey("ETH|0xabc|user1|USDT")
	hasFC := false
	for _, c := range cases {
		if c.Kind == creditoutgate.CaseFenceClose {
			hasFC = true
			break
		}
	}
	if !hasFC {
		t.Error("must have CaseFenceClose after deliver on revoked lease")
	}
	// 不得产生 CreditProof。
	proofs := svc.AllProofs()
	if len(proofs) != 0 {
		t.Errorf("must not produce CreditProof after fence close deliver, got %d", len(proofs))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. 验收 2：已 Deliver 的 key 重放 Deliver 幂等成功且不二次贷记
// ─────────────────────────────────────────────────────────────────────────────

func TestAcceptance_IdempotentDeliverNoDoubleCredit(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("TRX|USDT", "TRX|0x111|user4|USDT", 1, "tok-idem", "500.00")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 第一次 Deliver（正常）。
	r1, err := svc.Deliver("TRX|0x111|user4|USDT", 1, "tok-idem")
	if err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	if r1.Replay {
		t.Error("first deliver must not be replay")
	}
	if r1.Proof == nil {
		t.Fatal("first deliver must produce proof")
	}
	// 第二次 Deliver（重放）。
	r2, err := svc.Deliver("TRX|0x111|user4|USDT", 1, "tok-idem")
	if err != nil {
		t.Fatalf("second deliver: %v", err)
	}
	if !r2.Replay {
		t.Error("second deliver must be replay")
	}
	// 不得出现第二条 CreditProof。
	proofs := svc.AllProofs()
	if len(proofs) != 1 {
		t.Errorf("must have exactly 1 proof, got %d", len(proofs))
	}
	// 不得产生事故证明。
	cases := svc.AllCases()
	if len(cases) != 0 {
		t.Errorf("must not produce fence cases on idempotent deliver, got %d", len(cases))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. 验收 3：同 key 不同 amount Enqueue 冲突失败
// ─────────────────────────────────────────────────────────────────────────────

func TestAcceptance_SameKeyDifferentAmountConflict(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|ETH", "ETH|0xfed|user3|ETH", 1, "tok-first", "10.00")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	_, err = svc.Enqueue("ETH|ETH", "ETH|0xfed|user3|ETH", 2, "tok-second", "99.99")
	if err != creditoutgate.ErrAmountConflict {
		t.Fatalf("expected ErrAmountConflict, got %v", err)
	}
	cases := svc.CasesByKey("ETH|0xfed|user3|ETH")
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].Kind != creditoutgate.CaseAmountConflict {
		t.Errorf("case kind: want amount_conflict, got %s", cases[0].Kind)
	}
	// 租约状态不变（仍 enqueued）。
	l, ok := svc.LeaseByKey("ETH|0xfed|user3|ETH")
	if !ok {
		t.Fatal("lease must still exist after conflict")
	}
	if l.State != creditoutgate.LeaseEnqueued {
		t.Errorf("lease state after conflict: want enqueued, got %s", l.State)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. FenceFlip 护栏翻转（不关闭，仅抬升世代）
// ─────────────────────────────────────────────────────────────────────────────

func TestFenceFlip_GenDrift(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("SOL|USDC", "SOL|0xaaa|user5|USDC", 1, "tok-gen", "200.00")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	flipRes, err := svc.FenceFlip(creditoutgate.FenceFlipReq{
		BoardKey:	"SOL|USDC",
		LaneFlip:	true,
		Close:		false,
		Operator:	"ops-bob",
	})
	if err != nil {
		t.Fatalf("fence flip: %v", err)
	}
	if flipRes.NewGen.WorldGen != 2 {
		t.Errorf("world gen after flip: want 2, got %d", flipRes.NewGen.WorldGen)
	}
	if flipRes.NewGen.Closed {
		t.Error("board must not be closed after non-close flip")
	}
	if flipRes.RevokedCount != 1 {
		t.Errorf("revoked count: want 1, got %d", flipRes.RevokedCount)
	}
	if len(flipRes.Cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(flipRes.Cases))
	}
	if flipRes.Cases[0].Kind != creditoutgate.CaseGenDrift {
		t.Errorf("case kind: want gen_drift, got %s", flipRes.Cases[0].Kind)
	}
	// 翻转后 Deliver 必须失败。
	_, err = svc.Deliver("SOL|0xaaa|user5|USDC", 1, "tok-gen")
	if err != creditoutgate.ErrLeaseRevoked {
		t.Fatalf("expected ErrLeaseRevoked after gen drift, got %v", err)
	}
	// 板卡未关闭，可以重新 Enqueue。
	_, err = svc.Enqueue("SOL|USDC", "SOL|0xaaa|user5|USDC-new", 1, "tok-new", "200.00")
	if err != nil {
		t.Fatalf("re-enqueue after non-close flip: %v", err)
	}
}

func TestFenceFlip_NoActiveLeasesNoRevoke(t *testing.T) {
	svc := newTestService(t)
	// 翻转一个没有活跃租约的板卡。
	flipRes, err := svc.FenceFlip(creditoutgate.FenceFlipReq{
		BoardKey:	"BTC|BTC",
		LaneFlip:	false,
		Close:		false,
		Operator:	"ops-ops",
	})
	if err != nil {
		t.Fatalf("fence flip with no leases: %v", err)
	}
	if flipRes.RevokedCount != 0 {
		t.Errorf("expect 0 revoked, got %d", flipRes.RevokedCount)
	}
	if len(flipRes.Cases) != 0 {
		t.Errorf("expect 0 cases, got %d", len(flipRes.Cases))
	}
}

func TestFenceFlip_MissingOperator(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.FenceFlip(creditoutgate.FenceFlipReq{BoardKey: "ETH|USDT", Operator: ""})
	if err != creditoutgate.ErrOperator {
		t.Fatalf("expected ErrOperator, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. ReopenFence 重开板卡
// ─────────────────────────────────────────────────────────────────────────────

func TestReopenFence(t *testing.T) {
	svc := newTestService(t)
	// 关闭后重开。
	_, err := svc.CloseFence("ETH|USDT", "ops")
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	reopenRes, err := svc.ReopenFence("ETH|USDT", "ops")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopenRes.NewGen.Closed {
		t.Error("board must not be closed after reopen")
	}
	// 重开后可以 Enqueue。
	_, err = svc.Enqueue("ETH|USDT", "newkey", 1, "tok1", "50.00")
	if err != nil {
		t.Fatalf("enqueue after reopen: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. DeliverPending 批量投递
// ─────────────────────────────────────────────────────────────────────────────

func TestDeliverPending(t *testing.T) {
	svc := newTestService(t)
	// 创建 3 条租约。
	keys := []string{"key1", "key2", "key3"}
	for i, k := range keys {
		_, err := svc.Enqueue("ETH|USDT", k, int64(i+1), "tok"+k, "10")
		if err != nil {
			t.Fatalf("enqueue %s: %v", k, err)
		}
	}
	// 批量投递（limit=10）。
	res := svc.DeliverPending(10)
	if res.Requested != 3 {
		t.Errorf("requested: want 3, got %d", res.Requested)
	}
	if res.Delivered != 3 {
		t.Errorf("delivered: want 3, got %d", res.Delivered)
	}
	if res.Failed != 0 {
		t.Errorf("failed: want 0, got %d", res.Failed)
	}
	// 再次批量投递（全部已完成，无活跃）。
	res2 := svc.DeliverPending(10)
	if res2.Requested != 0 {
		t.Errorf("second batch requested: want 0, got %d", res2.Requested)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 9. PatrolAllBoards 安全网巡检
// ─────────────────────────────────────────────────────────────────────────────

func TestPatrolAllBoards_SafetyNet(t *testing.T) {
	svc := newTestService(t)
	// 手动注入一个"漏网"的 enqueued 租约（不通过 FenceFlip 而是直接关闭板卡）。
	_, err := svc.Enqueue("ETH|USDT", "leak-key", 1, "tok-leak", "10")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 直接操作内部状态漏网（不使用 FenceFlip）。
	svc.mu.Lock()
	bg := svc.gens["ETH|USDT"]
	if bg != nil {
		bg.Closed = true
	}
	svc.mu.Unlock()
	// PatrolAllBoards 应发现并吊销漏网租约。
	found := svc.PatrolAllBoards()
	if len(found) != 1 {
		t.Fatalf("safety net should find 1 leak, got %d", len(found))
	}
	if found[0].Kind != creditoutgate.CaseGenDrift {
		t.Errorf("case kind: want gen_drift, got %s", found[0].Kind)
	}
	// 吊销后租约应为 revoked。
	l, ok := svc.LeaseByKey("leak-key")
	if !ok {
		t.Fatal("lease must still exist")
	}
	if l.State != creditoutgate.LeaseRevoked {
		t.Errorf("lease state after safety net: want revoked, got %s", l.State)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 10. Patrol 双贷记巡检（外部 CreditOutbox 场景）
// ─────────────────────────────────────────────────────────────────────────────

func TestPatrol_DualCredit(t *testing.T) {
	svc := newTestService(t)
	outbox := newMockOutbox()
	svc.BindOutbox(outbox)
	// Enqueue + Deliver。
	_, err := svc.Enqueue("ETH|USDT", "dual-key", 1, "tok-dual", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	_, err = svc.Deliver("dual-key", 1, "tok-dual")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	// 外部 CreditOutbox 以不同 epoch 已贷记（护栏外漏双贷记）。
	outbox.setDelivered("dual-key", 99, "tok-external", "100")
	// 巡检应发现双贷记。
	res := svc.Patrol(64)
	if res.Detected != 1 {
		t.Errorf("patrol detected: want 1, got %d", res.Detected)
	}
	if len(res.Cases) != 1 {
		t.Fatalf("patrol cases: want 1, got %d", len(res.Cases))
	}
	if res.Cases[0].Kind != creditoutgate.CaseDualCredit {
		t.Errorf("case kind: want dual_credit, got %s", res.Cases[0].Kind)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 11. 证明导出
// ─────────────────────────────────────────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	svc := newTestService(t)
	// 创建并完成投递。
	_, err := svc.Enqueue("ETH|USDT", "exp-key", 1, "tok-exp", "50")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	_, err = svc.Deliver("exp-key", 1, "tok-exp")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	// 第一次导出：应包含 1 条 proof。
	exp1 := svc.ExportProofs(128)
	if len(exp1.Proofs) != 1 {
		t.Fatalf("first export: want 1 proof, got %d", len(exp1.Proofs))
	}
	if exp1.Proofs[0].ExportedAt == nil {
		t.Error("proof ExportedAt must be set after export")
	}
	// 第二次导出：已导出的不再出现。
	exp2 := svc.ExportProofs(128)
	if len(exp2.Proofs) != 0 {
		t.Errorf("second export: want 0 proofs, got %d", len(exp2.Proofs))
	}
	// 但 TotalProofs 仍然是 1。
	if exp2.TotalProofs != 1 {
		t.Errorf("total proofs: want 1, got %d", exp2.TotalProofs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 12. GenReader 接口（只读世代视图）
// ─────────────────────────────────────────────────────────────────────────────

func TestGenReader(t *testing.T) {
	svc := newTestService(t)
	// 初始：GenOf 未知板卡返回 false。
	_, ok := svc.GenOf("ETH|USDT")
	if ok {
		t.Error("GenOf for unknown board should return false")
	}
	// Enqueue 初始化板卡世代。
	_, err := svc.Enqueue("ETH|USDT", "key1", 1, "tok1", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	gv, ok := svc.GenOf("ETH|USDT")
	if !ok {
		t.Fatal("GenOf must return true after enqueue")
	}
	if gv.WorldGen != 1 {
		t.Errorf("world gen: want 1, got %d", gv.WorldGen)
	}
	// IsGenValid 校验。
	if !svc.IsGenValid("ETH|USDT", 1, 1) {
		t.Error("IsGenValid(1,1) must return true initially")
	}
	// FenceFlip 后世代变化。
	_, err = svc.FenceFlip(creditoutgate.FenceFlipReq{BoardKey: "ETH|USDT", LaneFlip: false, Close: false, Operator: "ops"})
	if err != nil {
		t.Fatalf("flip: %v", err)
	}
	if svc.IsGenValid("ETH|USDT", 1, 1) {
		t.Error("IsGenValid(1,1) must return false after flip")
	}
	if !svc.IsGenValid("ETH|USDT", 2, 1) {
		t.Error("IsGenValid(2,1) must return true after flip")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 13. Deliver_GenDrift（世代漂移但租约未吊销 — 理论上不会发生，防御性）
// ─────────────────────────────────────────────────────────────────────────────

func TestDeliver_GenDriftDirect(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|USDT", "drift-key", 1, "tok-drift", "100")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 直接抬升世代而不吊销租约（安全网前的竞态窗口）。
	svc.mu.Lock()
	bg := svc.gens["ETH|USDT"]
	if bg != nil {
		bg.WorldGen = 5
	}
	svc.mu.Unlock()
	// Deliver 应发现世代漂移并拒绝。
	_, err = svc.Deliver("drift-key", 1, "tok-drift")
	if err != creditoutgate.ErrGenDrift {
		t.Fatalf("expected ErrGenDrift, got %v", err)
	}
	cases := svc.CasesByKey("drift-key")
	if len(cases) == 0 {
		t.Fatal("must produce case on gen drift")
	}
	if cases[0].Kind != creditoutgate.CaseGenDrift {
		t.Errorf("case kind: want gen_drift, got %s", cases[0].Kind)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 14. 多板卡并发场景
// ─────────────────────────────────────────────────────────────────────────────

func TestMultiBoard_Isolation(t *testing.T) {
	svc := newTestService(t)
	// 在两个板卡各 Enqueue 一条。
	_, err := svc.Enqueue("ETH|USDT", "eth-key", 1, "tok-eth", "100")
	if err != nil {
		t.Fatalf("eth enqueue: %v", err)
	}
	_, err = svc.Enqueue("BTC|BTC", "btc-key", 1, "tok-btc", "0.1")
	if err != nil {
		t.Fatalf("btc enqueue: %v", err)
	}
	// 关闭 ETH|USDT，不影响 BTC|BTC。
	flipRes, err := svc.CloseFence("ETH|USDT", "ops")
	if err != nil {
		t.Fatalf("close eth fence: %v", err)
	}
	if flipRes.RevokedCount != 1 {
		t.Errorf("revoked count: want 1, got %d", flipRes.RevokedCount)
	}
	// BTC|BTC 的租约仍应 enqueued。
	l, ok := svc.LeaseByKey("btc-key")
	if !ok {
		t.Fatal("btc lease must exist")
	}
	if l.State != creditoutgate.LeaseEnqueued {
		t.Errorf("btc lease state: want enqueued, got %s", l.State)
	}
	// BTC Deliver 应正常成功。
	_, err = svc.Deliver("btc-key", 1, "tok-btc")
	if err != nil {
		t.Fatalf("btc deliver: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 15. Deliver_ClosedBoard（板卡关闭且租约未被吊销 — 防御性校验）
// ─────────────────────────────────────────────────────────────────────────────

func TestDeliver_ClosedBoardDirect(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Enqueue("ETH|USDT", "closed-key", 1, "tok-closed", "50")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 直接标记板卡为关闭，不经过 FenceFlip（竞态安全网场景）。
	svc.mu.Lock()
	bg := svc.gens["ETH|USDT"]
	if bg != nil {
		bg.Closed = true
		bg.WorldGen = 3
	}
	svc.mu.Unlock()
	// Deliver 应检测到 closed=true 并拒绝，追加 CaseFenceClose。
	_, err = svc.Deliver("closed-key", 1, "tok-closed")
	if err != creditoutgate.ErrGenDrift {
		t.Fatalf("expected ErrGenDrift (closed board), got %v", err)
	}
	cases := svc.CasesByKey("closed-key")
	if len(cases) == 0 {
		t.Fatal("must produce case on closed board deliver")
	}
	if cases[0].Kind != creditoutgate.CaseFenceClose {
		t.Errorf("case kind: want fence_close (closed board), got %s", cases[0].Kind)
	}
}
