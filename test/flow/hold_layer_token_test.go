package flow_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func newTokenBook() *holdcat.TokenBook {
	   return holdcat.NewTokenBook(persist.NewMemKV())
   }

  func TestTokenBook_Place_Validation(t *testing.T) {
	  ctx := context.Background()
	   book := newTokenBook()
	  cases := []struct {
		   name   string
		   userID string
		   asset  string
		  book2  string
		   amount string
		  layer      holdcat.Layer
		   reasonCode holdcat.ReasonCode
		  idemKey    string
		   wantErr    error
	   }{
		   {
			   name:   "valid_legal",
			  userID: "u1", asset: "USDT", book2: "funding",
			   amount: "1000", layer: holdcat.LayerLegal,
			  reasonCode: holdcat.ReasonCourtOrder, idemKey: "idem-legal-001",
		  },
		   {
			   name:   "valid_aml",
			  userID: "u1", asset: "BTC", book2: "funding",
			  amount: "0.5", layer: holdcat.LayerAML,
			  reasonCode: holdcat.ReasonSuspiciousTx, idemKey: "idem-aml-001",
		   },
		  {
			  name:   "valid_risk",
			  userID: "u2", asset: "ETH", book2: "funding",
			  amount: "10", layer: holdcat.LayerRisk,
			  reasonCode: holdcat.ReasonKYTHighRisk, idemKey: "idem-risk-001",
		   },
		   {
			  name:   "valid_ops",
			  userID: "u3", asset: "USDT", book2: "funding",
			   amount: "500", layer: holdcat.LayerOps,
			   reasonCode: holdcat.ReasonWithdrawReview, idemKey: "idem-ops-001",
		  },
		   {
			  name: "empty_user", userID: "", asset: "USDT",
			  amount: "100", layer: holdcat.LayerOps, reasonCode: holdcat.ReasonOpsManualHold,
			   idemKey: "idem-x1", wantErr: holdcat.ErrUser,
		   },
		  {
			   name: "empty_asset", userID: "u1", asset: "",
			  amount: "100", layer: holdcat.LayerOps, reasonCode: holdcat.ReasonOpsManualHold,
			  idemKey: "idem-x2", wantErr: holdcat.ErrAmount,
		   },
		   {
			   name: "negative_amount", userID: "u1", asset: "USDT",
			  amount: "-1", layer: holdcat.LayerOps, reasonCode: holdcat.ReasonOpsManualHold,
			   idemKey: "idem-x3", wantErr: holdcat.ErrAmount,
		  },
		   {
			   name: "zero_amount", userID: "u1", asset: "USDT",
			  amount: "0", layer: holdcat.LayerOps, reasonCode: holdcat.ReasonOpsManualHold,
			  idemKey: "idem-x4", wantErr: holdcat.ErrAmount,
		   },
		  {
			   name: "unknown_layer", userID: "u1", asset: "USDT",
			   amount: "100", layer: holdcat.LayerUnknown, reasonCode: holdcat.ReasonOpsManualHold,
			  idemKey: "idem-x5", wantErr: holdcat.ErrLayerInvalid,
		   },
		  {
			  name: "wrong_reason_for_layer", userID: "u1", asset: "USDT",
			  amount: "100", layer: holdcat.LayerLegal,
			   reasonCode: holdcat.ReasonWithdrawReview, // ops 原因码用于 legal 层
			  idemKey: "idem-x6", wantErr: holdcat.ErrReasonMismatch,
		   },
		   {
			  name: "empty_idem_key", userID: "u1", asset: "USDT",
			   amount: "100", layer: holdcat.LayerOps, reasonCode: holdcat.ReasonOpsManualHold,
			   idemKey: "", wantErr: holdcat.ErrIdemKeyRequired,
		   },
		  {
			   name: "risk_reason_used_for_aml", userID: "u1", asset: "USDT",
			  amount: "100", layer: holdcat.LayerAML,
			   reasonCode: holdcat.ReasonKYTHighRisk, // risk 原因码用于 aml 层
			  idemKey:    "idem-x7", wantErr: holdcat.ErrReasonMismatch,
		   },
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   tok, err := book.Place(ctx,
				  tc.userID,
				  tc.asset, tc.book2, tc.amount,
				  tc.layer, tc.reasonCode,
				   "", tc.idemKey, "", "",
			  )
			   if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					   t.Errorf("want err %v got %v", tc.wantErr, err)
				  }
				   return
			  }
			   if err != nil {
				  t.Fatalf("unexpected err: %v", err)
			   }
			  if tok.ID == "" {
				  t.Error("token ID empty")
			  }
			  if tok.State != holdcat.TokenStateActive {
				   t.Errorf("state=%v want active", tok.State)
			  }
			  if tok.Layer != tc.layer.Code() {
				  t.Errorf("layer=%q want %q", tok.Layer, tc.layer.Code())
			   }
		   })
	  }
  }

   func TestTokenBook_Idempotent(t *testing.T) {
	  ctx := context.Background()
	   book := newTokenBook()
	  idemKey := "idem-idempotent-001"
	  tok1, err := book.Place(ctx, "u1", "USDT", "funding", "100",
		   holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", idemKey, "", "")
	  if err != nil {
		  t.Fatalf("first place: %v", err)
	  }
	  tok2, err := book.Place(ctx, "u1", "USDT", "funding", "200", // 不同金额
		  holdcat.LayerOps, holdcat.ReasonWithdrawReview, "", idemKey, "", "") // 不同原因码
	   if err != nil {
		  t.Fatalf("second place: %v", err)
	   }
	   if tok1.ID != tok2.ID {
		   t.Errorf("idempotent: got different token IDs %s vs %s", tok1.ID, tok2.ID)
	  }
	  if tok2.Amount != "100" {
		   t.Errorf("idempotent: amount should not change, got %s", tok2.Amount)
	   }
  }

   func TestTokenBook_StateMachine(t *testing.T) {
	   ctx := context.Background()
	  book := newTokenBook()
	  tok, err := book.Place(ctx, "u1", "USDT", "funding", "1000",
		  holdcat.LayerRisk, holdcat.ReasonKYTHighRisk, "ledger-ref-001", "idem-sm-001", "", "")
	   if err != nil {
		  t.Fatalf("place: %v", err)
	   }
	  if tok.State != holdcat.TokenStateActive {
		   t.Fatalf("want active, got %s", tok.State)
	  }
	   // active → releasing
	   tok2, err := book.BeginRelease(ctx, tok.ID)
	  if err != nil {
		  t.Fatalf("begin release: %v", err)
	  }
	   if tok2.State != holdcat.TokenStateReleasing {
		   t.Errorf("want releasing, got %s", tok2.State)
	  }
	   // releasing 幂等
	  tok3, err := book.BeginRelease(ctx, tok.ID)
	   if err != nil {
		  t.Fatalf("begin release idempotent: %v", err)
	   }
	   if tok3.State != holdcat.TokenStateReleasing {
		   t.Errorf("idempotent: want releasing, got %s", tok3.State)
	  }
	  // releasing → released
	   tok4, err := book.CommitRelease(ctx, tok.ID)
	  if err != nil {
		  t.Fatalf("commit release: %v", err)
	  }
	  if tok4.State != holdcat.TokenStateReleased {
		  t.Errorf("want released, got %s", tok4.State)
	  }
	   if tok4.ReleasedAt == "" {
		   t.Error("released_at should be set")
	   }
	  // released 再次 commit 幂等
	   tok5, err := book.CommitRelease(ctx, tok.ID)
	   if err != nil {
		   t.Fatalf("commit released idempotent: %v", err)
	   }
	  if tok5.State != holdcat.TokenStateReleased {
		  t.Errorf("want released, got %s", tok5.State)
	   }
  }

  func TestTokenBook_MarkBroken(t *testing.T) {
	   ctx := context.Background()
	   book := newTokenBook()
	  tok, _ := book.Place(ctx, "u1", "USDT", "funding", "500",
		  holdcat.LayerAML, holdcat.ReasonSuspiciousTx, "", "idem-broken-001", "", "")
	  // active → broken
	  tok2, err := book.MarkBroken(ctx, tok.ID, "drift detected by patrol")
	   if err != nil {
		  t.Fatalf("mark broken: %v", err)
	   }
	   if tok2.State != holdcat.TokenStateBroken {
		  t.Errorf("want broken, got %s", tok2.State)
	   }
	  if tok2.BrokenAt == "" {
		  t.Error("broken_at should be set")
	  }
	  // broken 幂等
	   tok3, err := book.MarkBroken(ctx, tok.ID, "again")
	   if err != nil {
		  t.Fatalf("mark broken idempotent: %v", err)
	   }
	  if tok3.State != holdcat.TokenStateBroken {
		   t.Error("should remain broken")
	   }
   }

  func TestTokenBook_StateMachine_BeginRelease_NotActive(t *testing.T) {
	  ctx := context.Background()
	   book := newTokenBook()
	  tok, _ := book.Place(ctx, "u1", "USDT", "funding", "100",
		   holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", "idem-notactive-001", "", "")
	  // 先标 broken
	  _, _ = book.MarkBroken(ctx, tok.ID, "test")
	   // broken 状态 BeginRelease 应报错
	  _,
		   err := book.BeginRelease(ctx, tok.ID)
	   if !errors.Is(err, holdcat.ErrTokenNotActive) {
		  t.Errorf("want ErrTokenNotActive, got %v", err)
	  }
  }

   func TestTokenBook_HasHigherLayerToken(t *testing.T) {
	   ctx := context.Background()
	   book := newTokenBook()
	   // 加入 legal 层 token
	  _, err := book.Place(ctx, "u1", "USDT", "funding", "1000",
		   holdcat.LayerLegal, holdcat.ReasonCourtOrder, "", "idem-legal-high-001", "", "")
	   if err != nil {
		   t.Fatalf("place legal: %v", err)
	   }
	  cases := []struct {
		   name        string
		   callerLayer holdcat.Layer
		  wantBlocked bool
	  }{
		   {"ops_blocked_by_legal", holdcat.LayerOps, true},
		   {"risk_blocked_by_legal", holdcat.LayerRisk, true},
		   {"aml_blocked_by_legal", holdcat.LayerAML, true},
		  {"legal_not_blocked", holdcat.LayerLegal, false}, // legal 可释放 legal
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   blocked, highest, err := book.HasHigherLayerToken(ctx, "u1", "USDT", "funding", tc.callerLayer)
			   if err != nil {
				   t.Fatalf("HasHigherLayerToken: %v", err)
			  }
			  if blocked != tc.wantBlocked {
				   t.Errorf("blocked=%v want %v (highest=%s)", blocked, tc.wantBlocked, highest)
			   }
		  })
	   }
   }

   func TestTokenBook_ActiveLayersForUser(t *testing.T) {
	   ctx := context.Background()
	  book := newTokenBook()
	   // 加入 aml 和 risk 两层
	   _, _ = book.Place(ctx, "u1", "USDT", "funding", "1000",
		  holdcat.LayerAML, holdcat.ReasonSuspiciousTx, "", "idem-multi-aml", "", "")
	  _, _ = book.Place(ctx, "u1", "USDT", "funding", "500",
		   holdcat.LayerRisk, holdcat.ReasonKYTHighRisk, "", "idem-multi-risk", "", "")
	  // 加入另一用户的 legal（不应混入）
	  _, _ = book.Place(ctx, "u2", "USDT", "funding", "200",
		  holdcat.LayerLegal, holdcat.ReasonCourtOrder, "", "idem-multi-u2", "", "")
	  layers, err := book.ActiveLayersForUser(ctx, "u1", "USDT")
	  if err != nil {
		  t.Fatalf("ActiveLayersForUser: %v", err)
	  }
	   if len(layers) != 2 {
		   t.Errorf("want 2 layers, got %d: %v", len(layers), layers)
	  }
	  seen := map[holdcat.Layer]bool{}
	  for _, l := range layers {
		   seen[l] = true
	  }
	  if !seen[holdcat.LayerAML] || !seen[holdcat.LayerRisk] {
		   t.Errorf("want aml+risk, got %v", layers)
	   }
  }

   func TestTokenBook_List_ActiveOnly(t *testing.T) {
	   ctx := context.Background()
	   book := newTokenBook()
	  // 放入 3 个 token，释放 1 个
	   tok1, _ := book.Place(ctx, "u1", "USDT", "funding", "100",
		  holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", "idem-list-001", "", "")
	  tok2, _ := book.Place(ctx, "u1", "USDT", "funding", "200",
		   holdcat.LayerOps, holdcat.ReasonPromoHold, "", "idem-list-002", "", "")
	   _, _ = book.Place(ctx, "u1", "USDT", "funding", "300",
		  holdcat.LayerRisk, holdcat.ReasonKYCPending, "", "idem-list-003", "", "")
	  // 释放 tok1
	  _, _ = book.BeginRelease(ctx, tok1.ID)
	  _, _ = book.CommitRelease(ctx, tok1.ID)
	   // activeOnly=true 应返回 2 个（tok2 + tok3）
	  toks, err := book.List(ctx, "u1", true)
	  if err != nil {
		  t.Fatalf("list: %v", err)
	  }
	  if len(toks) != 2 {
		  t.Errorf("want 2 active tokens, got %d", len(toks))
	  }
	   for _, t2 := range toks {
		   if t2.ID == tok2.ID {
			  continue
		   }
		   // tok3 也应在其中
	  }
	   // activeOnly=false 应返回 3 个
	   all, err := book.List(ctx, "u1", false)
	   if err != nil {
		  t.Fatalf("list all: %v", err)
	   }
	   if len(all) != 3 {
		  t.Errorf("want 3 total tokens, got %d", len(all))
	  }
   }
