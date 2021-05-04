package feepathgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

func newTestVault() *feepathgate.HedgeVault {
	store := feepathgate.NewMemStore()
	return feepathgate.NewHedgeVault(store, func() time.Time { return time.Date(2025, 3, 22, 0, 0, 0, 0, time.UTC) })
}

func takeLease(id, bizKey, fee, ledgerID string) *feepathgate.PathLease {
	return &feepathgate.PathLease{
		ID:	id, BusinessKey: bizKey, Kind: feepathgate.KindTake, Asset: "USDT",
		Gross:	"100.00", Fee: fee, Net: "95.00",
		CreditID:	"c-" + id, LedgerID: ledgerID, Status: feepathgate.LeaseBound,
	}
}

func rebateLease(id, bizKey, fee, ledgerID string) *feepathgate.PathLease {
	return &feepathgate.PathLease{
		ID:	id, BusinessKey: bizKey, Kind: feepathgate.KindRebate, Asset: "USDT",
		Gross:	fee, Fee: fee, Net: "0.00",
		CreditID:	"c-" + id, LedgerID: ledgerID, Status: feepathgate.LeaseBound,
	}
}

func TestHedgeVault_Build(t *testing.T) {
	type tc struct {
		name	string
		take	*feepathgate.PathLease
		rebate	*feepathgate.PathLease
		wantErr	error
		wantOK	bool
	}
	tests := []tc{
		{
			name:		"balanced_with_ledger",
			take:		takeLease("t1", "u1|USDT|h1", "5.00", ""),
			rebate:		rebateLease("r1", "u1|USDT|h1", "5.00", "ldg-r1"),
			wantErr:	nil,
			wantOK:		true,
		},
		{
			name:		"imbalanced_fee",
			take:		takeLease("t2", "u2|USDT|h2", "5.00", ""),
			rebate:		rebateLease("r2", "u2|USDT|h2", "4.99", "ldg-r2"),
			wantErr:	feepathgate.ErrHedgeImbalance,
		},
		{
			name:		"missing_rebate_ledger",
			take:		takeLease("t3", "u3|USDT|h3", "5.00", ""),
			rebate:		rebateLease("r3", "u3|USDT|h3", "5.00", ""),
			wantErr:	feepathgate.ErrHedgeImbalance,
		},
		{
			name:		"zero_fee",
			take:		takeLease("t4", "u4|USDT|h4", "0", ""),
			rebate:		rebateLease("r4", "u4|USDT|h4", "0", "ldg-r4"),
			wantErr:	feepathgate.ErrHedgeImbalance,
		},
		{
			name:	"wrong_take_kind",
			take: &feepathgate.PathLease{ID: "t5", BusinessKey: "u5|USDT|h5", Kind: feepathgate.KindRebate,
				Asset:	"USDT", Fee: "5.00", LedgerID: "", Status: feepathgate.LeaseBound},
			rebate:		rebateLease("r5", "u5|USDT|h5", "5.00", "ldg-r5"),
			wantErr:	feepathgate.ErrHedgeImbalance,
		},
		{
			name:		"biz_key_mismatch",
			take:		takeLease("t6", "u6|USDT|h6a", "5.00", ""),
			rebate:		rebateLease("r6", "u6|USDT|h6b", "5.00", "ldg-r6"),
			wantErr:	feepathgate.ErrHedgeImbalance,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault()
			ctx := context.Background()
			proof, err := v.Build(ctx, tt.take, tt.rebate)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("want err wrapping %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proof == nil {
				t.Fatal("want proof, got nil")
			}
			if proof.Status != feepathgate.HedgePending {
				t.Errorf("want pending, got %s", proof.Status)
			}
			if len(proof.Digest) != 64 {
				t.Errorf("want 64-char digest, got len=%d: %s", len(proof.Digest), proof.Digest)
			}
			if len(proof.LedgerIDs) == 0 {
				t.Error("want at least 1 ledger_id")
			}
		})
	}
}

// TestHedgeVault_Build_Idempotent 同对重复 Build 返回相同 proof_id。
func TestHedgeVault_Build_Idempotent(t *testing.T) {
	v := newTestVault()
	ctx := context.Background()
	take := takeLease("t-idem", "u10|USDT|hi", "5.00", "")
	rebate := rebateLease("r-idem", "u10|USDT|hi", "5.00", "ldg-r-idem")
	p1, err1 := v.Build(ctx, take, rebate)
	if err1 != nil {
		t.Fatalf("first build: %v", err1)
	}
	p2, err2 := v.Build(ctx, take, rebate)
	if err2 != nil {
		t.Fatalf("second build: %v", err2)
	}
	if p1.ID != p2.ID {
		t.Errorf("idempotent: want same proof_id, got %s vs %s", p1.ID, p2.ID)
	}
}

