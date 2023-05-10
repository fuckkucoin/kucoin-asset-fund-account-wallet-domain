package fxonce_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ─── 辅助构造 ─────────────────────────────────────────────────────────────

func makeEffect(businessKey string, gen int64) *fxonce.FundEffect {
	now := time.Now().UTC()
	ek := fxonce.MakeEffectKey(businessKey, gen)
	return &fxonce.FundEffect{
		ID:		fxonce.NewEffectID(),
		BusinessKey:	businessKey,
		Kind:		fxonce.KindCreditIntent,
		Generation:	gen,
		EffectKey:	ek,
		State:		fxonce.EffectProposed,
		CreatedAt:	now,
		UpdatedAt:	now,
	}
}

func makeGen(businessKey string, gen int64, effectID string) *fxonce.DeliveryGen {
	now := time.Now().UTC()
	return &fxonce.DeliveryGen{
		ID:		fxonce.NewGenID(),
		BusinessKey:	businessKey,
		Kind:		fxonce.KindCreditIntent,
		ExpectedGen:	gen,
		State:		fxonce.GenInflight,
		MaxRetry:	5,
		EffectID:	effectID,
		CreatedAt:	now,
		UpdatedAt:	now,
	}
}

func makeDup(businessKey string, winnerGen, loserGen int64) *fxonce.DupProof {
	now := time.Now().UTC()
	return &fxonce.DupProof{
		ID:		fxonce.NewDupID(),
		BusinessKey:	businessKey,
		Kind:		fxonce.KindCreditIntent,
		WinnerGen:	winnerGen,
		LoserGen:	loserGen,
		WinnerAt:	now.Add(-time.Second),
		LoserAt:	now,
		WindowMs:	1000,
		RejectCode:	fxonce.RejectStalegen,
		CreatedAt:	now,
	}
}

// ─── EffectStore ──────────────────────────────────────────────────────────

func TestMemStore_PutGetEffect(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-s-001", 1)
	if err := store.PutEffect(ctx, e); err != nil {
		t.Fatalf("PutEffect: %v", err)
	}
	got, err := store.GetEffect(ctx, e.EffectKey)
	if err != nil {
		t.Fatalf("GetEffect: %v", err)
	}
	if got.ID != e.ID || got.BusinessKey != e.BusinessKey {
		t.Errorf("GetEffect mismatch: got %+v, want %+v", got, e)
	}
}

func TestMemStore_PutEffect_UniqueKey_Conflict(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-s-002", 1)
	_ = store.PutEffect(ctx, e)
	// 同 effect_key 再次写入 → 冲突
	e2 := makeEffect("dep-s-002", 1)
	e2.ID = fxonce.NewEffectID()	// 不同 ID，但同 effect_key
	if err := store.PutEffect(ctx, e2); err != fxonce.ErrEffectConflict {
		t.Errorf("duplicate effect_key: want ErrEffectConflict, got %v", err)
	}
}

func TestMemStore_GetEffect_NotFound(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	_, err := store.GetEffect(ctx, "nonexistent-key-00000000000000000")
	if err != fxonce.ErrEffectNotFound {
		t.Errorf("GetEffect not found: want ErrEffectNotFound, got %v", err)
	}
}

func TestMemStore_GetEffectByID_NotFound(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	_, err := store.GetEffectByID(ctx, "no-such-id")
	if err != fxonce.ErrEffectNotFound {
		t.Errorf("GetEffectByID not found: want ErrEffectNotFound, got %v", err)
	}
}

func TestMemStore_UpdateEffectState_CAS(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	e := makeEffect("dep-s-003", 1)
	_ = store.PutEffect(ctx, e)
	// 正常迁移 proposed → applied
	if err := store.UpdateEffectState(ctx, e.ID, fxonce.EffectProposed, fxonce.EffectApplied, now, ""); err != nil {
		t.Fatalf("UpdateEffectState: %v", err)
	}
	got, _ := store.GetEffectByID(ctx, e.ID)
	if got.State != fxonce.EffectApplied {
		t.Errorf("state: want applied, got %q", got.State)
	}
	// oldState 不符 → ErrEffectConflict
	if err := store.UpdateEffectState(ctx, e.ID, fxonce.EffectProposed, fxonce.EffectRejected, now, fxonce.RejectStalegen); err != fxonce.ErrEffectConflict {
		t.Errorf("wrong oldState: want ErrEffectConflict, got %v", err)
	}
}

func TestMemStore_MarkPoisoned_MarkForceReplay(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	e := makeEffect("dep-s-004", 1)
	_ = store.PutEffect(ctx, e)
	// MarkPoisoned
	_ = store.MarkPoisoned(ctx, e.ID, 5, now)
	got, _ := store.GetEffectByID(ctx, e.ID)
	if got.State != fxonce.EffectPoisoned || got.PoisonRetries != 5 {
		t.Errorf("MarkPoisoned: state=%q, retries=%d", got.State, got.PoisonRetries)
	}
	// MarkForceReplay
	_ = store.MarkForceReplay(ctx, e.ID, "alice+bob", now)
	got2, _ := store.GetEffectByID(ctx, e.ID)
	if got2.State != fxonce.EffectProposed || got2.ForceReplayBy != "alice+bob" {
		t.Errorf("MarkForceReplay: state=%q, by=%q", got2.State, got2.ForceReplayBy)
	}
}

