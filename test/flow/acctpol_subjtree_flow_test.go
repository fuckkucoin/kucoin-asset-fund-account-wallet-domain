package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/acctpol"
	"account-assets.kucoin.net/funding/internal/subjtree"
)

   func TestAcctPolDefaultV1AllowsLegalHops(t *testing.T) {
	  ctx := context.Background()
	   store := acctpol.NewStore(nil)
	  v1 := acctpol.DefaultV1("test-actor")
	   if err := store.Publish(ctx, v1, "test-actor"); err != nil {
		  t.Fatalf("publish v1: %v", err)
	   }
	   eng := acctpol.NewEngine(store)
	  legalHops := []struct {
		   biz  acctpol.BizKind
		   from string
		  to string
	   }{
		  {acctpol.BizDeposit, "bridge", "wallet"},
		   {acctpol.BizWithdraw, "wallet", "bridge"},
		   {acctpol.BizSpotToWallet, "spot", "wallet"},
		   {acctpol.BizWalletToSpot, "wallet", "spot"},
		  {acctpol.BizEarnSubscribe, "spot", "earn"},
		  {acctpol.BizEarnRedeem, "earn", "spot"},
	  }
	  for _, c := range legalHops {
		  res, err := eng.Eval(ctx, acctpol.EvalRequest{
			   BizKind: c.biz,
			  HopFrom: c.from,
			   HopTo: c.to,
		   })
		   if err != nil {
			  t.Fatalf("eval %s %s→%s: %v", c.biz, c.from, c.to, err)
		   }
		  if !res.Allow {
			  t.Fatalf("expected allow for %s %s→%s, got denied: %s", c.biz, c.from, c.to, res.Reason)
		   }
	  }
  }

   func TestAcctPolBlocksDisallowedHop(t *testing.T) {
	   ctx := context.Background()
	   store := acctpol.NewStore(nil)
	   v1 := acctpol.DefaultV1("test-actor")
	   if err := store.Publish(ctx, v1, "test-actor"); err != nil {
		   t.Fatalf("publish v1: %v", err)
	   }
	  eng := acctpol.NewEngine(store)
	   // bridge→wallet for withdraw 不在策略内（withdraw 只允许 wallet→bridge）
	   res, err := eng.Eval(ctx, acctpol.EvalRequest{
		   BizKind: acctpol.BizWithdraw,
		   HopFrom: "bridge",
		   HopTo: "wallet",
	  })
	  if err != nil {
		  t.Fatalf("eval: %v", err)
	   }
	  if res.Allow {
		  t.Fatal("expected hop to be blocked, but got allowed")
	  }
   }

  func TestAcctPolVersionNotFoundBeforePublish(t *testing.T) {
	  ctx := context.Background()
	   store := acctpol.NewStore(nil)
	   eng := acctpol.NewEngine(store)
	  _, err := eng.Eval(ctx, acctpol.EvalRequest{
		   BizKind: acctpol.BizDeposit,
		   HopFrom: "bridge",
		  HopTo:   "wallet",
	  })
	  if err == nil {
		  t.Fatal("expected error when no policy version published")
	   }
  }

  func TestAcctPolVersionDuplicatePublishRejected(t *testing.T) {
	   ctx := context.Background()
	   store := acctpol.NewStore(nil)
	   v1 := acctpol.DefaultV1("actor-a")
	   if err := store.Publish(ctx, v1, "actor-a"); err != nil {
		   t.Fatalf("first publish: %v", err)
	   }
	  err := store.Publish(ctx, v1, "actor-b")
	   if !errors.Is(err, acctpol.ErrPolicyVersionExists) {
		   t.Fatalf("expected ErrPolicyVersionExists, got %v", err)
	   }
   }

   func TestSubjectTreeSystemNodeReadOnly(t *testing.T) {
	   ctx := context.Background()
	   st := subjtree.NewStore(nil)
	   v1 := subjtree.TreeVersion{
		  Version: 1,
		  PublishedAt: time.Now().UTC(),
		  Nodes: []subjtree.Node{
			   {ID: "wallet", Kind: subjtree.KindAsset, Label: "链上钱包", IsSystem: true},
			   {ID: "spot", Kind: subjtree.KindAsset, Label: "现货账户", IsSystem: true},
			   {ID: "bridge", Kind: subjtree.KindBridge, Label: "内部桥", IsSystem: true},
		   },
		  Edges: []subjtree.Edge{
			  {From: "wallet", To: "bridge"},
			   {From: "bridge", To: "spot"},
		  },
	  }
	   if err := st.Publish(ctx, v1, "seed-actor"); err != nil {
		  t.Fatalf("publish v1: %v", err)
	  }
	  // 系统科目不得删除或重命名（通过 CloseNode 应拒绝）
	   _, err := st.CloseNode(ctx, "wallet", "ops-actor")
	  if !errors.Is(err, subjtree.ErrSystemSubject) && !errors.Is(err, subjtree.ErrUnclosable) {
		   t.Fatalf("expected system node to be unclosable, got %v", err)
	   }
  }

   func TestSubjectTreeDuplicateVersionRejected(t *testing.T) {
	  ctx := context.Background()
	   st := subjtree.NewStore(nil)
	  v1 := subjtree.TreeVersion{
		   Version: 1,
		   PublishedAt: time.Now().UTC(),
		   Nodes: []subjtree.Node{
			  {ID: "wallet", Kind: subjtree.KindAsset, Label: "链上钱包", IsSystem: true},
		   },
	  }
	   if err := st.Publish(ctx, v1, "actor-a"); err != nil {
		  t.Fatalf("first publish: %v", err)
	   }
	   err := st.Publish(ctx, v1, "actor-b")
	  if !errors.Is(err, subjtree.ErrVersionExists) {
		  t.Fatalf("expected ErrVersionExists, got %v", err)
	  }
   }
