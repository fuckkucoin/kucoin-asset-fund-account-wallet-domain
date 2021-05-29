package flow_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/holdrel"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func newProtocolStack() (*holdcat.TokenBook, *holdrel.Protocol, *holdrel.GapStore) {
	  kv := persist.NewMemKV()
	  tokens := holdcat.NewTokenBook(kv)
	   proofs := holdrel.NewProofStore(kv)
	  gaps := holdrel.NewGapStore(kv)
	   protocol := holdrel.NewProtocol(tokens, proofs, kv, nil)
	   return tokens, protocol, gaps
  }

  func placeToken(t *testing.T, book *holdcat.TokenBook, userID, asset string, layer holdcat.Layer, rc holdcat.ReasonCode, idemKey string) holdcat.HoldToken {
	  t.Helper()
	  tok, err := book.Place(context.Background(), userID, asset, "funding", "1000",
		   layer, rc, "ledger-ref-x", idemKey, "", "")
	   if err != nil {
		   t.Fatalf("placeToken(%s,%s): %v", layer, rc, err)
	   }
	  return tok
  }

// TestRelease_LayerDenied 覆盖低层不可释放高层冻结的完整矩阵。

  func TestRelease_LayerDenied(t *testing.T) {
	  type releaseCase struct {
		   name string
		  frozenLayer holdcat.Layer
		  frozenRC    holdcat.ReasonCode
		  callerLayer string
		   callerRC    string
		   approvalRef string
		   wantErr error
	   }
	  cases := []releaseCase{
		   {
			   name: "ops_cannot_release_legal",
			  frozenLayer: holdcat.LayerLegal, frozenRC: holdcat.ReasonCourtOrder,
			  callerLayer: "ops", callerRC: "ops_manual_hold",
			   wantErr: holdrel.ErrLayerDenied,
		   },
		   {
			  name:        "ops_cannot_release_aml",
			  frozenLayer: holdcat.LayerAML, frozenRC: holdcat.ReasonSuspiciousTx,
			   callerLayer: "ops", callerRC: "ops_manual_hold",
			   wantErr: holdrel.ErrLayerDenied,
		   },
		   {
			  name: "ops_cannot_release_risk",
			  frozenLayer: holdcat.LayerRisk, frozenRC: holdcat.ReasonKYTHighRisk,
			   callerLayer: "ops", callerRC: "ops_manual_hold",
			  wantErr: holdrel.ErrLayerDenied,
		   },
		  {
			  name: "risk_cannot_release_legal",
			  frozenLayer: holdcat.LayerLegal, frozenRC: holdcat.ReasonCourtOrder,
			  callerLayer: "risk", callerRC: "manual_risk_hold",
			  wantErr: holdrel.ErrLayerDenied,
		  },
		  {
			   name:        "risk_cannot_release_aml",
			   frozenLayer: holdcat.LayerAML, frozenRC: holdcat.ReasonSuspiciousTx,
			  callerLayer: "risk", callerRC: "manual_risk_hold",
			   wantErr: holdrel.ErrLayerDenied,
		  },
		   {
			  name: "aml_cannot_release_legal",
			   frozenLayer: holdcat.LayerLegal, frozenRC: holdcat.ReasonCourtOrder,
			  callerLayer: "aml", callerRC: "sar_filed",
			  wantErr: holdrel.ErrLayerDenied,
		   },
		  // 低层释放低层是允许的（确保不误报）
		  {
			   name: "ops_can_release_ops",
			   frozenLayer: holdcat.LayerOps, frozenRC: holdcat.ReasonOpsManualHold,
			   callerLayer: "ops", callerRC: "ops_manual_hold",
		   },
		  {
			  name: "risk_can_release_ops",
			   frozenLayer: holdcat.LayerOps, frozenRC: holdcat.ReasonOpsManualHold,
			  callerLayer: "risk", callerRC: "manual_risk_hold",
		  },
		   {
			   name: "aml_can_release_risk",
			   frozenLayer: holdcat.LayerRisk, frozenRC: holdcat.ReasonKYTHighRisk,
			  callerLayer: "aml", callerRC: "sar_filed",
			   approvalRef: "AML-APPROV-001", // aml 释放 risk 不需要审批（只有释放 aml/legal 层才需要）
		   },
		   {
			  name:        "legal_can_release_aml_with_approval",
			  frozenLayer: holdcat.LayerAML, frozenRC: holdcat.ReasonSuspiciousTx,
			   callerLayer: "legal", callerRC: "court_order",
			   approvalRef: "COURT-ORDER-001",
		   },
	  }
	  for i, tc := range cases {
		   tc := tc
		  idx := i
		  t.Run(tc.name, func(t *testing.T) {
			   tokens,
				   protocol, _ := newProtocolStack()
			  idemKey := "idem-deny-test-" + string(rune('A'+idx))
			  tok := placeToken(t, tokens, "u1", "USDT", tc.frozenLayer, tc.frozenRC, idemKey+"-place")
			  releaseIdem := idemKey + "-release"
			   approvalRef := tc.approvalRef
			  // aml 层释放 aml 层本身需要 approvalRef
			   if tc.frozenLayer == holdcat.LayerAML && tc.callerLayer == "aml" && approvalRef == "" {
				  approvalRef = "AML-DEFAULT-APPROV"
			   }
			  // legal 层释放需要 approvalRef
			   if tc.frozenLayer == holdcat.LayerLegal && approvalRef == "" {
				   approvalRef = "LEGAL-DEFAULT-APPROV"
			   }
			   _, err := protocol.RequestRelease(context.Background(), holdrel.ReleaseReq{
				  TokenID: tok.ID,
				   CallerLayer: tc.callerLayer,
				  ReasonCode:  tc.callerRC,
				   ApprovalRef: approvalRef,
				  IdemKey:     releaseIdem,
	  ctx := context.Background()
	  tokens, protocol, _ := newProtocolStack()
	  tok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerOps, holdcat.ReasonOpsManualHold, "idem-tok-idem-001")
	  req := holdrel.ReleaseReq{
		  TokenID:     tok.ID,
		   CallerLayer: "ops",
		  ReasonCode: "ops_manual_hold",
		  IdemKey: "idem-proof-001",
		  Requester: "ops-user",
		   At: time.Now().UTC(),
	   }
	   proof1, err := protocol.RequestRelease(ctx, req)
	   if err != nil {
		  t.Fatalf("first release: %v", err)
	  }
	   if proof1.ProofID == "" {
		  t.Error("proof_id empty after release")
	  }
	   // 相同 idem_key 重放
	   proof2, err := protocol.RequestRelease(ctx, req)
	  if err != nil {
		  t.Fatalf("idempotent release: %v", err)
	   }
	  if proof1.ProofID != proof2.ProofID {
		   t.Errorf("idempotent: different proof IDs %s vs %s", proof1.ProofID, proof2.ProofID)
	  }
	   // ProofRef 查询
	  proof3, err := protocol.QueryProof(ctx, req.IdemKey)
	  if err != nil {
		   t.Fatalf("query proof by ref: %v", err)
	  }
	   if proof3.ProofID != proof1.ProofID {
		   t.Errorf("query by ref: different proof IDs")
	  }
	  if proof3.FrozenLayer != "ops" {
		  t.Errorf("frozen_layer=%q want ops", proof3.FrozenLayer)
	  }
   }

// TestRelease_ApprovalRequired legal/aml 层释放必须提供 ApprovalRef。

  func TestRelease_ApprovalRequired(t *testing.T) {
	  ctx := context.Background()
	  cases := []struct {
		  name string
		  frozenLayer holdcat.Layer
		  frozenRC holdcat.ReasonCode
		  callerLayer  string
		   withApproval bool
		   wantErr      error
	   }{
		   {
			  name:        "legal_no_approval",
			   frozenLayer: holdcat.LayerLegal, frozenRC: holdcat.ReasonCourtOrder,
			   callerLayer: "legal", withApproval: false,
			  wantErr: holdrel.ErrApprovalRequired,
		  },
		  {
			  name:        "legal_with_approval_ok",
			   frozenLayer: holdcat.LayerLegal, frozenRC: holdcat.ReasonCourtOrder,
			  callerLayer: "legal", withApproval: true,
		   },
		   {
			   name:        "aml_no_approval",
			   frozenLayer: holdcat.LayerAML, frozenRC: holdcat.ReasonSuspiciousTx,
			  callerLayer: "aml", withApproval: false,
			   wantErr: holdrel.ErrApprovalRequired,
		   },
		   {
			  name: "aml_with_approval_ok",
			   frozenLayer: holdcat.LayerAML, frozenRC: holdcat.ReasonSuspiciousTx,
			  callerLayer: "aml", withApproval: true,
		  },
		   {
			  name: "risk_no_approval_ok", // risk 层不强制审批
			  frozenLayer: holdcat.LayerRisk, frozenRC: holdcat.ReasonKYTHighRisk,
			   callerLayer: "risk", withApproval: false,
		  },
		  {
			  name:        "ops_no_approval_ok",
			   frozenLayer: holdcat.LayerOps, frozenRC: holdcat.ReasonOpsManualHold,
			   callerLayer: "ops", withApproval: false,
		   },
	  }
	  for i, tc := range cases {
		   tc := tc
		   idx := i
		  t.Run(tc.name, func(t *testing.T) {
			  tokens, protocol, _ := newProtocolStack()
			   idemSuffix := string(rune('A' + idx))
			   tok := placeToken(t, tokens, "u1", "USDT", tc.frozenLayer, tc.frozenRC, "idem-approv-"+idemSuffix)
			  approvalRef := ""
			   if tc.withApproval {
				  approvalRef = "APPROV-" + idemSuffix
			  }
			  _, err := protocol.RequestRelease(ctx, holdrel.ReleaseReq{
				   TokenID: tok.ID,
				  CallerLayer: tc.callerLayer,
				   ReasonCode:  string(tc.frozenRC),
				  ApprovalRef: approvalRef,
				   IdemKey:     "idem-approv-rel-" + idemSuffix,
				  Requester: "compliance@internal",
				  At:        time.Now().UTC(),
			  })
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Errorf("want %v got %v", tc.wantErr, err)
				  }
				   return
			  }
			   if err != nil {
				  t.Errorf("unexpected err: %v", err)
			   }
		   })
	  }
  }

// TestRelease_HigherLayerBlocks 同用户存在高层 active token 时低层释放被拒。
  func TestRelease_HigherLayerBlocks(t *testing.T) {
	   ctx := context.Background()
	  tokens, protocol, _ := newProtocolStack()
	   // 加入 legal 层 token（高层）
	  placeToken(t, tokens, "u1", "USDT", holdcat.LayerLegal, holdcat.ReasonCourtOrder, "idem-block-legal")
	  // 加入 ops 层 token（低层，尝试释放）
	  opsTok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerOps, holdcat.ReasonOpsManualHold, "idem-block-ops")
	  // ops 释放 ops 层 token，但因存在 legal 层（priority更高）被拒
	   _, err := protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		   TokenID: opsTok.ID,
		  CallerLayer: "ops",
		  ReasonCode:  "ops_manual_hold",
		   IdemKey:     "idem-block-rel",
		   Requester:   "ops-user",
		   At: time.Now().UTC(),
	  })
	   if !errors.Is(err, holdrel.ErrLayerDenied) {
		   t.Errorf("want ErrLayerDenied when legal layer active, got %v", err)
	   }
   }

