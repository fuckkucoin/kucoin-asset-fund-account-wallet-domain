package feequotelease_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feequotelease"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────────────────────
// testReader 用 FeeScheduleReader（可注入 schedule 快照）。

type testReader struct {
	schedules map[string]feequotelease.ScheduleSnapshot
}

func newTestReader(snaps ...feequotelease.ScheduleSnapshot,) *testReader {
	r := &testReader{schedules: map[string]feequotelease.ScheduleSnapshot{}}
	for _, s := range snaps {
		key := s.Kind + "|" + s.Asset
		r.schedules[key] = s
		// wildcard fallback
		if s.Asset != "*" {
			wk := s.Kind + "|*"
			if _, ok := r.schedules[wk]; !ok {
				r.schedules[wk] = feequotelease.ScheduleSnapshot{
					ID:	s.ID + "-wc", Kind: s.Kind, Asset: "*",
					BPS:	s.BPS, Version: s.Version,
				}
			}
		}
	}
	return r
}

func (r *testReader) GetSchedule(_ context.Context, kind, asset string) (feequotelease.ScheduleSnapshot, error) {
	if s, ok := r.schedules[kind+"|"+asset]; ok {
		return s, nil
	}
	if s, ok := r.schedules[kind+"|*"]; ok {
		return s, nil
	}
	return feequotelease.ScheduleSnapshot{}, feequotelease.ErrScheduleNotFound
}

func (r *testReader) putSchedule(snap feequotelease.ScheduleSnapshot,) {
	r.schedules[snap.Kind+"|"+snap.Asset] = snap
}

// newTestService 创建用 Service（内存存储 + 可控时钟）。

func newTestService(t *testing.T, snaps ...feequotelease.ScheduleSnapshot,) (*feequotelease.Service, *testReader) {
	t.Helper()
	if len(snaps) == 0 {
		snaps = []feequotelease.ScheduleSnapshot{
			{ID: "sch-withdraw-usdt-1", Kind: "withdraw", Asset: "USDT", BPS: 10, Version: 1},
			{ID: "sch-withdraw-btc-1", Kind: "withdraw", Asset: "BTC", BPS: 5, Version: 1},
			{ID: "sch-withdraw-eth-1", Kind: "withdraw", Asset: "ETH", BPS: 10, Version: 1},
			{ID: "sch-deposit-1", Kind: "deposit", Asset: "*", BPS: 0, Version: 1},
		}
	}
	reader := newTestReader(snaps...)
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	seq := int64(0)
	svc := feequotelease.New(reader).WithClock(func() time.Time {
		seq++
		return now.Add(time.Duration(seq) * time.Millisecond)
	})
	return svc, reader
}

// advancedSvc 返回一个时间快进后的 Service（用于 TTL ）。
func advancedSvc(svc *feequotelease.Service, delta time.Duration) *feequotelease.Service {
	base := svc.clock()
	svc.clock = func() time.Time { return base.Add(delta) }
	return svc
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. QuoteLease 表驱动
// ─────────────────────────────────────────────────────────────────────────────

func TestQuoteLease(t *testing.T) {
	type tc struct {
		name		string
		req		feequotelease.QuoteLeaseReq
		tiers		[]feequotelease.Tier
		wantErr		error
		wantBPS		int
		wantState	feequotelease.LeaseState
	}
	cases := []tc{
		{
			name:		"正常 BPS 报价",
			req:		feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "1000.00", TTLSeconds: 300},
			wantBPS:	10,
			wantState:	feequotelease.LeaseStateQuoted,
		},
		{
			name:		"分档报价（volume 选档）",
			req:		feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "ETH", Gross: "500.00", Volume: "50000", TTLSeconds: 300},
			tiers:		[]feequotelease.Tier{{Notional: "0", BPS: 10}, {Notional: "10000", BPS: 8}, {Notional: "100000", BPS: 5}},
			wantBPS:	8,
			wantState:	feequotelease.LeaseStateQuoted,
		},
		{
			name:		"零费率（deposit）",
			req:		feequotelease.QuoteLeaseReq{Kind: "deposit", Asset: "USDT", Gross: "200.00", TTLSeconds: 60},
			wantBPS:	0,
			wantState:	feequotelease.LeaseStateQuoted,
		},
		{
			name:		"缺 kind 拒绝",
			req:		feequotelease.QuoteLeaseReq{Asset: "USDT", Gross: "100.00"},
			wantErr:	feequotelease.ErrInvalidReq,
		},
		{
			name:		"缺 asset 拒绝",
			req:		feequotelease.QuoteLeaseReq{Kind: "withdraw", Gross: "100.00"},
			wantErr:	feequotelease.ErrInvalidReq,
		},
		{
			name:		"缺 gross 拒绝",
			req:		feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT"},
			wantErr:	feequotelease.ErrInvalidReq,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := newTestService(t)
			lease, err := svc.QuoteLease(context.Background(), c.req, c.tiers)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("want err %v, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if lease.State != c.wantState {
				t.Errorf("state: want %s, got %s", c.wantState, lease.State)
			}
			if lease.QuotedBPS != c.wantBPS {
				t.Errorf("bps: want %d, got %d", c.wantBPS, lease.QuotedBPS)
			}
			if lease.ID == "" {
				t.Error("lease.ID must not be empty")
			}
			if lease.SchGen <= 0 {
				t.Error("lease.SchGen must be positive")
			}
		})
	}
}