func TestMemStore_MarkForceReplay_NotPoisoned_Error(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-s-005", 1)
	_ = store.PutEffect(ctx, e)
	// 未毒丸时调用 MarkForceReplay → 冲突
	if err := store.MarkForceReplay(ctx, e.ID, "alice+bob", time.Now().UTC()); err != fxonce.ErrEffectConflict {
		t.Errorf("ForceReplay on non-poisoned: want ErrEffectConflict, got %v", err)
	}
}

func TestMemStore_ListEffects_Pagination(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		_ = store.PutEffect(ctx, makeEffect("list-dep-001", int64(i)))
	}
	// 只拿前 3 条
	effects, err := store.ListEffects(ctx, "list-dep-001", 3, 0)
	if err != nil {
		t.Fatalf("ListEffects: %v", err)
	}
	if len(effects) != 3 {
		t.Errorf("ListEffects limit=3: got %d", len(effects))
	}
}

// ─── GenStore ─────────────────────────────────────────────────────────────

func TestMemStore_PutGetGen(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-g-001", 1)
	_ = store.PutEffect(ctx, e)
	g := makeGen("dep-g-001", 1, e.ID)
	if err := store.PutGen(ctx, g); err != nil {
		t.Fatalf("PutGen: %v", err)
	}
	got, err := store.GetGen(ctx, "dep-g-001", 1)
	if err != nil {
		t.Fatalf("GetGen: %v", err)
	}
	if got.ID != g.ID {
		t.Errorf("GetGen: id mismatch %q != %q", got.ID, g.ID)
	}
}

func TestMemStore_PutGen_UniqueConstraint(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-g-002", 1)
	_ = store.PutEffect(ctx, e)
	g := makeGen("dep-g-002", 1, e.ID)
	_ = store.PutGen(ctx, g)
	g2 := makeGen("dep-g-002", 1, e.ID)
	g2.ID = fxonce.NewGenID()
	if err := store.PutGen(ctx, g2); err != fxonce.ErrEffectConflict {
		t.Errorf("duplicate gen: want ErrEffectConflict, got %v", err)
	}
}

func TestMemStore_GetCurrentAppliedGen(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	// 初始应为 0
	gen, _ := store.GetCurrentAppliedGen(ctx, "dep-g-003")
	if gen != 0 {
		t.Errorf("initial applied gen: want 0, got %d", gen)
	}
	// 写入并 acked 一条 gen=2
	e := makeEffect("dep-g-003", 2)
	_ = store.PutEffect(ctx, e)
	g := makeGen("dep-g-003", 2, e.ID)
	_ = store.PutGen(ctx, g)
	now := time.Now().UTC()
	_ = store.UpdateGenState(ctx, g.ID, fxonce.GenAcked, 2, now, "")
	cur, _ := store.GetCurrentAppliedGen(ctx, "dep-g-003")
	if cur != 2 {
		t.Errorf("applied gen after ack: want 2, got %d", cur)
	}
}

// ─── DupProofStore ────────────────────────────────────────────────────────

func TestMemStore_PutGetDupProof(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	d := makeDup("dep-d-001", 2, 1)
	if err := store.PutDupProof(ctx, d); err != nil {
		t.Fatalf("PutDupProof: %v", err)
	}
	got, err := store.GetDupProof(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDupProof: %v", err)
	}
	if got.BusinessKey != d.BusinessKey {
		t.Errorf("GetDupProof: business_key mismatch")
	}
}

func TestMemStore_PutDupProof_IDConflict(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	d := makeDup("dep-d-002", 2, 1)
	_ = store.PutDupProof(ctx, d)
	if err := store.PutDupProof(ctx, d); err != fxonce.ErrDupProofConflict {
		t.Errorf("duplicate dup proof: want ErrDupProofConflict, got %v", err)
	}
}

func TestMemStore_ListUnreviewed(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		d := makeDup("dep-d-003", 2, 1)
		_ = store.PutDupProof(ctx, d)
	}
	dups, err := store.ListUnreviewed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnreviewed: %v", err)
	}
	if len(dups) != 5 {
		t.Errorf("ListUnreviewed: want 5, got %d", len(dups))
	}
}

func TestMemStore_MarkReviewed(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	d := makeDup("dep-d-004", 2, 1)
	_ = store.PutDupProof(ctx, d)
	_ = store.MarkReviewed(ctx, d.ID, "desk-operator", time.Now().UTC())
	got, _ := store.GetDupProof(ctx, d.ID)
	if !got.Reviewed || got.ReviewedBy != "desk-operator" {
		t.Errorf("MarkReviewed: reviewed=%v, by=%q", got.Reviewed, got.ReviewedBy)
	}
	// 已审阅的不出现在 ListUnreviewed
	remaining, _ := store.ListUnreviewed(ctx, 10)
	for _, dp := range remaining {
		if dp.ID == d.ID {
			t.Error("reviewed dup proof should not appear in ListUnreviewed")
		}
	}
}

// ─── 并发安全 ─────────────────────────────────────────────────────────

func TestMemStore_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			e := makeEffect("conc-dep", int64(i+1))
			_ = store.PutEffect(ctx, e)
		}()
	}
	wg.Wait()
	effects, _ := store.ListEffects(ctx, "conc-dep", n*2, 0)
	if len(effects) > n {
		t.Errorf("concurrent writes: expected at most %d effects, got %d", n, len(effects))
	}
}
