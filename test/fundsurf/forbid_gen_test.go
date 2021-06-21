package fundsurf_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/forbidgate"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/persist"
)

// newTestBook 构造带 forbidGate 的 Book。
   func newTestBook() *fundsurf.Book {
	   kv := persist.NewMemKV()
	  b := fundsurf.New(kv, nil, nil)
	   gate := forbidgate.NewGate(kv, nil)
	  drift := forbidgate.NewDriftService(nil)
	   b.BindForbidGate(gate, drift)
	   return b
   }

// TestSetForbiddenWithGen_BumpsGen SetForbiddenWithGen 抬升世代。
   func TestSetForbiddenWithGen_BumpsGen(t *testing.T) {
	  b := newTestBook()
	   ctx := context.Background()
	   userID := "fs_user_001"
	   if err := b.SetForbiddenWithGen(ctx, userID, true, "ops", "test"); err != nil {
		   t.Fatalf("SetForbiddenWithGen: %v", err)
	   }
	   view, err := b.ForbidGenView(ctx, userID)
	  if err != nil {
		   t.Fatalf("ForbidGenView: %v", err)
	  }
	   if view.Generation != 1 {
		   t.Errorf("expected gen=1, got=%d", view.Generation)
	   }
	   if view.State != forbidgate.ForbidStateOn {
		  t.Errorf("expected state=on, got=%s", view.State)
	   }
  }

	  }
	  _, err := b.RequireWithdrawWithBind(ctx, userID, "wd_ref_001")
	  if err == nil {
		  t.Fatal("expected ErrForbid for forbidden user")
	  }
	  if err != fundsurf.ErrForbid {
		   t.Errorf("expected ErrForbid, got: %v", err)
	  }
  }

// TestRequireWithdrawWithBind_IssuesBind 非禁提用户 RequireWithdrawWithBind 返回有效绑定 ID。
   func TestRequireWithdrawWithBind_IssuesBind(t *testing.T) {
	  b := newTestBook()
	  ctx := context.Background()
	  userID := "fs_user_bind_ok"
	  bindID, err := b.RequireWithdrawWithBind(ctx, userID, "wd_ref_bind")
	  if err != nil {
		   t.Fatalf("RequireWithdrawWithBind: %v", err)
	   }
	  if bindID == "" {
		   t.Error("expected non-empty bindID")
	  }
  }

// TestForbidGenView_NoGate 未装配 forbidGate 时退化为读原始标志。

   func TestForbidGenView_NoGate(t *testing.T) {
	  kv := persist.NewMemKV()
	  b := fundsurf.New(kv, nil, nil)
	  ctx := context.Background()
	   userID := "fs_user_nogate"
	  view, err := b.ForbidGenView(ctx, userID)
	   if err != nil {
		   t.Fatalf("ForbidGenView (no gate): %v", err)
	   }
	   if view.Generation != 0 {
		  t.Errorf("expected gen=0 (no gate), got=%d", view.Generation)
	  }
	  if view.State != forbidgate.ForbidStateOff {
		   t.Errorf("expected state=off (no gate), got=%s", view.State)
	  }
   }

// TestCheckAndConsumePrefightBind_NoGate 未装配 forbidGate 时返回 ErrStore。
  func TestCheckAndConsumePrefightBind_NoGate(t *testing.T) {
	  kv := persist.NewMemKV()
	  b := fundsurf.New(kv, nil, nil)
	   err := b.CheckAndConsumePrefightBind(context.Background(), "some-bind-id", "user_x")
	  if err != fundsurf.ErrStore {
		   t.Errorf("expected ErrStore, got: %v", err)
	  }
   }

// TestIssuePrefightBind_NoGate 未装配 forbidGate 时返回 ErrStore。

  func TestIssuePrefightBind_NoGate(t *testing.T) {
	   kv := persist.NewMemKV()
	   b := fundsurf.New(kv, nil, nil)
	  _, err := b.IssuePrefightBind(context.Background(), "user_x", "wd_ref")
	  if err != fundsurf.ErrStore {
		   t.Errorf("expected ErrStore, got: %v", err)
	   }
  }

// TestSetForbiddenWithGen_EmptyUser 空 userID 应返回 ErrUser。

   func TestSetForbiddenWithGen_EmptyUser(t *testing.T) {
	   b := newTestBook()
	  err := b.SetForbiddenWithGen(context.Background(), "", true, "ops", "reason")
	   if err != fundsurf.ErrUser {
		  t.Errorf("expected ErrUser, got: %v", err)
	  }
   }