// TestRelease_MultiLayerSequence 多层叠加合法释放顺序。

  func TestRelease_MultiLayerSequence(t *testing.T) {
	   ctx := context.Background()
	  tokens,
		  protocol, _ := newProtocolStack()
	   // 加入 legal + aml + risk + ops 四层冻结
	  legalTok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerLegal, holdcat.ReasonCourtOrder, "idem-seq-legal")
	  amlTok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerAML, holdcat.ReasonSuspiciousTx, "idem-seq-aml")
	  riskTok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerRisk, holdcat.ReasonKYTHighRisk, "idem-seq-risk")
	  opsTok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerOps, holdcat.ReasonOpsManualHold, "idem-seq-ops")
	   // 尝试按错误顺序释放（ops 尝试释放最先）应被拒
	   _, err := protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		  TokenID: opsTok.ID, CallerLayer: "ops",
		   ReasonCode: "ops_manual_hold", IdemKey: "idem-seq-rel-ops-fail",
		   Requester: "ops-user", At: time.Now(),
	  })
	  if !errors.Is(err, holdrel.ErrLayerDenied) {
		  t.Errorf("ops release should be denied when legal active, got %v", err)
	   }
	  // 1. legal 先释放
	   _, err = protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		  TokenID: legalTok.ID, CallerLayer: "legal",
		  ReasonCode: "court_order", ApprovalRef: "COURT-LIFT-001",
		  IdemKey: "idem-seq-rel-legal", Requester: "compliance@internal", At: time.Now(),
	   })
	  if err != nil {
		   t.Fatalf("legal release: %v", err)
	   }
	  // 2. aml 释放（legal 已释，aml 是新的最高层）
	  _, err = protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		   TokenID: amlTok.ID, CallerLayer: "aml",
		   ReasonCode: "suspicious_transaction", ApprovalRef: "AML-LIFT-001",
		   IdemKey: "idem-seq-rel-aml", Requester: "compliance@internal", At: time.Now(),
	   })
	   if err != nil {
		   t.Fatalf("aml release: %v", err)
	   }
	   // 3. risk 释放
	   _, err = protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		  TokenID: riskTok.ID, CallerLayer: "risk",
		  ReasonCode: "kyt_high_risk",
		  IdemKey:    "idem-seq-rel-risk", Requester: "risk-system", At: time.Now(),
	  })
	   if err != nil {
		   t.Fatalf("risk release: %v", err)
	  }
	   // 4. ops 释放（此时无更高层阻塞）
	  proof, err := protocol.RequestRelease(ctx, holdrel.ReleaseReq{
		  TokenID: opsTok.ID, CallerLayer: "ops",
		   ReasonCode: "ops_manual_hold",
		   IdemKey: "idem-seq-rel-ops", Requester: "ops-user", At: time.Now(),
	   })
	  if err != nil {
		   t.Fatalf("ops release: %v", err)
	   }
	  if proof.FrozenLayer != "ops" {
		   t.Errorf("proof frozen_layer=%q want ops", proof.FrozenLayer)
	  }
   }

