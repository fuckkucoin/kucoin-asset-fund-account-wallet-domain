package feepathgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

func buildLease(id, bizKey string, kind feepathgate.Kind, fee, ledgerID string, status feepathgate.LeaseStatus, proofID string) *feepathgate.PathLease {
	return &feepathgate.PathLease{
		ID:		id,
		BusinessKey:	bizKey,
		Kind:		kind,
		Asset:		"USDT",
		Gross:		"100.00",
		Fee:		fee,
		Net:		"95.00",
		CreditID:	"credit-" + id,
		LedgerID:	ledgerID,
		ProofID:	proofID,
		Status:		status,
		CreatedAt:	time.Now().UTC(),
		UpdatedAt:	time.Now().UTC(),
	}
}

func newTestDetector() *feepathgate.DualHitDetector {
	store := feepathgate.NewMemStore()
	return feepathgate.NewDetector(store, func() time.Time { return time.Date(2025, 3, 21, 0, 0, 0, 0, time.UTC) })
}

func TestDualHitDetector_Scan(t *testing.T) {
	type tc struct {
		name		string
		take		*feepathgate.PathLease
		rebate		*feepathgate.PathLease
		wantHit		bool
		wantSeverity	string
	}
	tests := []tc{
		{
			name:		"both_bound_with_ledger_high",
			take:		buildLease("t1", "u1|USDT|i1", feepathgate.KindTake, "5.00", "ldg-t1", feepathgate.LeaseBound, ""),
			rebate:		buildLease("r1", "u1|USDT|i1", feepathgate.KindRebate, "5.00", "ldg-r1", feepathgate.LeaseBound, ""),
			wantHit:	true,
			wantSeverity:	"high",
		},
		{
			name:		"rebate_no_ledger_no_hit",
			take:		buildLease("t2", "u2|USDT|i2", feepathgate.KindTake, "5.00", "ldg-t2", feepathgate.LeaseBound, ""),
			rebate:		buildLease("r2", "u2|USDT|i2", feepathgate.KindRebate, "5.00", "", feepathgate.LeaseBound, ""),
			wantHit:	false,
			wantSeverity:	"",
		},
		{
			name:		"take_open_no_hit",
			take:		buildLease("t3", "u3|USDT|i3", feepathgate.KindTake, "5.00", "ldg-t3", feepathgate.LeaseOpen, ""),
			rebate:		buildLease("r3", "u3|USDT|i3", feepathgate.KindRebate, "5.00", "ldg-r3", feepathgate.LeaseBound, ""),
			wantHit:	false,
			wantSeverity:	"",
		},
		{
			name:		"tiny_fee_low_severity",
			take:		buildLease("t4", "u4|USDT|i4", feepathgate.KindTake, "0.0000001", "ldg-t4", feepathgate.LeaseBound, ""),
			rebate:		buildLease("r4", "u4|USDT|i4", feepathgate.KindRebate, "0.0000001", "ldg-r4", feepathgate.LeaseBound, ""),
			wantHit:	true,
			wantSeverity:	"low",
		},
		{
			name:		"partial_proof_medium_severity",
			take:		buildLease("t5", "u5|USDT|i5", feepathgate.KindTake, "5.00", "ldg-t5", feepathgate.LeaseBound, "proof-existing"),
			rebate:		buildLease("r5", "u5|USDT|i5", feepathgate.KindRebate, "5.00", "ldg-r5", LaseBound(), ""),
			wantHit:	true,
			wantSeverity:	"medium",
		},
		{
			name:		"nil_take_no_hit",
			take:		nil,
			rebate:		buildLease("r6", "u6|USDT|i6", feepathgate.KindRebate, "5.00", "ldg-r6", feepathgate.LeaseBound, ""),
			wantHit:	false,
			wantSeverity:	"",
		},
		{
			name:		"nil_rebate_no_hit",
			take:		buildLease("t7", "u7|USDT|i7", feepathgate.KindTake, "5.00", "ldg-t7", feepathgate.LeaseBound, ""),
			rebate:		nil,
			wantHit:	false,
			wantSeverity:	"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDetector()
			ctx := context.Background()
			hit, err := d.Scan(ctx, tt.take, tt.rebate)
			if err != nil {
				t.Fatalf("scan error: %v", err)
			}
			if tt.wantHit && hit == nil {
				t.Errorf("want DualHit, got nil")
				return
			}
			if !tt.wantHit && hit != nil {
				t.Errorf("want no DualHit, got %+v", hit)
				return
			}
			if tt.wantHit && hit.Severity != tt.wantSeverity {
				t.Errorf("want severity=%s, got %s", tt.wantSeverity, hit.Severity)
			}
			if tt.wantHit && hit.Status != feepathgate.DualHitOpen {
				t.Errorf("want status=open, got %s", hit.Status)
			}
		})
	}
}

