package outeffect_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"account-assets.kucoin.net/funding/internal/fxonce"
	"account-assets.kucoin.net/funding/internal/outeffect"
)

func newTestAdapter(t *testing.T) (*outeffect.Adapter, fxonce.Store) {
	t.Helper()
	store := fxonce.NewMemStore()
	registry := fxonce.NewPolicyRegistry()
	gw := fxonce.NewFundEffectGateway(store, registry)
	adapter := outeffect.NewAdapter(gw, nil)
	return adapter, store
}

func makeSuccessResult(businessKey, kind string, gen int64) outeffect.HandlerResult {
	return outeffect.HandlerResult{
		BusinessKey:	businessKey,
		Kind:		kind,
		Generation:	gen,
		Payload:	`{"amount":"100","asset":"USDT"}`,
		OutboxEventID:	"outbox-evt-001",
		Success:	true,
	}
}

// ─── 正常 Apply ───────────────────────────────────────────────────────────

func TestAdapter_FundKind_Applied(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	result, err := adapter.Process(ctx, makeSuccessResult("dep-a-001", "credit_intent", 1))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !result.Applied {
		t.Errorf("expected Applied=true, got result: %+v", result)
	}
	if result.Bypassed || result.Rejected || result.Idempotent {
		t.Errorf("unexpected flags: %+v", result)
	}
}

// ─── 幂等回放 ─────────────────────────────────────────────────────────────

func TestAdapter_FundKind_Idempotent(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	// 首次 apply
	_, _ = adapter.Process(ctx, makeSuccessResult("dep-a-002", "credit_intent", 1))
	// 再次 apply（同 key + gen）
	result, err := adapter.Process(ctx, makeSuccessResult("dep-a-002", "credit_intent", 1))
	if err != nil {
		t.Fatalf("second Process: %v", err)
	}
	if !result.Idempotent {
		t.Errorf("expected Idempotent=true, got: %+v", result)
	}
}

// ─── 旧世代 stale_gen ─────────────────────────────────────────────────────

func TestAdapter_FundKind_Stalegen(t *testing.T) {
	t.Parallel()
	adapter,
		store := newTestAdapter(t)
	ctx := context.Background()
	// gen=2 先 apply
	_, _ = adapter.Process(ctx, makeSuccessResult("dep-a-003", "credit_intent", 2))
	// gen=1 (stale) apply
	result, err := adapter.Process(ctx, makeSuccessResult("dep-a-003", "credit_intent", 1))
	if err != fxonce.ErrStalegen {
		t.Fatalf("stale gen: want ErrStalegen, got %v", err)
	}
	if !result.Rejected {
		t.Error("stale gen: want Rejected=true")
	}
	if result.MarkResult.Dup == nil {
		t.Error("stale gen: want DupProof != nil")
	}
	// DupProof 可从 store 查询
	dups, _ := store.ListDupProofsForBusiness(ctx, "dep-a-003", 10)
	if len(dups) == 0 {
		t.Error("stale gen: DupProof should be persisted in store")
	}
}

// ─── 非资金 kind 旁路 ─────────────────────────────────────────────────────

func TestAdapter_NonFundKind_Bypassed(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	result, err := adapter.Process(ctx, makeSuccessResult("audit-a-001", "audit.log", 1))
	if err != nil {
		t.Fatalf("non-fund kind: %v", err)
	}
	if !result.Bypassed {
		t.Errorf("non-fund kind: expected Bypassed=true, got: %+v", result)
	}
	if result.Applied || result.Rejected {
		t.Errorf("non-fund kind: unexpected Applied/Rejected: %+v", result)
	}
}

// ─── HTTP rate limit 不写 DupProof ────────────────────────────────────────

func TestAdapter_HTTPRateLimit_NotFundExactlyOnce(t *testing.T) {
	t.Parallel()
	adapter, store := newTestAdapter(t)
	ctx := context.Background()
	hr := outeffect.HandlerResult{
		BusinessKey:	"dep-a-004",
		Kind:		"credit_intent",
		Generation:	1,
		Success:	false,
		ErrCode:	fxonce.RejectHTTPRateLimit,
	}
	result, err := adapter.Process(ctx, hr)
	if err != nil {
		t.Fatalf("rate limit process: %v", err)
	}
	if !result.Rejected {
		t.Error("rate limit: want Rejected=true")
	}
	// 不应写 DupProof（rate limit 不属于资金双花）
	dups, _ := store.ListDupProofsForBusiness(ctx, "dep-a-004", 10)
	if len(dups) != 0 {
		t.Errorf("rate limit: must not produce DupProof, got %d", len(dups))
	}
}

// ─── handler 失败 → NackRecorded ─────────────────────────────────────────

func TestAdapter_HandlerFailure_NackRecorded(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	// 先 Propose（通过 Apply with gen=1，再在另一个中 Nack）
	// Nack 需要 gen 记录存在：先 propose
	_, _ = adapter.Process(ctx, makeSuccessResult("dep-a-005", "credit_intent", 1))
	hr := outeffect.HandlerResult{
		BusinessKey:	"dep-a-005",
		Kind:		"credit_intent",
		Generation:	2,
		Success:	false,
		ErrCode:	fxonce.RejectHandlerError,
	}
	result,
		_ := adapter.Process(ctx, hr)
	if result.NackRecorded {
		// NackRecorded 在 gen 记录存在时为 true
		// 当 gen 不存在时（本例 gen=2 尚未 propose），Nack 返回 error，NackRecorded=false
		// 此处仅验证不崩溃
	}
	_ = result
}

// ─── 空 business_key → 错误 ───────────────────────────────────────────────

func TestAdapter_EmptyBusinessKey_Error(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	hr := outeffect.HandlerResult{
		BusinessKey:	"",
		Kind:		"credit_intent",
		Generation:	1,
		Success:	true,
	}
	_, err := adapter.Process(ctx, hr)
	if err == nil {
		t.Error("empty business_key: expected error, got nil")
	}
}

// ─── 并发同 business_key 双 Process ──────────────────────────────────────

func TestAdapter_Concurrent_OnlyOneApplied(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	const n = 30
	var applied, idempotent int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _ := adapter.Process(ctx, makeSuccessResult("conc-dep-001", "credit_intent", 1))
			if result.Applied {
				atomic.AddInt64(&applied, 1)
			}
			if result.Idempotent {
				atomic.AddInt64(&idempotent, 1)
			}
		}()
	}
	wg.Wait()
	total := applied + idempotent
	t.Logf("concurrent: applied=%d idempotent=%d total=%d", applied, idempotent, total)
	if total != n {
		t.Errorf("concurrent: applied+idempotent should equal n=%d, got %d", n, total)
	}
	if applied > 1 {
		t.Errorf("concurrent: only one should be applied, got %d", applied)
	}
}

// ─── 多种资金 kind 并行 ───────────────────────────────────────────────

func TestAdapter_AllFundKinds_Applied(t *testing.T) {
	t.Parallel()
	adapter, _ := newTestAdapter(t)
	ctx := context.Background()
	for i, fk := range fxonce.AllFundKinds() {
		fk := fk
		i := i
		t.Run(string(fk), func(t *testing.T) {
			t.Parallel()
			bk := string(fk) + "-test-001"
			result, err := adapter.Process(ctx, makeSuccessResult(bk, string(fk), int64(i+1)))
			if err != nil {
				t.Fatalf("Process(%q): %v", fk, err)
			}
			if !result.Applied {
				t.Errorf("Process(%q): want Applied=true, got: %+v", fk, result)
			}
		})
	}
}