// 幂等：同一 idempotency_key 重复 QuoteLease 返回相同租约。

func TestQuoteLeaseIdempotent(t *testing.T) {
	svc, _ := newTestService(t)
	req := feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "1000.00", TTLSeconds: 300, IdempotencyKey: "idem-quote-001"}
	l1, err := svc.QuoteLease(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := svc.QuoteLease(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l1.ID != l2.ID {
		t.Errorf("idempotent: want same ID, got %s vs %s", l1.ID, l2.ID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. ApplyBound 表驱动
// ─────────────────────────────────────────────────────────────────────────────

func TestApplyBound(t *testing.T) {
	type tc struct {
		name		string
		setupLease	bool
		leaseID		string
		modLease	func(*feequotelease.Service, string)
		tiers		[]feequotelease.Tier
		wantErr		error
		wantState	feequotelease.LeaseState
		wantIdem	bool
	}
	cases := []tc{
		{
			name:		"正常消费",
			setupLease:	true,
			wantErr:	nil,
			wantState:	feequotelease.LeaseStateApplied,
		},
		{
			name:		"fail-closed：无 lease_id",
			leaseID:	"",
			wantErr:	feequotelease.ErrNoLease,
		},
		{
			name:		"租约不存在",
			leaseID:	"nonexistent-lease-id",
			wantErr:	feequotelease.ErrLeaseNotFound,
		},
		{
			name:		"已消费：双扣拒绝",
			setupLease:	true,
			modLease: func(svc *feequotelease.Service, id string) {
				// 先消费一次
				_, _ = svc.ApplyBound(context.Background(), feequotelease.ApplyBoundReq{LeaseID: id}, nil)
			},
			wantErr:	feequotelease.ErrLeaseConsumed,
		},
		{
			name:		"已吊销：拒绝",
			setupLease:	true,
			modLease: func(svc *feequotelease.Service, id string) {
				l, _ := svc.store.GetLease(context.Background(), id)
				t := time.Now()
				l.State = feequotelease.LeaseStateRevoked
				l.RevokedAt = &t
				_ = svc.store.SaveLease(context.Background(),
					l)
			},
			wantErr:	feequotelease.ErrLeaseRevoked,
		},
		{
			name:		"已过期：拒绝",
			setupLease:	true,
			modLease: func(svc *feequotelease.Service, id string) {
				l, _ := svc.store.GetLease(context.Background(), id)
				t := time.Now().Add(-time.Second)
				l.State = feequotelease.LeaseStateExpired
				l.ExpiredAt = &t
				_ = svc.store.SaveLease(context.Background(), l)
			},
			wantErr:	feequotelease.ErrLeaseExpired,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := newTestService(t)
			var leaseID string
			if c.setupLease {
				l, err := svc.QuoteLease(context.Background(), feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "1000.00", TTLSeconds: 300}, nil)
				if err != nil {
					t.Fatal("setup QuoteLease:", err)
				}
				leaseID = l.ID
			} else if c.leaseID != "" {
				leaseID = c.leaseID
			}
			if c.modLease != nil {
				c.modLease(svc, leaseID)
			}
			req := feequotelease.ApplyBoundReq{LeaseID: leaseID}
			result, err := svc.ApplyBound(context.Background(), req, c.tiers)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("want err %v, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if result.Lease.State != c.wantState {
				t.Errorf("state: want %s, got %s", c.wantState, result.Lease.State)
			}
			if result.Idempotent != c.wantIdem {
				t.Errorf("idempotent: want %v, got %v", c.wantIdem, result.Idempotent)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. AC1: Quote 后 Put 降 BPS → Apply 失败 + FeeQuoteDriftProof
// ─────────────────────────────────────────────────────────────────────────────

func TestAC1_QuoteThenPutLowerBPS_ApplyFail(t *testing.T) {
	svc, reader := newTestService(t)
	ctx := context.Background()
	// Step 1: QuoteLease（BPS=10）
	lease, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "1000.00", TTLSeconds: 300}, nil)
	if err != nil {
		t.Fatalf("QuoteLease: %v", err)
	}
	if lease.State != feequotelease.LeaseStateQuoted {
		t.Fatalf("want quoted, got %s", lease.State)
	}
	// Step 2: PutSchedule 降低 BPS（更新 reader 快照， feeeng.Book.Put）
	reader.putSchedule(feequotelease.ScheduleSnapshot{
		ID:	"sch-withdraw-usdt-2", Kind: "withdraw", Asset: "USDT", BPS: 5, Version: 2,
	})
	gen, revoked, err := svc.PutSchedule(ctx, feequotelease.SchedulePutReq{
		Kind:	"withdraw", Asset: "USDT", BPS: 5, Active: true, PutBy: "ops",
	})
	if err != nil {
		t.Fatalf("PutSchedule: %v", err)
	}
	if gen.Gen <= lease.SchGen {
		t.Errorf("PutSchedule should bump gen: lease.SchGen=%d new gen=%d", lease.SchGen, gen.Gen)
	}
	if revoked != 1 {
		t.Errorf("want 1 revoked lease, got %d", revoked)
	}
	// Step 3: ApplyBound 用旧 lease → 必须失败（lease 已被吊销）
	_, err = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID}, nil)
	if !errors.Is(err, feequotelease.ErrLeaseRevoked) {
		t.Errorf("want ErrLeaseRevoked, got %v", err)
	}
	// 验证漂移证明已落仓
	proofs, err := svc.store.ListProofs(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) == 0 {
		t.Error("want at least one drift proof, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. AC2: 无 lease Apply → fail-closed
// ─────────────────────────────────────────────────────────────────────────────

func TestAC2_NoLease_FailClosed(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: ""}, nil)
	if !errors.Is(err, feequotelease.ErrNoLease) {
		t.Errorf("want ErrNoLease, got %v", err)
	}
	// 验证 DriftNoLease 证明已落仓
	proofs, _ := svc.store.ListProofs(ctx, 10)
	found := false
	for _, p := range proofs {
		if p.Kind == feequotelease.DriftNoLease {
			found = true
		}
	}
	if !found {
		t.Error("want DriftNoLease proof, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. AC3: 合法 lease Apply 后重复 Apply → 幂等不双扣
// ─────────────────────────────────────────────────────────────────────────────

func TestAC3_IdempotentApply_NoDoubleDebit(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	lease, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "1000.00", TTLSeconds: 300, IdempotencyKey: "quote-001"}, nil)
	if err != nil {
		t.Fatalf("QuoteLease: %v", err)
	}
	// 第一次 Apply：成功
	r1, err := svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID, IdempotencyKey: "apply-001"}, nil)
	if err != nil {
		t.Fatalf("first ApplyBound: %v", err)
	}
	if r1.Lease.State != feequotelease.LeaseStateApplied {
		t.Errorf("want applied, got %s", r1.Lease.State)
	}
	if r1.Idempotent {
		t.Error("first apply must not be idempotent")
	}
	// 第二次 Apply（相同 lease_id）：必须拒绝，落 DriftLeaseConsumed 证明
	_, err = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID, IdempotencyKey: "apply-002"}, nil)
	if !errors.Is(err, feequotelease.ErrLeaseConsumed) {
		t.Errorf("want ErrLeaseConsumed, got %v", err)
	}
	proofs, _ := svc.store.ListProofs(ctx, 10)
	found := false
	for _, p := range proofs {
		if p.Kind == feequotelease.DriftLeaseConsumed {
			found = true
		}
	}
	if !found {
		t.Error("want DriftLeaseConsumed proof, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. sch_gen 漂移拒绝
// ─────────────────────────────────────────────────────────────────────────────

func TestSchGenDriftReject(t *testing.T) {
	svc, reader := newTestService(t)
	ctx := context.Background()
	lease, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "BTC", Gross: "1.00", TTLSeconds: 300}, nil)
	if err != nil {
		t.Fatalf("QuoteLease: %v", err)
	}
	// 直接抬升 sch_gen（绕过 PutSchedule，竞态场景）
	reader.putSchedule(feequotelease.ScheduleSnapshot{
		ID:	"sch-withdraw-btc-9", Kind: "withdraw", Asset: "BTC", BPS: 3, Version: 9,
	})
	newGen := feequotelease.ScheduleGeneration{
		ID:	"gen-btc-9", Kind: "withdraw", Asset: "BTC", Gen: 9,
		ScheduleID:	"sch-withdraw-btc-9", BPS: 3,
		PutAt:	time.Now(),
	}
	_ = svc.store.SaveGen(ctx, newGen)
	_, err = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID}, nil)
	if !errors.Is(err, feequotelease.ErrSchGenDrift) {
		t.Errorf("want ErrSchGenDrift, got %v", err)
	}
	proofs,
		_ := svc.store.ListProofs(ctx, 10)
	found := false
	for _, p := range proofs {
		if p.Kind == feequotelease.DriftSchGenAdvanced {
			found = true
		}
	}
	if !found {
		t.Error("want DriftSchGenAdvanced proof, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. tiers 指纹篡改拒绝
// ─────────────────────────────────────────────────────────────────────────────

func TestFingerprintTamperReject(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	originalTiers := []feequotelease.Tier{
		{Notional: "0", BPS: 10},
		{Notional: "10000", BPS: 8},
		{Notional: "100000", BPS: 5},
	}
	lease, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "ETH", Gross: "100.00", Volume: "50000", TTLSeconds: 300}, originalTiers)
	if err != nil {
		t.Fatalf("QuoteLease: %v", err)
	}
	if lease.TiersFingerprint == "" {
		t.Error("tiers fingerprint must not be empty for tiered quote")
	}
	tamperedTiers := []feequotelease.Tier{
		{Notional: "0", BPS: 5},
		{Notional: "10000", BPS: 3},
	}
	_, err = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID}, tamperedTiers)
	if !errors.Is(err, feequotelease.ErrFingerprintMismatch,
	) {
		t.Errorf("want ErrFingerprintMismatch, got %v", err)
	}
	proofs, _ := svc.store.ListProofs(ctx, 10)
	found := false
	for _, p := range proofs {
		if p.Kind == feequotelease.DriftFingerprintMismatch {
			found = true
		}
	}
	if !found {
		t.Error("want DriftFingerprintMismatch proof, got none")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. TTL 过期拒绝
// ─────────────────────────────────────────────────────────────────────────────

func TestTTLExpiredReject(t *testing.T) {
	// 使用固定基准时钟
	base := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	seq := int64(0)
	svc := feequotelease.New(newTestReader(feequotelease.ScheduleSnapshot{ID: "sch-wd-usdt", Kind: "withdraw", Asset: "USDT", BPS: 10, Version: 1}, feequotelease.ScheduleSnapshot{ID: "sch-wd-all", Kind: "withdraw", Asset: "*", BPS: 10, Version: 1},
	)).WithClock(func() time.Time {
		seq++
		return base.Add(time.Duration(seq) * time.Millisecond)
	})
	ctx := context.Background()
	// 签发短 TTL 租约（1 秒）
	lease, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "200.00", TTLSeconds: 1}, nil)
	if err != nil {
		t.Fatalf("QuoteLease: %v", err)
	}
	// 快进时钟：超过 lease.ExpiresAt
	svc.clock = func() time.Time { return lease.ExpiresAt.Add(5 * time.Second) }
	_,
		err = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: lease.ID}, nil)
	if !errors.Is(err, feequotelease.ErrLeaseExpired) {
		t.Errorf("want ErrLeaseExpired, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 9. PutSchedule 世代抬升与吊销
// ─────────────────────────────────────────────────────────────────────────────

func TestPutSchedule_GenBumpAndRevoke(t *testing.T) {
	svc, reader := newTestService(t)
	ctx := context.Background()
	// 签发 3 条租约（相同 kind/asset）
	for i := 0; i < 3; i++ {
		_, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "100.00", TTLSeconds: 300}, nil)
		if err != nil {
			t.Fatalf("QuoteLease[%d]: %v", i, err)
		}
	}
	// 抬升 sch_gen
	reader.putSchedule(feequotelease.ScheduleSnapshot{
		ID:	"sch-withdraw-usdt-99", Kind: "withdraw", Asset: "USDT", BPS: 3, Version: 99,
	})
	gen, revoked, err := svc.PutSchedule(ctx, feequotelease.SchedulePutReq{
		Kind:	"withdraw", Asset: "USDT", BPS: 3, Active: true, PutBy: "test",
	})
	if err != nil {
		t.Fatalf("PutSchedule: %v", err)
	}
	if revoked != 3 {
		t.Errorf("want 3 revoked leases, got %d", revoked)
	}
	if gen.Gen <= 0 {
		t.Error("gen.Gen must be positive")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 10. ExpireLeases Worker
// ─────────────────────────────────────────────────────────────────────────────

func TestExpireLeases_Worker(t *testing.T) {
	// 固定基准时钟
	base := time.Date(2026,
		9, 22, 10, 0, 0, 0, time.UTC)
	seq := int64(0)
	svc := feequotelease.New(newTestReader(feequotelease.ScheduleSnapshot{ID: "sch-wd-usdt-exp", Kind: "withdraw", Asset: "USDT", BPS: 10, Version: 1}, feequotelease.ScheduleSnapshot{ID: "sch-wd-all-exp", Kind: "withdraw", Asset: "*", BPS: 10, Version: 1},
	)).WithClock(func() time.Time {
		seq++
		return base.Add(time.Duration(seq) * time.Millisecond)
	})
	ctx := context.Background()
	// 签发短 TTL 租约（1 秒）
	var lastExpires time.Time
	for i := 0; i < 5; i++ {
		l, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "100.00", TTLSeconds: 1}, nil)
		if err != nil {
			t.Fatalf("QuoteLease[%d]: %v", i, err)
		}
		if l.ExpiresAt.After(lastExpires) {
			lastExpires = l.ExpiresAt
		}
	}
	// 快进时钟：超过所有租约的 ExpiresAt
	svc.clock = func() time.Time { return lastExpires.Add(10 * time.Second) }
	w := feequotelease.NewExpireWorker(svc, time.Second)
	n, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	if n != 5 {
		t.Errorf("want 5 expired, got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 11. RevokeByKindAsset Worker
// ─────────────────────────────────────────────────────────────────────────────

func TestRevokeByKindAsset(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "100.00", TTLSeconds: 300}, nil)
		if err != nil {
			t.Fatalf("QuoteLease[%d]: %v", i, err)
		}
	}
	// 额外签发不同 asset（不应被吊销）
	_, _ = svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "BTC", Gross: "1.00", TTLSeconds: 300}, nil)
	n, err := svc.RevokeByKindAsset(ctx, "withdraw", "USDT", "test_revoke")
	if err != nil {
		t.Fatalf("RevokeByKindAsset: %v", err)
	}
	if n != 4 {
		t.Errorf("want 4 revoked, got %d", n)
	}
	// BTC 租约不受影响
	btcLeases, _ := svc.store.ListActiveLeasesByKindAsset(ctx, "withdraw", "BTC")
	if len(btcLeases) != 1 {
		t.Errorf("BTC lease should remain quoted, got %d active", len(btcLeases))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 12. ExportProofs
// ─────────────────────────────────────────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// 触发 NoLease 证明
	for i := 0; i < 3; i++ {
		_, _ = svc.ApplyBound(ctx, feequotelease.ApplyBoundReq{LeaseID: ""}, nil)
	}
	w := feequotelease.NewProofExportWorker(svc, time.Second, feequotelease.NoopSink{})
	proofs, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("ExportProofs: %v", err)
	}
	if len(proofs) != 3 {
		t.Errorf("want 3 exported proofs, got %d", len(proofs))
	}
	// 再次导出应为空（已标记）
	proofs2, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("ExportProofs2: %v", err)
	}
	if len(proofs2) != 0 {
		t.Errorf("want 0 remaining proofs, got %d", len(proofs2))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 13. TiersFingerprint 确定性
// ─────────────────────────────────────────────────────────────────────────────

func TestTiersFingerprint(t *testing.T) {
	tiers := []feequotelease.Tier{
		{Notional: "10000", BPS: 8},
		{Notional: "0", BPS: 10},
		{Notional: "100000", BPS: 5},
	}
	fp1 := feequotelease.TiersFingerprint(tiers)
	// 不同顺序应得到相同结果（规范化排序）
	tiers2 := []feequotelease.Tier{
		{Notional: "0", BPS: 10},
		{Notional: "100000", BPS: 5},
		{Notional: "10000", BPS: 8},
	}
	fp2 := feequotelease.TiersFingerprint(tiers2)
	if fp1 != fp2 {
		t.Errorf("fingerprint must be order-independent: %s vs %s", fp1, fp2)
	}
	// 空 tiers 返回空串
	if feequotelease.TiersFingerprint(nil) != "" {
		t.Error("empty tiers must return empty fingerprint")
	}
	// 不同 tiers 返回不同指纹
	tiers3 := []feequotelease.Tier{{Notional: "0", BPS: 5}}
	fp3 := feequotelease.TiersFingerprint(tiers3)
	if fp1 == fp3 {
		t.Error("different tiers must produce different fingerprint")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 14. DefaultTTL 与 MaxTTL 限制
// ─────────────────────────────────────────────────────────────────────────────

func TestQuoteLease_TTLBounds(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// TTL=0 → 使用默认 300s
	l1, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "100.00", TTLSeconds: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l1.ExpiresAt.IsZero() {
		t.Error("expires_at must not be zero")
	}
	// TTL 超 MaxTTLSeconds → 钳制为 MaxTTLSeconds
	l2, err := svc.QuoteLease(ctx, feequotelease.QuoteLeaseReq{Kind: "withdraw", Asset: "USDT", Gross: "100.00", TTLSeconds: 99999}, nil)
	if err != nil {
		t.Fatal(err)
	}
	diff := l2.ExpiresAt.Sub(l2.QuotedAt)
	if diff > time.Duration(feequotelease.MaxTTLSeconds)*time.Second+time.Second {
		t.Errorf("TTL should be capped at MaxTTLSeconds, got %v", diff)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 15. PutSchedule 参数校验
// ─────────────────────────────────────────────────────────────────────────────

func TestPutSchedule_Validation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	cases := []struct {
		name	string
		req	feequotelease.SchedulePutReq
		wantErr	error
	}{
		{
			name:		"缺 kind",
			req:		feequotelease.SchedulePutReq{Asset: "USDT", BPS: 10},
			wantErr:	feequotelease.ErrInvalidReq,
		},
		{
			name:		"BPS 超范围",
			req:		feequotelease.SchedulePutReq{Kind: "withdraw", Asset: "USDT", BPS: 20000},
			wantErr:	feequotelease.ErrInvalidReq,
		},
		{
			name:		"BPS 负数",
			req:		feequotelease.SchedulePutReq{Kind: "withdraw", Asset: "USDT", BPS: -1},
			wantErr:	feequotelease.ErrInvalidReq,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := svc.PutSchedule(ctx, c.req)
			if !errors.Is(err, c.wantErr) {
				t.Errorf("want %v, got %v", c.wantErr, err)
			}
		})
	}
}