// LaseBound は helper，避免字段名复用 LeaseBound（Go 不允许同名变量与常量）。
func LaseBound() feepathgate.LeaseStatus	{ return feepathgate.LeaseBound }

// TestDualHitDetector_Idempotent 同一 biz_key 重复 Scan 返回同一条记录。

func TestDualHitDetector_Idempotent(t *testing.T) {
	d := newTestDetector()
	ctx := context.Background()
	bizKey := "u30|USDT|idem-scan"
	take := buildLease("t-idem", bizKey, feepathgate.KindTake, "5.00", "ldg-take", feepathgate.LeaseBound, "")
	rebate := buildLease("r-idem", bizKey, feepathgate.KindRebate, "5.00", "ldg-rebate", feepathgate.LeaseBound, "")
	hit1,
		err1 := d.Scan(ctx, take, rebate)
	if err1 != nil || hit1 == nil {
		t.Fatalf("first scan: hit=%v err=%v", hit1, err1)
	}
	hit2, err2 := d.Scan(ctx, take, rebate)
	if err2 != nil || hit2 == nil {
		t.Fatalf("second scan: hit=%v err=%v", hit2, err2)
	}
	if hit1.ID != hit2.ID {
		t.Errorf("idempotent scan should return same hit_id; got %s vs %s", hit1.ID, hit2.ID)
	}
}

// TestDualHitDetector_MarkProven DualHit 推进至 proven 状态。

func TestDualHitDetector_MarkProven(t *testing.T) {
	d := newTestDetector()
	ctx := context.Background()
	bizKey := "u40|USDT|proven"
	take := buildLease("t-p", bizKey, feepathgate.KindTake, "5.00", "ldg-t-p", feepathgate.LeaseBound, "")
	rebate := buildLease("r-p", bizKey, feepathgate.KindRebate, "5.00", "ldg-r-p", feepathgate.LeaseBound, "")
	hit, _ := d.Scan(ctx, take, rebate)
	proved, err := d.MarkProven(ctx, hit.ID, "proof-xyz")
	if err != nil {
		t.Fatalf("mark_proven: %v", err)
	}
	if proved.Status != feepathgate.DualHitProven {
		t.Errorf("want proven, got %s", proved.Status)
	}
	if proved.ProofID != "proof-xyz" {
		t.Errorf("want proof_id=proof-xyz, got %s", proved.ProofID)
	}
}

// TestDualHitDetector_Resolve 人工确认关闭 DualHit，不再出现于 ListOpen。
func TestDualHitDetector_Resolve(t *testing.T) {
	d := newTestDetector()
	ctx := context.Background()
	bizKey := "u50|USDT|resolve"
	take := buildLease("t-res", bizKey, feepathgate.KindTake, "5.00", "ldg-t-res", feepathgate.LeaseBound, "")
	rebate := buildLease("r-res", bizKey, feepathgate.KindRebate, "5.00", "ldg-r-res", feepathgate.LeaseBound, "")
	hit, _ := d.Scan(ctx, take, rebate)
	openHits, _ := d.ListOpen(ctx)
	if len(openHits) != 1 {
		t.Fatalf("want 1 open hit, got %d", len(openHits))
	}
	closed, err := d.Resolve(ctx, feepathgate.DualHitResolveReq{
		DualHitID:	hit.ID,
		ResolvedBy:	"ops-bob@kucoin.net",
		Note:		"manually verified",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if closed.Status != feepathgate.DualHitClosed {
		t.Errorf("want closed, got %s", closed.Status)
	}
	if closed.ResolvedBy != "ops-bob@kucoin.net" {
		t.Errorf("want resolved_by=ops-bob@kucoin.net, got %s", closed.ResolvedBy)
	}
	openAfter, _ := d.ListOpen(ctx)
	if len(openAfter) != 0 {
		t.Errorf("after resolve: want 0 open hits, got %d", len(openAfter))
	}
}

// TestDualHitDetector_GetByBizKey 通过 biz_key 查询活跃命中。
func TestDualHitDetector_GetByBizKey(t *testing.T) {
	d := newTestDetector()
	ctx := context.Background()
	bizKey := "u60|ETH|gbk"
	take := buildLease("t-gbk", bizKey, feepathgate.KindTake, "0.01", "ldg-t-gbk", feepathgate.LeaseBound, "")
	rebate := buildLease("r-gbk", bizKey, feepathgate.KindRebate, "0.01", "ldg-r-gbk", feepathgate.LeaseBound, "")
	hit, _ := d.Scan(ctx, take, rebate)
	got, err := d.GetByBizKey(ctx, bizKey)
	if err != nil {
		t.Fatalf("get_by_biz_key: %v", err)
	}
	if got == nil {
		t.Fatal("want hit, got nil")
	}
	if got.ID != hit.ID {
		t.Errorf("want hit_id=%s, got %s", hit.ID, got.ID)
	}
}
