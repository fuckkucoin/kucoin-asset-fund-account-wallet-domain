package holdwritegate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/holdwritegate"
)

// ──────────────────────────────────────────
// Mock GapQuerier
// ──────────────────────────────────────────

type mockGapQuerier struct {
	gapAmount	string
	gapOpen		bool
	err		error
}

func (m *mockGapQuerier) QueryGap(_ context.Context, _, _, _ string) (string, bool, error) {
	return m.gapAmount, m.gapOpen, m.err
}

func newTestEngine(gapQ holdwritegate.HoldGapQuerier, now time.Time) *holdwritegate.Engine {
	return holdwritegate.New(holdwritegate.Config{
		Store:		holdwritegate.NewMemStore(),
		GapQuerier:	gapQ,
		Clock:		func() time.Time { return now },
	})
}

// ──────────────────────────────────────────
// TestEscalateAdvice 建议升级 table-driven
// ──────────────────────────────────────────

func TestEscalateAdvice(t *testing.T) {
	baseTime := time.Date(2021,
		9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		asset		string
		book		string
		adviceID	string
		reason		string
		kind		holdwritegate.EscalationKind
		prevEscalate	bool
		wantGen		int64
		wantLeaseState	holdwritegate.BlockLeaseState
		wantEscState	holdwritegate.EscalationState
		wantErr		string
	}{
		{
			name:		"drift_advice_first_escalation",
			userID:		"USER-001",
			asset:		"USDT",
			book:		"funding",
			adviceID:	"ADV-001",
			reason:		"drift_detected",
			kind:		holdwritegate.EscalationDriftAdvice,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"hold_gap_escalation",
			userID:		"USER-002",
			asset:		"BTC",
			book:		"funding",
			adviceID:	"ADV-002",
			reason:		"hold_gap_found",
			kind:		holdwritegate.EscalationHoldGap,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"orphan_tag_escalation",
			userID:		"USER-003",
			asset:		"ETH",
			book:		"funding",
			adviceID:	"ADV-003",
			reason:		"orphan_holdcat_tag",
			kind:		holdwritegate.EscalationOrphanTag,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"manual_block_escalation",
			userID:		"USER-004",
			asset:		"USDT",
			book:		"trading",
			adviceID:	"ADV-004",
			reason:		"manual_investigation",
			kind:		holdwritegate.EscalationManualBlock,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"second_escalation_same_subject",
			userID:		"USER-005",
			asset:		"USDT",
			book:		"funding",
			adviceID:	"ADV-005",
			reason:		"second_drift",
			kind:		holdwritegate.EscalationDriftAdvice,
			prevEscalate:	true,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"default_book_funding",
			userID:		"USER-006",
			asset:		"SOL",
			book:		"",
			adviceID:	"ADV-006",
			reason:		"gap",
			kind:		holdwritegate.EscalationHoldGap,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			asset:		"USDT",
			adviceID:	"ADV-007",
			reason:		"test",
			wantErr:	"user",
		},
		{
			name:		"empty_asset_rejected",
			userID:		"USER-007",
			asset:		"",
			adviceID:	"ADV-008",
			reason:		"test",
			wantErr:	"asset",
		},
		{
			name:		"empty_advice_id_rejected",
			userID:		"USER-008",
			asset:		"USDT",
			reason:		"test",
			wantErr:	"advice_id",
		},
		{
			name:		"empty_reason_rejected",
			userID:		"USER-009",
			asset:		"USDT",
			adviceID:	"ADV-009",
			wantErr:	"reason",
		},
		{
			name:		"whitespace_user_rejected",
			userID:		"   ",
			asset:		"USDT",
			adviceID:	"ADV-010",
			reason:		"test",
			wantErr:	"user",
		},
		{
			name:		"asset_case_normalized",
			userID:		"USER-010",
			asset:		"usdt",
			book:		"FUNDING",
			adviceID:	"ADV-011",
			reason:		"normalize_test",
			kind:		holdwritegate.EscalationDriftAdvice,
			wantGen:	1,
			wantLeaseState:	holdwritegate.BlockLeaseBlocking,
			wantEscState:	holdwritegate.EscalationOpen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			if tt.prevEscalate {
				_, _, _ = eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		tt.userID,
					Asset:		tt.asset,
					Book:		tt.book,
					AdviceID:	"PREV-ADV",
					Reason:		"prev_reason",
					Kind:		holdwritegate.EscalationDriftAdvice,
				})
			}
			esc, lease, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
				UserID:		tt.userID,
				Asset:		tt.asset,
				Book:		tt.book,
				AdviceID:	tt.adviceID,
				Reason:		tt.reason,
				Kind:		tt.kind,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.BlockGen != tt.wantGen {
				t.Errorf("block_gen: want %d, got %d", tt.wantGen, lease.BlockGen)
			}
			if lease.State != tt.wantLeaseState {
				t.Errorf("lease state: want %s, got %s", tt.wantLeaseState, lease.State)
			}
			if esc.State != tt.wantEscState {
				t.Errorf("escalation state: want %s, got %s", tt.wantEscState, esc.State)
			}
			if esc.BlockGen != tt.wantGen {
				t.Errorf("esc block_gen: want %d, got %d", tt.wantGen, esc.BlockGen)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestCheckWriteGate 写门检查 table-driven
// ──────────────────────────────────────────

func TestCheckWriteGate(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		asset		string
		book		string
		writeID		string
		writeKind	string
		setupBlock	bool
		blockAsset	string
		wantAllowed	bool
		wantErr		string
	}{
		{
			name:		"no_block_allowed",
			userID:		"USER-WG-001",
			asset:		"USDT",
			book:		"funding",
			writeID:	"WR-001",
			writeKind:	"hold_release",
			wantAllowed:	true,
		},
		{
			name:		"active_block_rejected",
			userID:		"USER-WG-002",
			asset:		"USDT",
			book:		"funding",
			writeID:	"WR-002",
			writeKind:	"hold_release",
			setupBlock:	true,
			blockAsset:	"USDT",
			wantAllowed:	false,
		},
		{
			name:		"different_asset_allowed",
			userID:		"USER-WG-003",
			asset:		"ETH",
			book:		"funding",
			writeID:	"WR-003",
			writeKind:	"fund_transfer",
			setupBlock:	true,
			blockAsset:	"BTC",
			wantAllowed:	true,
		},
		{
			name:		"hold_release_blocked",
			userID:		"USER-WG-004",
			asset:		"BTC",
			book:		"funding",
			writeID:	"WR-004",
			writeKind:	"hold_release",
			setupBlock:	true,
			blockAsset:	"BTC",
			wantAllowed:	false,
		},
		{
			name:		"fund_transfer_blocked",
			userID:		"USER-WG-005",
			asset:		"USDT",
			book:		"funding",
			writeID:	"WR-005",
			writeKind:	"fund_transfer",
			setupBlock:	true,
			blockAsset:	"USDT",
			wantAllowed:	false,
		},
		{
			name:		"freeze_release_blocked",
			userID:		"USER-WG-006",
			asset:		"USDT",
			book:		"funding",
			writeID:	"WR-006",
			writeKind:	"freeze_release",
			setupBlock:	true,
			blockAsset:	"USDT",
			wantAllowed:	false,
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			asset:		"USDT",
			writeID:	"WR-007",
			wantErr:	"user",
		},
		{
			name:		"empty_asset_rejected",
			userID:		"USER-WG-008",
			asset:		"",
			writeID:	"WR-008",
			wantErr:	"asset",
		},
		{
			name:		"empty_write_id_rejected",
			userID:		"USER-WG-009",
			asset:		"USDT",
			writeID:	"",
			wantErr:	"write_id",
		},
		{
			name:		"different_book_allowed",
			userID:		"USER-WG-010",
			asset:		"USDT",
			book:		"trading",
			writeID:	"WR-010",
			writeKind:	"hold_release",
			setupBlock:	true,
			blockAsset:	"USDT",
			wantAllowed:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			if tt.setupBlock {
				_, _, _ = eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		tt.userID,
					Asset:		tt.blockAsset,
					Book:		"funding",
					AdviceID:	"ADV-BLOCK-" + tt.name,
					Reason:		"block_test",
					Kind:		holdwritegate.EscalationDriftAdvice,
				})
			}
			book := tt.book
			if book == "" {
				book = "funding"
			}
			resp, err := eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
				UserID:		tt.userID,
				Asset:		tt.asset,
				Book:		book,
				WriteID:	tt.writeID,
				WriteKind:	tt.writeKind,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Allowed != tt.wantAllowed {
				t.Errorf("allowed: want %v, got %v", tt.wantAllowed, resp.Allowed)
			}
			if !tt.wantAllowed && resp.LeaseID == "" {
				t.Error("expected lease_id when blocked")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestSubmitCloseProof 提交证明 table-driven
// ──────────────────────────────────────────

func TestSubmitCloseProof(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupLease	bool
		leaseID		string
		gapBefore	string
		gapAfter	string
		gapClosed	bool
		wantErr		string
	}{
		{
			name:		"valid_proof_submitted",
			setupLease:	true,
			gapBefore:	"100.50",
			gapAfter:	"0.00",
			gapClosed:	true,
		},
		{
			name:		"gap_not_closed_proof",
			setupLease:	true,
			gapBefore:	"200.00",
			gapAfter:	"50.00",
			gapClosed:	false,
		},
		{
			name:		"lease_not_found",
			leaseID:	"NONEXIST",
			wantErr:	"not found",
		},
		{
			name:		"empty_lease_id",
			leaseID:	"",
			wantErr:	"lease_id",
		},
		{
			name:		"zero_gap",
			setupLease:	true,
			gapBefore:	"0.00",
			gapAfter:	"0.00",
			gapClosed:	true,
		},
		{
			name:		"large_gap",
			setupLease:	true,
			gapBefore:	"999999.99",
			gapAfter:	"0.00",
			gapClosed:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			leaseID := tt.leaseID
			if tt.setupLease {
				_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		"USER-PROOF-" + tt.name,
					Asset:		"USDT",
					Book:		"funding",
					AdviceID:	"ADV-PROOF-" + tt.name,
					Reason:		"proof_test",
					Kind:		holdwritegate.EscalationHoldGap,
				})
				leaseID = lease.ID
			}
			proof, err := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
				LeaseID:	leaseID,
				GapBefore:	tt.gapBefore,
				GapAfter:	tt.gapAfter,
				GapClosed:	tt.gapClosed,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proof.GapClosed != tt.gapClosed {
				t.Errorf("gap_closed: want %v, got %v", tt.gapClosed, proof.GapClosed)
			}
			if proof.Validated {
				t.Error("new proof should not be validated")
			}
			if proof.LeaseID != leaseID {
				t.Errorf("lease_id: want %s, got %s", leaseID, proof.LeaseID)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestValidateProof 校验证明 table-driven
// ──────────────────────────────────────────

func TestValidateProof(t *testing.T) {
	baseTime := time.Date(2021,
		9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupProof	bool
		gapClosed	bool
		preValidated	bool
		wantValidated	bool
		wantErr		string
	}{
		{
			name:		"validate_closed_gap",
			setupProof:	true,
			gapClosed:	true,
			wantValidated:	true,
		},
		{
			name:		"gap_not_closed_rejected",
			setupProof:	true,
			gapClosed:	false,
			wantErr:	"gap not closed",
		},
		{
			name:		"already_validated_idempotent",
			setupProof:	true,
			gapClosed:	true,
			preValidated:	true,
			wantValidated:	true,
		},
		{
			name:		"proof_not_found",
			wantErr:	"not found",
		},
		{
			name:		"empty_proof_id",
			wantErr:	"proof_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			proofID := ""
			if tt.setupProof {
				_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		"USER-VAL-" + tt.name,
					Asset:		"USDT",
					Book:		"funding",
					AdviceID:	"ADV-VAL-" + tt.name,
					Reason:		"val_test",
					Kind:		holdwritegate.EscalationHoldGap,
				})
				proof, _ := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
					LeaseID:	lease.ID,
					GapBefore:	"100.00",
					GapAfter:	"0.00",
					GapClosed:	tt.gapClosed,
				})
				proofID = proof.ID
				if tt.preValidated && tt.gapClosed {
					_, _ = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{
						ProofID:	proofID,
						ValidatedBy:	"PRE-VALIDATOR",
					})
				}
			} else {
				if tt.wantErr == "proof_id" {
					proofID = ""
				} else {
					proofID = "NONEXIST"
				}
			}
			proof, err := eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{
				ProofID:	proofID,
				ValidatedBy:	"TEST-VALIDATOR",
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proof.Validated != tt.wantValidated {
				t.Errorf("validated: want %v, got %v", tt.wantValidated, proof.Validated)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestDualControlClear 双人审批清除 table-driven
// ──────────────────────────────────────────

func TestDualControlClear(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupFull	bool
		noProof		bool
		unvalidated	bool
		sameApprover	bool
		gapNotClosed	bool
		wantState	holdwritegate.BlockLeaseState
		wantErr		string
	}{
		{
			name:		"clear_with_valid_proof",
			setupFull:	true,
			wantState:	holdwritegate.BlockLeaseCleared,
		},
		{
			name:		"clear_without_proof_rejected",
			noProof:	true,
			wantErr:	"proof required",
		},
		{
			name:		"unvalidated_proof_rejected",
			setupFull:	true,
			unvalidated:	true,
			wantErr:	"not validated",
		},
		{
			name:		"same_approver_rejected",
			setupFull:	true,
			sameApprover:	true,
			wantErr:	"dual-control",
		},
		{
			name:		"gap_not_closed_rejected",
			setupFull:	true,
			gapNotClosed:	true,
			wantErr:	"not validated",
		},
		{
			name:		"lease_not_found",
			wantErr:	"not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			var leaseID, proofID string
			if tt.setupFull || tt.unvalidated || tt.gapNotClosed {
				_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		"USER-DC-" + tt.name,
					Asset:		"USDT",
					Book:		"funding",
					AdviceID:	"ADV-DC-" + tt.name,
					Reason:		"dc_test",
					Kind:		holdwritegate.EscalationHoldGap,
				})
				leaseID = lease.ID
				gapClosed := true
				if tt.gapNotClosed {
					gapClosed = false
				}
				proof, _ := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
					LeaseID:	leaseID,
					GapBefore:	"100.00",
					GapAfter:	"0.00",
					GapClosed:	gapClosed,
				})
				proofID = proof.ID
				if !tt.unvalidated && !tt.gapNotClosed {
					_, _ = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{
						ProofID:	proofID,
						ValidatedBy:	"VALIDATOR",
					})
				}
			} else if tt.noProof {
				_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		"USER-DC-NP",
					Asset:		"USDT",
					Book:		"funding",
					AdviceID:	"ADV-DC-NP",
					Reason:		"dc_no_proof",
					Kind:		holdwritegate.EscalationHoldGap,
				})
				leaseID = lease.ID
				proofID = ""
			} else {
				leaseID = "NONEXIST"
				proofID = "NONEXIST"
			}
			approverA := "OPS-A"
			approverB := "OPS-B"
			if tt.sameApprover {
				approverA = "OPS-SAME"
				approverB = "OPS-SAME"
			}
			lease, err := eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
				LeaseID:	leaseID,
				ProofID:	proofID,
				ApproverA:	approverA,
				ApproverB:	approverB,
				Resolution:	"test_clear",
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, lease.State)
			}
			if lease.ClearProofID == "" {
				t.Error("expected clear_proof_id")
			}
			if lease.ClearedBy == "" {
				t.Error("expected cleared_by")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestBindOrphanTag 孤儿标签绑定 table-driven
// ──────────────────────────────────────────

func TestBindOrphanTag(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupLease	bool
		holdcatID	string
		cleared		bool
		expired		bool
		wantErr		string
	}{
		{
			name:		"bind_to_active_lease",
			setupLease:	true,
			holdcatID:	"HOLDCAT-001",
		},
		{
			name:		"bind_second_orphan",
			setupLease:	true,
			holdcatID:	"HOLDCAT-002",
		},
		{
			name:		"lease_not_found",
			holdcatID:	"HC-NOTFOUND",
			wantErr:	"lease not found",
		},
		{
			name:		"empty_holdcat_id",
			setupLease:	true,
			holdcatID:	"",
			wantErr:	"orphan tag bind failed",
		},
		{
			name:		"cleared_lease_rejected",
			setupLease:	true,
			holdcatID:	"HOLDCAT-CLR",
			cleared:	true,
			wantErr:	"expired",
		},
		{
			name:		"expired_lease_rejected",
			setupLease:	true,
			holdcatID:	"HOLDCAT-EXP",
			expired:	true,
			wantErr:	"expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			var leaseID string
			if tt.setupLease {
				_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		"USER-OT-" + tt.name,
					Asset:		"ETH",
					Book:		"funding",
					AdviceID:	"ADV-OT-" + tt.name,
					Reason:		"orphan_test",
					Kind:		holdwritegate.EscalationOrphanTag,
				})
				leaseID = lease.ID
				if tt.cleared {
					proof, _ := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
						LeaseID:	leaseID,
						GapBefore:	"10",
						GapAfter:	"0",
						GapClosed:	true,
					})
					_, _ = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{ProofID: proof.ID, ValidatedBy: "V"})
					_, _ = eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
						LeaseID:	leaseID, ProofID: proof.ID,
						ApproverA:	"A", ApproverB: "B",
					})
				}
				if tt.expired {
					_, _ = eng.ExpireLease(ctx, leaseID)
				}
			} else {
				leaseID = "NONEXIST"
			}
			bind, err := eng.BindOrphanTag(ctx, holdwritegate.BindOrphanTagReq{
				LeaseID:	leaseID,
				HoldcatID:	tt.holdcatID,
				UserID:		"USER-OT",
				Asset:		"ETH",
				Book:		"funding",
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bind.LeaseID != leaseID {
				t.Errorf("lease_id: want %s, got %s", leaseID, bind.LeaseID)
			}
			if bind.HoldcatID != tt.holdcatID {
				t.Errorf("holdcat_id: want %s, got %s", tt.holdcatID, bind.HoldcatID)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestExpireLease 过期租约 table-driven
// ──────────────────────────────────────────

func TestExpireLease(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setup		bool
		preState	holdwritegate.BlockLeaseState
		wantState	holdwritegate.BlockLeaseState
		wantErr		string
	}{
		{
			name:		"expire_blocking_lease",
			setup:		true,
			preState:	holdwritegate.BlockLeaseBlocking,
			wantState:	holdwritegate.BlockLeaseExpired,
		},
		{
			name:		"expire_open_lease",
			setup:		true,
			preState:	holdwritegate.BlockLeaseOpen,
			wantState:	holdwritegate.BlockLeaseExpired,
		},
		{
			name:		"already_expired_idempotent",
			setup:		true,
			preState:	holdwritegate.BlockLeaseExpired,
			wantState:	holdwritegate.BlockLeaseExpired,
		},
		{
			name:		"already_cleared_noop",
			setup:		true,
			preState:	holdwritegate.BlockLeaseCleared,
			wantState:	holdwritegate.BlockLeaseCleared,
		},
		{
			name:		"not_found",
			wantErr:	"not found",
		},
		{
			name:		"empty_lease_id",
			wantErr:	"lease_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			leaseID := ""
			if tt.setup {
				leaseID = "LEASE-EXP-" + tt.name
				_, _ = eng.store.SaveBlockLease(ctx, holdwritegate.HoldBlockLease{
					ID:		leaseID,
					UserID:		"USER-EXP",
					Asset:		"USDT",
					Book:		"funding",
					State:		tt.preState,
					BlockGen:	1,
					LeaseExpiry:	baseTime.Add(72 * time.Hour),
					CreatedAt:	baseTime,
					UpdatedAt:	baseTime,
				})
			} else if tt.wantErr == "lease_id" {
				leaseID = ""
			} else {
				leaseID = "NONEXIST"
			}
			lease, err := eng.ExpireLease(ctx, leaseID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, lease.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestSealProof 封口证明 table-driven
// ──────────────────────────────────────────

func TestSealProof(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setup		bool
		preSealed	bool
		wantSealed	bool
		wantErr		string
	}{
		{
			name:		"seal_unsealed",
			setup:		true,
			wantSealed:	true,
		},
		{
			name:		"already_sealed_idempotent",
			setup:		true,
			preSealed:	true,
			wantSealed:	true,
		},
		{
			name:		"not_found",
			wantErr:	"not found",
		},
		{
			name:		"empty_proof_id",
			wantErr:	"proof_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			proofID := ""
			if tt.setup {
				proofID = "PROOF-SEAL-" + tt.name
				_, _ = eng.store.SaveCloseProof(ctx, holdwritegate.GapCloseProof{
					ID:		proofID,
					LeaseID:	"LEASE-SEAL",
					Sealed:		tt.preSealed,
				})
			} else if tt.wantErr == "proof_id" {
				proofID = ""
			} else {
				proofID = "NONEXIST"
			}
			proof, err := eng.SealProof(ctx, proofID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proof.Sealed != tt.wantSealed {
				t.Errorf("sealed: want %v, got %v", tt.wantSealed, proof.Sealed)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestBlockLeaseFSM FSM 转移 table-driven
// ──────────────────────────────────────────

func TestBlockLeaseFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	holdwritegate.BlockLeaseState
		event	string
		wantTo	holdwritegate.BlockLeaseState
		wantErr	bool
	}{
		{"open_to_blocking", holdwritegate.BlockLeaseOpen, holdwritegate.BlockEvActivate, holdwritegate.BlockLeaseBlocking, false},
		{"open_to_expired", holdwritegate.BlockLeaseOpen, holdwritegate.BlockEvExpire, holdwritegate.BlockLeaseExpired, false},
		{"blocking_to_cleared", holdwritegate.BlockLeaseBlocking, holdwritegate.BlockEvClear, holdwritegate.BlockLeaseCleared, false},
		{"blocking_to_expired", holdwritegate.BlockLeaseBlocking, holdwritegate.BlockEvExpire, holdwritegate.BlockLeaseExpired, false},
		{"cleared_no_activate", holdwritegate.BlockLeaseCleared, holdwritegate.BlockEvActivate, holdwritegate.BlockLeaseCleared, true},
		{"cleared_no_clear", holdwritegate.BlockLeaseCleared, holdwritegate.BlockEvClear, holdwritegate.BlockLeaseCleared, true},
		{"cleared_no_expire", holdwritegate.BlockLeaseCleared, holdwritegate.BlockEvExpire, holdwritegate.BlockLeaseCleared, true},
		{"expired_no_activate", holdwritegate.BlockLeaseExpired, holdwritegate.BlockEvActivate, holdwritegate.BlockLeaseExpired, true},
		{"expired_no_clear", holdwritegate.BlockLeaseExpired, holdwritegate.BlockEvClear, holdwritegate.BlockLeaseExpired, true},
		{"open_no_clear", holdwritegate.BlockLeaseOpen, holdwritegate.BlockEvClear, holdwritegate.BlockLeaseOpen, true},
	}
	fsm := holdwritegate.NewBlockLeaseFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, err := fsm.Transition(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if to != tt.wantTo {
				t.Errorf("want %s, got %s", tt.wantTo, to)
			}
		})
	}
}

func TestEscalationFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	holdwritegate.EscalationState
		event	string
		wantTo	holdwritegate.EscalationState
		wantErr	bool
	}{
		{"open_to_proven", holdwritegate.EscalationOpen, holdwritegate.EscalationEvProve, holdwritegate.EscalationProven, false},
		{"proven_to_closed", holdwritegate.EscalationProven, holdwritegate.EscalationEvClose, holdwritegate.EscalationClosed, false},
		{"open_no_close", holdwritegate.EscalationOpen, holdwritegate.EscalationEvClose, holdwritegate.EscalationOpen, true},
		{"closed_no_prove", holdwritegate.EscalationClosed, holdwritegate.EscalationEvProve, holdwritegate.EscalationClosed, true},
		{"closed_no_close", holdwritegate.EscalationClosed, holdwritegate.EscalationEvClose, holdwritegate.EscalationClosed, true},
		{"proven_no_prove", holdwritegate.EscalationProven, holdwritegate.EscalationEvProve, holdwritegate.EscalationProven, true},
	}
	fsm := holdwritegate.NewEscalationFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, err := fsm.Transition(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if to != tt.wantTo {
				t.Errorf("want %s, got %s", tt.wantTo, to)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestBlockGenQuery 代次查询 table-driven
// ──────────────────────────────────────────

func TestBlockGenQuery(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		asset		string
		book		string
		escalates	int
		wantGen		int64
		wantErr		string
	}{
		{
			name:		"no_blocks_gen_0",
			userID:		"USER-BGQ-001",
			asset:		"USDT",
			book:		"funding",
			wantGen:	0,
		},
		{
			name:		"one_escalation_gen_1",
			userID:		"USER-BGQ-002",
			asset:		"USDT",
			book:		"funding",
			escalates:	1,
			wantGen:	1,
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			asset:		"USDT",
			book:		"funding",
			wantErr:	"user",
		},
		{
			name:		"empty_asset_rejected",
			userID:		"USER-BGQ-003",
			asset:		"",
			book:		"funding",
			wantErr:	"asset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil,
				baseTime)
			ctx := context.Background()
			for i := 0; i < tt.escalates; i++ {
				_, _, _ = eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
					UserID:		tt.userID,
					Asset:		tt.asset,
					Book:		tt.book,
					AdviceID:	fmt.Sprintf("ADV-BGQ-%d", i),
					Reason:		"gen_test",
					Kind:		holdwritegate.EscalationDriftAdvice,
				})
			}
			resp, err := eng.BlockGenQuery(ctx, tt.userID, tt.asset, tt.book)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.CurrentGen != tt.wantGen {
				t.Errorf("gen: want %d, got %d", tt.wantGen, resp.CurrentGen)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestProofExport 证明导出 table-driven
// ──────────────────────────────────────────

func TestProofExport(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		leaseID		string
		userID		string
		setupProof	bool
		wantCount	int
		wantErr		string
	}{
		{
			name:		"by_user_empty",
			userID:		"USER-PE-001",
			wantCount:	0,
		},
		{
			name:		"missing_params",
			wantErr:	"lease_id",
		},
		{
			name:		"by_lease",
			leaseID:	"LEASE-PE-001",
			setupProof:	true,
			wantCount:	1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			if tt.setupProof && tt.leaseID != "" {
				_, _ = eng.store.SaveCloseProof(ctx, holdwritegate.GapCloseProof{
					ID:		"PROOF-PE-" + tt.name,
					LeaseID:	tt.leaseID,
					UserID:		"USER-PE-SETUP",
				})
			}
			proofs, err := eng.ProofExport(ctx, holdwritegate.ProofExportReq{
				LeaseID:	tt.leaseID,
				UserID:		tt.userID,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(proofs) != tt.wantCount {
				t.Errorf("proof count: want %d, got %d", tt.wantCount, len(proofs))
			}
		})
	}
}

// ──────────────────────────────────────────
// TestNilEngine 空引擎防御
// ──────────────────────────────────────────

func TestNilEngine(t *testing.T) {
	var eng *holdwritegate.Engine
	ctx := context.Background()
	_, _, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{UserID: "u", Asset: "a", AdviceID: "x", Reason: "r"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("EscalateAdvice: want ErrEngineNil, got %v", err)
	}
	_, err = eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{UserID: "u", Asset: "a", WriteID: "w"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("CheckWriteGate: want ErrEngineNil, got %v", err)
	}
	_, err = eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{LeaseID: "l"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("SubmitCloseProof: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{ProofID: "p"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ValidateProof: want ErrEngineNil, got %v", err)
	}
	_, err = eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{LeaseID: "l", ProofID: "p", ApproverA: "a", ApproverB: "b"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("DualControlClear: want ErrEngineNil, got %v", err)
	}
	_, err = eng.BindOrphanTag(ctx, holdwritegate.BindOrphanTagReq{LeaseID: "l", HoldcatID: "h"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("BindOrphanTag: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExpireLease(ctx, "l")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ExpireLease: want ErrEngineNil, got %v", err)
	}
	_, err = eng.SealProof(ctx, "p")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("SealProof: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetBlockLease(ctx, "l")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("GetBlockLease: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetEscalation(ctx, "e")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("GetEscalation: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetCloseProof(ctx, "p")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("GetCloseProof: want ErrEngineNil, got %v", err)
	}
	_, err = eng.BlockQuery(ctx, holdwritegate.BlockQueryReq{UserID: "u"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("BlockQuery: want ErrEngineNil, got %v", err)
	}
	_, err = eng.BlockGenQuery(ctx, "u", "a", "b")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("BlockGenQuery: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ProofExport(ctx, holdwritegate.ProofExportReq{UserID: "u"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ProofExport: want ErrEngineNil, got %v", err)
	}
	_, err = eng.EscalationQuery(ctx, holdwritegate.EscalationQueryReq{UserID: "u"})
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("EscalationQuery: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ListWriteGateEntries(ctx, "l", 10)
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ListWriteGateEntries: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ListOrphanBinds(ctx, "l", 10)
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ListOrphanBinds: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ListBlockingLeases(ctx, 10)
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("ListBlockingLeases: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetActiveLeaseBySubject(ctx, "u", "a", "b")
	if !errors.Is(err, holdwritegate.ErrEngineNil) {
		t.Errorf("GetActiveLeaseBySubject: want ErrEngineNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// TestWorkerAdviceEscalateScan 建议升级扫描
// ──────────────────────────────────────────

func TestWorkerAdviceEscalateScan(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		leaseExpired	bool
		leaseCleared	bool
		wantEscClosed	bool
	}{
		{
			name:		"expired_lease_closes_escalation",
			leaseExpired:	true,
			wantEscClosed:	true,
		},
		{
			name:		"cleared_lease_closes_escalation",
			leaseCleared:	true,
			wantEscClosed:	true,
		},
		{
			name:		"active_lease_keeps_escalation",
			wantEscClosed:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
				UserID:		"USER-WK-" + tt.name,
				Asset:		"USDT",
				Book:		"funding",
				AdviceID:	"ADV-WK-" + tt.name,
				Reason:		"wk_test",
				Kind:		holdwritegate.EscalationDriftAdvice,
			})
			if tt.leaseExpired {
				_, _ = eng.ExpireLease(ctx, lease.ID)
			}
			if tt.leaseCleared {
				proof, _ := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
					LeaseID:	lease.ID, GapBefore: "1", GapAfter: "0", GapClosed: true,
				})
				_, _ = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{ProofID: proof.ID, ValidatedBy: "V"})
				_, _ = eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
					LeaseID:	lease.ID, ProofID: proof.ID,
					ApproverA:	"A", ApproverB: "B",
				})
			}
			job := holdwritegate.AdviceEscalateScanJob(holdwritegate.AdviceEscalateScanConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			escs, _ := eng.store.ListEscalationsByState(ctx, holdwritegate.EscalationOpen, 10)
			if tt.wantEscClosed && len(escs) > 0 {
				t.Error("expected all escalations closed")
			}
			if !tt.wantEscClosed && len(escs) == 0 {
				t.Error("expected open escalations")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerBlockExpiry 过期 block 扫描
// ──────────────────────────────────────────

func TestWorkerBlockExpiry(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		leaseExpired	bool
		hasEscalation	bool
		wantTimeout	bool
	}{
		{
			name:		"expired_with_escalation_creates_timeout",
			leaseExpired:	true,
			hasEscalation:	true,
			wantTimeout:	true,
		},
		{
			name:		"expired_no_escalation",
			leaseExpired:	true,
			hasEscalation:	false,
			wantTimeout:	false,
		},
		{
			name:		"active_lease_no_change",
			leaseExpired:	false,
			hasEscalation:	true,
			wantTimeout:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			leaseID := "LEASE-BEX-" + tt.name
			expiry := baseTime.Add(72 * time.Hour)
			if tt.leaseExpired {
				expiry = baseTime.Add(-time.Minute)
			}
			_, _ = eng.store.SaveBlockLease(ctx, holdwritegate.HoldBlockLease{
				ID:		leaseID,
				UserID:		"USER-BEX",
				Asset:		"USDT",
				Book:		"funding",
				State:		holdwritegate.BlockLeaseBlocking,
				BlockGen:	1,
				LeaseExpiry:	expiry,
				CreatedAt:	baseTime,
				UpdatedAt:	baseTime,
			})
			if tt.hasEscalation {
				_, _ = eng.store.SaveEscalation(ctx, holdwritegate.AdviceEscalation{
					ID:		"ESC-BEX-" + tt.name,
					LeaseID:	leaseID,
					UserID:		"USER-BEX",
					Asset:		"USDT",
					Book:		"funding",
					AdviceID:	"ADV-BEX-" + tt.name,
					Kind:		holdwritegate.EscalationDriftAdvice,
					State:		holdwritegate.EscalationOpen,
					BlockGen:	1,
				})
			}
			job := holdwritegate.BlockExpiryJob(holdwritegate.BlockExpiryConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			if tt.leaseExpired {
				l, _ := eng.store.GetBlockLease(ctx, leaseID)
				if l.State != holdwritegate.BlockLeaseExpired {
					t.Errorf("lease state: want expired, got %s", l.State)
				}
			}
			allEscs, _ := eng.store.ListEscalationsByLease(ctx, leaseID, 100)
			hasTimeout := false
			for _, e := range allEscs {
				if e.Kind == holdwritegate.EscalationTimeoutReopen {
					hasTimeout = true
				}
			}
			if tt.wantTimeout && !hasTimeout {
				t.Error("expected timeout_reopen escalation")
			}
			if !tt.wantTimeout && hasTimeout {
				t.Error("unexpected timeout_reopen escalation")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerGapCloseVerify Gap 关闭校验
// ──────────────────────────────────────────

func TestWorkerGapCloseVerify(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		gapOpen		bool
		hasProof	bool
		wantProven	bool
	}{
		{
			name:		"gap_closed_validates_proof",
			gapOpen:	false,
			hasProof:	true,
			wantProven:	true,
		},
		{
			name:		"gap_still_open_no_change",
			gapOpen:	true,
			hasProof:	true,
			wantProven:	false,
		},
		{
			name:		"gap_closed_no_proof_still_proven",
			gapOpen:	false,
			hasProof:	false,
			wantProven:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gapQ := &mockGapQuerier{gapAmount: "0", gapOpen: tt.gapOpen}
			eng := newTestEngine(gapQ, baseTime)
			ctx := context.Background()
			leaseID := "LEASE-GCV-" + tt.name
			_, _ = eng.store.SaveBlockLease(ctx, holdwritegate.HoldBlockLease{
				ID:		leaseID,
				UserID:		"USER-GCV",
				Asset:		"USDT",
				Book:		"funding",
				State:		holdwritegate.BlockLeaseBlocking,
				BlockGen:	1,
				LeaseExpiry:	baseTime.Add(72 * time.Hour),
			})
			escID := "ESC-GCV-" + tt.name
			_, _ = eng.store.SaveEscalation(ctx, holdwritegate.AdviceEscalation{
				ID:		escID,
				LeaseID:	leaseID,
				UserID:		"USER-GCV",
				State:		holdwritegate.EscalationOpen,
				BlockGen:	1,
			})
			if tt.hasProof {
				_, _ = eng.store.SaveCloseProof(ctx, holdwritegate.GapCloseProof{
					ID:		"PROOF-GCV-" + tt.name,
					LeaseID:	leaseID,
					GapClosed:	true,
					Validated:	false,
				})
			}
			job := holdwritegate.GapCloseVerifyJob(holdwritegate.GapCloseVerifyConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			esc, _ := eng.store.GetEscalation(ctx, escID)
			if tt.wantProven && esc.State != holdwritegate.EscalationProven {
				t.Errorf("escalation: want proven, got %s", esc.State)
			}
			if !tt.wantProven && esc.State == holdwritegate.EscalationProven {
				t.Errorf("escalation: want open, got %s", esc.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerEscalationProofPatrol 证明巡检
// ──────────────────────────────────────────

func TestWorkerEscalationProofPatrol(t *testing.T) {
	baseTime := time.Date(2021,
		9, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		proofSealed	bool
	}{
		{
			name:		"unsealed_proof_gets_sealed",
			proofSealed:	false,
		},
		{
			name:		"already_sealed_noop",
			proofSealed:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, baseTime)
			ctx := context.Background()
			proofID := "PROOF-EPP-" + tt.name
			_, _ = eng.store.SaveCloseProof(ctx, holdwritegate.GapCloseProof{
				ID:	proofID,
				Sealed:	tt.proofSealed,
			})
			_, _ = eng.store.SaveEscalation(ctx, holdwritegate.AdviceEscalation{
				ID:		"ESC-EPP-" + tt.name,
				State:		holdwritegate.EscalationProven,
				ProofIDs:	[]string{proofID},
			})
			job := holdwritegate.EscalationProofPatrolJob(holdwritegate.EscalationProofPatrolConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			proof, _ := eng.store.GetCloseProof(ctx, proofID)
			if !proof.Sealed {
				t.Error("expected proof sealed")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestMemStore 存储层 table-driven
// ──────────────────────────────────────────

func TestMemStoreBlockLease(t *testing.T) {
	store := holdwritegate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetBlockLease(ctx, "nonexist")
	if !errors.Is(err, holdwritegate.ErrBlockLeaseNotFound) {
		t.Errorf("want ErrBlockLeaseNotFound, got %v", err)
	}
	saved, err := store.SaveBlockLease(ctx, holdwritegate.HoldBlockLease{
		ID:	"LEASE-1", UserID: "USER-1", Asset: "USDT", Book: "funding",
		State:	holdwritegate.BlockLeaseBlocking, BlockGen: 1,
		LeaseExpiry:	time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID != "LEASE-1" {
		t.Errorf("id: want LEASE-1, got %s", saved.ID)
	}
	got, err := store.GetBlockLease(ctx, "LEASE-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BlockGen != 1 {
		t.Errorf("gen: want 1, got %d", got.BlockGen)
	}
	bySubject, err := store.GetActiveLeaseBySubject(ctx, "USER-1", "USDT", "funding")
	if err != nil {
		t.Fatalf("by subject: %v", err)
	}
	if bySubject.ID != "LEASE-1" {
		t.Errorf("id: want LEASE-1, got %s", bySubject.ID)
	}
	byUser, err := store.ListBlockLeasesByUser(ctx, "USER-1", 10)
	if err != nil {
		t.Fatalf("by user: %v", err)
	}
	if len(byUser) != 1 {
		t.Errorf("want 1 lease, got %d", len(byUser))
	}
	gen, err := store.LatestGenForSubject(ctx, "USER-1", "USDT", "funding")
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if gen != 1 {
		t.Errorf("gen: want 1, got %d", gen)
	}
}

func TestMemStoreEscalation(t *testing.T) {
	store := holdwritegate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetEscalation(ctx, "nonexist")
	if !errors.Is(err, holdwritegate.ErrEscalationNotFound) {
		t.Errorf("want ErrEscalationNotFound, got %v", err)
	}
	_, err = store.SaveEscalation(ctx, holdwritegate.AdviceEscalation{
		ID:	"ESC-1", LeaseID: "LEASE-1", UserID: "USER-1",
		AdviceID:	"ADV-1", State: holdwritegate.EscalationOpen, Kind: holdwritegate.EscalationDriftAdvice,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	byLease, _ := store.ListEscalationsByLease(ctx, "LEASE-1", 10)
	if len(byLease) != 1 {
		t.Errorf("want 1, got %d", len(byLease))
	}
	byState, _ := store.ListEscalationsByState(ctx, holdwritegate.EscalationOpen, 10)
	if len(byState) != 1 {
		t.Errorf("want 1, got %d", len(byState))
	}
	byUser, _ := store.ListEscalationsByUser(ctx, "USER-1", 10)
	if len(byUser) != 1 {
		t.Errorf("want 1, got %d", len(byUser))
	}
	byAdvice, _ := store.GetEscalationByAdvice(ctx, "ADV-1")
	if byAdvice.ID != "ESC-1" {
		t.Errorf("want ESC-1, got %s", byAdvice.ID)
	}
}

func TestMemStoreCloseProof(t *testing.T) {
	store := holdwritegate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetCloseProof(ctx, "nonexist")
	if !errors.Is(err, holdwritegate.ErrCloseProofNotFound) {
		t.Errorf("want ErrCloseProofNotFound, got %v", err)
	}
	_, err = store.SaveCloseProof(ctx, holdwritegate.GapCloseProof{
		ID:	"PROOF-1", LeaseID: "LEASE-1", UserID: "USER-1",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	byLease, _ := store.ListProofsByLease(ctx, "LEASE-1", 10)
	if len(byLease) != 1 {
		t.Errorf("want 1, got %d", len(byLease))
	}
	byUser, _ := store.ListProofsByUser(ctx, "USER-1", 10)
	if len(byUser) != 1 {
		t.Errorf("want 1, got %d", len(byUser))
	}
}

func TestMemStoreWriteGateEntry(t *testing.T) {
	store := holdwritegate.NewMemStore()
	ctx := context.Background()
	_, err := store.SaveWriteGateEntry(ctx, holdwritegate.WriteGateEntry{
		ID:	"WGE-1", LeaseID: "LEASE-1", WriteID: "WR-1",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	byLease, _ := store.ListWriteGateByLease(ctx, "LEASE-1", 10)
	if len(byLease) != 1 {
		t.Errorf("want 1, got %d", len(byLease))
	}
}

func TestMemStoreOrphanTagBind(t *testing.T) {
	store := holdwritegate.NewMemStore()
	ctx := context.Background()
	_, err := store.SaveOrphanTagBind(ctx, holdwritegate.OrphanTagBind{
		ID:	"OTB-1", LeaseID: "LEASE-1", HoldcatID: "HC-1",
		BoundAt:	time.Now(),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	byLease, _ := store.ListOrphanBindsByLease(ctx, "LEASE-1", 10)
	if len(byLease) != 1 {
		t.Errorf("want 1, got %d", len(byLease))
	}
}

// ──────────────────────────────────────────
// TestEndToEnd 端到端场景
// ──────────────────────────────────────────

func TestEndToEnd_EscalateBlockClearRestore(t *testing.T) {
	baseTime := time.Date(2021,
		9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	esc, lease, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-001",
		Asset:		"USDT",
		Book:		"funding",
		AdviceID:	"ADV-E2E-001",
		Reason:		"drift_gap",
		Kind:		holdwritegate.EscalationHoldGap,
	})
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	if lease.State != holdwritegate.BlockLeaseBlocking {
		t.Fatalf("lease state: want blocking, got %s", lease.State)
	}
	if esc.State != holdwritegate.EscalationOpen {
		t.Fatalf("escalation state: want open, got %s", esc.State)
	}
	resp, err := eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-001", Asset: "USDT", Book: "funding",
		WriteID:	"RELEASE-E2E", WriteKind: "hold_release",
	})
	if err != nil {
		t.Fatalf("write gate: %v", err)
	}
	if resp.Allowed {
		t.Fatal("expected write blocked")
	}
	proof, err := eng.SubmitCloseProof(ctx, holdwritegate.SubmitCloseProofReq{
		LeaseID:	lease.ID,
		GapBefore:	"500.00",
		GapAfter:	"0.00",
		GapClosed:	true,
	})
	if err != nil {
		t.Fatalf("submit proof: %v", err)
	}
	if proof.Validated {
		t.Fatal("new proof should not be validated")
	}
	_, err = eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
		LeaseID:	lease.ID, ProofID: proof.ID,
		ApproverA:	"A", ApproverB: "B",
	})
	if err == nil {
		t.Fatal("expected clear rejected for unvalidated proof")
	}
	if !strings.Contains(err.Error(), "not validated") {
		t.Fatalf("unexpected error: %v", err)
	}
	proof, err = eng.ValidateProof(ctx, holdwritegate.ValidateProofReq{
		ProofID:	proof.ID,
		ValidatedBy:	"OPS-VALIDATOR",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !proof.Validated {
		t.Fatal("proof should be validated")
	}
	clearedLease,
		err := eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
		LeaseID:	lease.ID,
		ProofID:	proof.ID,
		ApproverA:	"OPS-A",
		ApproverB:	"OPS-B",
		Resolution:	"gap_fixed",
	})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if clearedLease.State != holdwritegate.BlockLeaseCleared {
		t.Fatalf("lease state: want cleared, got %s", clearedLease.State)
	}
	if clearedLease.ClearedBy != "OPS-A+OPS-B" {
		t.Errorf("cleared_by: want OPS-A+OPS-B, got %s", clearedLease.ClearedBy)
	}
	resp2, err := eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-001", Asset: "USDT", Book: "funding",
		WriteID:	"RELEASE-AFTER", WriteKind: "hold_release",
	})
	if err != nil {
		t.Fatalf("write gate after clear: %v", err)
	}
	if !resp2.Allowed {
		t.Fatal("expected write allowed after clear")
	}
}

func TestEndToEnd_OrphanBindSameFamily(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	_, lease, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-002",
		Asset:		"ETH",
		Book:		"funding",
		AdviceID:	"ADV-E2E-002",
		Reason:		"orphan_drift",
		Kind:		holdwritegate.EscalationOrphanTag,
	})
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	bind1, err := eng.BindOrphanTag(ctx, holdwritegate.BindOrphanTagReq{
		LeaseID:	lease.ID, HoldcatID: "HC-ORPHAN-1",
		UserID:	"USER-E2E-002", Asset: "ETH", Book: "funding",
	})
	if err != nil {
		t.Fatalf("bind 1: %v", err)
	}
	if bind1.BlockGen != lease.BlockGen {
		t.Errorf("bind1 gen: want %d, got %d", lease.BlockGen, bind1.BlockGen)
	}
	bind2, err := eng.BindOrphanTag(ctx, holdwritegate.BindOrphanTagReq{
		LeaseID:	lease.ID, HoldcatID: "HC-ORPHAN-2",
		UserID:	"USER-E2E-002", Asset: "ETH", Book: "funding",
	})
	if err != nil {
		t.Fatalf("bind 2: %v", err)
	}
	if bind2.BlockGen != lease.BlockGen {
		t.Errorf("bind2 gen: want %d, got %d", lease.BlockGen, bind2.BlockGen)
	}
	binds, _ := eng.ListOrphanBinds(ctx, lease.ID, 10)
	if len(binds) != 2 {
		t.Errorf("want 2 orphan binds, got %d", len(binds))
	}
}

func TestEndToEnd_IdempotentEscalation(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	esc1, lease1, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-003",
		Asset:		"USDT",
		Book:		"funding",
		AdviceID:	"ADV-IDEM-003",
		Reason:		"idem_test",
		Kind:		holdwritegate.EscalationDriftAdvice,
	})
	if err != nil {
		t.Fatalf("first escalate: %v", err)
	}
	esc2, lease2, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-003",
		Asset:		"USDT",
		Book:		"funding",
		AdviceID:	"ADV-IDEM-003",
		Reason:		"idem_test",
		Kind:		holdwritegate.EscalationDriftAdvice,
	})
	if err != nil {
		t.Fatalf("second escalate: %v", err)
	}
	if esc1.ID != esc2.ID {
		t.Errorf("escalation IDs differ: %s vs %s (should be idempotent)", esc1.ID, esc2.ID)
	}
	if lease1.ID != lease2.ID {
		t.Errorf("lease IDs differ: %s vs %s", lease1.ID, lease2.ID)
	}
}

func TestEndToEnd_DifferentAssetNotBlocked(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	_, _, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-004",
		Asset:		"BTC",
		Book:		"funding",
		AdviceID:	"ADV-E2E-004",
		Reason:		"btc_gap",
		Kind:		holdwritegate.EscalationHoldGap,
	})
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	resp, err := eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-004", Asset: "ETH", Book: "funding",
		WriteID:	"RELEASE-ETH", WriteKind: "hold_release",
	})
	if err != nil {
		t.Fatalf("write gate: %v", err)
	}
	if !resp.Allowed {
		t.Fatal("expected different asset to be allowed")
	}
	resp2, err := eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-004", Asset: "BTC", Book: "funding",
		WriteID:	"RELEASE-BTC", WriteKind: "hold_release",
	})
	if err != nil {
		t.Fatalf("write gate: %v", err)
	}
	if resp2.Allowed {
		t.Fatal("expected BTC to be blocked")
	}
}

func TestEndToEnd_ClearWithoutProofRejected(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	_, lease, err := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-005",
		Asset:		"USDT",
		Book:		"funding",
		AdviceID:	"ADV-E2E-005",
		Reason:		"gap",
		Kind:		holdwritegate.EscalationHoldGap,
	})
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	_, err = eng.DualControlClear(ctx, holdwritegate.DualControlClearReq{
		LeaseID:	lease.ID,
		ProofID:	"",
		ApproverA:	"A",
		ApproverB:	"B",
	})
	if err == nil {
		t.Fatal("expected error for clear without proof")
	}
	if !strings.Contains(err.Error(), "proof required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEndToEnd_WriteGateRecordsEntry(t *testing.T) {
	baseTime := time.Date(2021, 9, 14, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, baseTime)
	ctx := context.Background()
	_, lease, _ := eng.EscalateAdvice(ctx, holdwritegate.EscalateAdviceReq{
		UserID:		"USER-E2E-006",
		Asset:		"USDT",
		Book:		"funding",
		AdviceID:	"ADV-E2E-006",
		Reason:		"test",
		Kind:		holdwritegate.EscalationDriftAdvice,
	})
	_, _ = eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-006", Asset: "USDT", Book: "funding",
		WriteID:	"WR-REC-1", WriteKind: "hold_release",
	})
	_, _ = eng.CheckWriteGate(ctx, holdwritegate.WriteGateCheckReq{
		UserID:	"USER-E2E-006", Asset: "USDT", Book: "funding",
		WriteID:	"WR-REC-2", WriteKind: "fund_transfer",
	})
	entries, err := eng.ListWriteGateEntries(ctx, lease.ID, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("want 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if !e.Rejected {
			t.Errorf("entry %s should be rejected", e.ID)
		}
	}
}