// TestRelease_MissingFields 字段缺失校验。

  func TestRelease_MissingFields(t *testing.T) {
	   ctx := context.Background()
	  tokens, protocol, _ := newProtocolStack()
	   tok := placeToken(t, tokens, "u1", "USDT", holdcat.LayerOps, holdcat.ReasonOpsManualHold, "idem-missing-tok")
	   cases := []struct {
		  name    string
		  req     holdrel.ReleaseReq
		   wantErr error
	  }{
		   {
			   name: "missing_idem_key",
			   req: holdrel.ReleaseReq{
				   TokenID: tok.ID, CallerLayer: "ops", ReasonCode: "ops_manual_hold",
				   IdemKey: "", Requester: "user",
			  },
			  wantErr: holdrel.ErrIdemKeyMissing,
		   },
		   {
			  name: "missing_requester",
			  req: holdrel.ReleaseReq{
				  TokenID: tok.ID, CallerLayer: "ops", ReasonCode: "ops_manual_hold",
				   IdemKey: "idem-nouser", Requester: "",
			  },
			   wantErr: holdrel.ErrRequesterMissing,
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   tc.req.At = time.Now()
			  _, err := protocol.RequestRelease(ctx, tc.req)
			  if !errors.Is(err, tc.wantErr) {
				  t.Errorf("want %v got %v", tc.wantErr, err)
			   }
		   })
	   }
   }