// TestHedgeVault_Verify_OK Digest 可复现，验证通过后 status=verified。
func TestHedgeVault_Verify_OK(t *testing.T) {
	v := newTestVault()
	ctx := context.Background()
	take := takeLease("t-v", "u20|USDT|hv", "5.00", "")
	rebate := rebateLease("r-v", "u20|USDT|hv", "5.00", "ldg-r-v")
	proof, _ := v.Build(ctx, take, rebate)
	verified, err := v.Verify(ctx, proof.ID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified.Status != feepathgate.HedgeVerified {
		t.Errorf("want verified, got %s", verified.Status)
	}
	if verified.VerifiedAt == nil {
		t.Error("verified_at should be set")
	}
	// 再次 Verify 幂等。
	v2,
		err2 := v.Verify(ctx, proof.ID)
	if err2 != nil {
		t.Fatalf("second verify: %v", err2)
	}
	if v2.Status != feepathgate.HedgeVerified {
		t.Errorf("second verify: want verified, got %s", v2.Status)
	}
}

// TestHedgeVault_Verify_DigestTampered Digest 被修改后验证失败。

func TestHedgeVault_Verify_DigestTampered(t *testing.T) {
	v := newTestVault()
	ctx := context.Background()
	take := takeLease("t-tamper", "u30|USDT|ht", "5.00", "")
	rebate := rebateLease("r-tamper", "u30|USDT|ht", "5.00", "ldg-r-tamper")
	proof, _ := v.Build(ctx, take, rebate)
	// 直接篡改内存中的 Digest。
	v.mu.Lock()
	v.byID[proof.ID].Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	v.mu.Unlock()
	_, err := v.Verify(ctx, proof.ID)
	if err != feepathgate.ErrProofVerify {
		t.Errorf("want ErrProofVerify, got %v", err)
	}
}

// TestHedgeProof_ComputeDigest 确定性：相同输入产出相同 SHA-256 摘要。
func TestHedgeProof_ComputeDigest(t *testing.T) {
	p := &feepathgate.HedgeProof{
		BusinessKey:	"u40|USDT|digest-test",
		Asset:		"USDT",
		GrossTake:	"1000.00",
		FeeTake:	"5.00",
		NetTake:	"995.00",
		GrossRebate:	"5.00",
		FeeRebate:	"5.00",
		NetRebate:	"0.00",
		LedgerIDs:	[]string{"ledger-b", "ledger-a"},	// 故意乱序
	}
	d1 := p.ComputeDigest()
	d2 := p.ComputeDigest()
	if d1 != d2 {
		t.Errorf("non-deterministic digest: %s vs %s", d1, d2)
	}
	if len(d1) != 64 {
		t.Errorf("want 64-char hex digest, got %d: %s", len(d1), d1)
	}
	// 改变 LedgerIDs 顺序不应影响摘要（排序后计算）。
	p2 := *p
	p2.LedgerIDs = []string{"ledger-a", "ledger-b"}
	if p.ComputeDigest() != p2.ComputeDigest() {
		t.Error("digest should be independent of LedgerIDs order")
	}
	// 改变任何金额字段必须产出不同摘要。
	p3 := *p
	p3.FeeTake = "5.01"
	if p.ComputeDigest() == p3.ComputeDigest() {
		t.Error("different fee_take should produce different digest")
	}
}

// TestHedgeVault_GetByPair 按 (take_id, rebate_id) 查询证明。

func TestHedgeVault_GetByPair(t *testing.T) {
	v := newTestVault()
	ctx := context.Background()
	take := takeLease("t-pair", "u50|USDT|hp", "5.00", "")
	rebate := rebateLease("r-pair", "u50|USDT|hp", "5.00", "ldg-pair")
	built, _ := v.Build(ctx, take, rebate)
	got,
		err := v.GetByPair(ctx, take.ID, rebate.ID)
	if err != nil {
		t.Fatalf("get_by_pair: %v", err)
	}
	if got.ID != built.ID {
		t.Errorf("want proof_id=%s, got %s", built.ID, got.ID)
	}
}
