package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/exjob"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

// TestWithdrawStuckRecoverReleasesHoldcat signed 无 hash 卡单经 RecoverStuck 解冻并幂等释 pending_withdraw；live hash 拒绝。
  func TestWithdrawStuckRecoverReleasesHoldcat(t *testing.T) {
	   rt := openMem(t)
	   ctx := context.Background()
	  uid := seedUser(rt)
	  rt.Ledger.SetKYT(kyt.Static{Mode: kyt.Review})
	  holds := holdcat.New(rt.KV)
	  svc := withdraw.Attach(rt, nil).BindHolds(holds)
	  tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("1"), Chain: "ETH", ToAddress: extDest("ETH"),
		   IdempotencyKey: "stuck-wd-1", WaitConfirm: true,
	  })
	  if !errors.Is(err, ledger.ErrKYTReview) || tr == nil {
		   t.Fatalf("create %v tr=%v", err, tr)
	   }
	  if _, err := holds.PlacePendingWithdraw(ctx, uid, "USDT", "1", tr.ID); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := svc.Broadcasts().Track(tr.ID, "ETH", "", "USDT", "1"); err != nil {
		   t.Fatal(err)
	  }
	  row,
		  ok := svc.Broadcasts().Get(tr.ID)
	  if !ok || row.Status != withdraw.BcSigned {
		  t.Fatalf("track %+v ok=%v", row, ok)
	  }
	  st, err := svc.RecoverStuck(tr.ID, row.UpdatedAt.Add(2*time.Hour))
	  if err != nil {
		   t.Fatalf("recover %v st=%+v", err, st)
	   }
	  if st.Action != withdraw.StuckRecover {
		  t.Fatalf("action %+v", st)
	   }
	  open, err := holds.List(ctx, uid, true)
	   if err != nil {
		  t.Fatal(err)
	  }
	   for _, r := range open {
		  if r.Reason == holdcat.ReasonPendingWithdraw && r.Ref == tr.ID && r.Open {
			  t.Fatalf("pending_withdraw still open: %+v", r)
		   }
	  }
	  row2,
		   _ := svc.Broadcasts().Get(tr.ID)
	  if row2.Status != withdraw.BcFailed {
		   t.Fatalf("want failed got %+v", row2)
	  }
	   tr2, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1"), Chain: "ETH", ToAddress: extDest("ETH"),
		  IdempotencyKey: "stuck-wd-live", WaitConfirm: true,
	  })
	   if !errors.Is(err, ledger.ErrKYTReview) || tr2 == nil {
		  t.Fatalf("create2 %v tr=%v", err, tr2)
	  }
	  if _, err := svc.Broadcasts().Track(tr2.ID, "ETH", "0xabc", "USDT", "1"); err != nil {
		  t.Fatal(err)
	   }
	   _, err = svc.RecoverStuck(tr2.ID, time.Now().UTC().Add(3*time.Hour))
	   if err == nil || err.Error() != "live hash forbids auto unfreeze" {
		  t.Fatalf("want live refuse, got %v", err)
	   }
   }

// TestCustodyHaltBlocksDepositObserve Engage 后 RequireDeposit/Observe 失败关闭；BindFundsRecon 清扫不得 Clear。
   func TestCustodyHaltBlocksDepositObserve(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	  uid := seedUser(rt)
	   gate := rt.FundSurf.HaltGate()
	   if err := gate.Engage(ctx, "ops-risk", "hot wallet anomaly fixture"); err != nil {
		   t.Fatal(err)
	   }
	  if err := rt.FundSurf.RequireDeposit(ctx); err == nil || !errors.Is(err, custodyhalt.ErrHalted) {
		   t.Fatalf("RequireDeposit %v", err)
	   }
	   if err := rt.FundSurf.RequireWithdraw(ctx, uid); err == nil || !errors.Is(err, custodyhalt.ErrHalted) {
		  t.Fatalf("RequireWithdraw %v", err)
	   }
	   dep := deposit.New(rt.Ledger, rt.Wallet).BindHalt(gate.Check)
	   _, err := dep.Observe(ctx, deposit.ObserveReq{
		   UserID: uid, Chain: "ETH", TxHash: "0xhalted", Asset: "USDT",
		   IdempotencyKey: "dep-halt-1",
	  })
	   if err == nil || !errors.Is(err, custodyhalt.ErrHalted) {
		  t.Fatalf("Observe under halt %v", err)
	  }
	  kcfund.BindFundsRecon(rt)
	  rt.FundsRecon(ctx)
	  f, err := gate.Snapshot(ctx)
	   if err != nil || !f.On {
		   t.Fatalf("recon must keep halt on: %+v %v", f, err)
	  }
   }

// TestReconDriftBlocksFreeze 未关差异工单时 BlocksFreeze 拦截；结案后放行。

   func TestReconDriftBlocksFreeze(t *testing.T) {
	  rt := openMem(t)
	  uid := seedUser(rt)
	   plat := recon.NewPlatform(rt.Ledger)
	  if err := plat.BlocksFreeze(uid); err != nil {
		  t.Fatalf("clean %v", err)
	  }
	   tk, err := plat.Tickets().Open("ops-recon", uid, "funding", "USDT", "hot vs book drift")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if err := plat.BlocksFreeze(uid); err == nil {
		  t.Fatal("want blocks with open ticket")
	  }
	  as, err := plat.Tickets().Assign(tk.ID, "ops-2", "desk-a", tk.Fence.Epoch, tk.Fence.Token)
	   if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := plat.Tickets().Resolve(as.ID, "ops-3", as.Fence.Epoch, as.Fence.Token); err != nil {
		   t.Fatal(err)
	   }
	  if err := plat.BlocksFreeze(uid); err != nil {
		   t.Fatalf("after resolve %v", err)
	  }
   }

// TestExportWindowAndDailyLimit 缺省近 100 天窗口；超窗拒绝；日限额 5 次（含 KV 跨实例计数）。

   func TestExportWindowAndDailyLimit(t *testing.T) {
	   rt := openMem(t)
	  uid := seedUser(rt)
	   book := exjob.New().BindKV(rt.KV).BindLedger(rt.Ledger)
	   job, err := book.Enqueue(uid, "withdraw", "csv")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if job.StartAt.IsZero() || job.EndAt.IsZero() {
		   t.Fatalf("default window required: %+v", job)
	  }
	  if job.EndAt.Sub(job.StartAt) > 100*24*time.Hour+time.Minute {
		   t.Fatalf("window too wide %+v", job)
	   }
	   end := time.Now().UTC()
	  start := end.Add(-101 * 24 * time.Hour)
	   if _, err := book.EnqueueWindow(uid, "withdraw", "csv", start, end); !errors.Is(err, fundsurf.ErrWindow) {
		  t.Fatalf("want ErrWindow got %v", err)
	   }
	  for i := 0; i < 4; i++ {
		   if _, err := book.Enqueue(uid, "deposit", "csv"); err != nil {
			  t.Fatalf("enqueue %d %v", i, err)
		  }
	  }
	  if _, err := book.Enqueue(uid, "all", "csv"); !errors.Is(err, exjob.ErrDaily) {
		   t.Fatalf("want ErrDaily got %v", err)
	   }
	   book2 := exjob.New().BindKV(rt.KV).BindLedger(rt.Ledger)
	   if _, err := book2.Enqueue(uid, "all", "csv"); !errors.Is(err, exjob.ErrDaily) {
		  t.Fatalf("kv daily %v", err)
	   }
   }

type fakeMemoDep struct {
	bound bool
	tr    *ledger.Transfer
}

   func (f *fakeMemoDep) BindMemo(userID, chain, memo string) error {
	  f.bound = true
	   return nil
   }

   func (f *fakeMemoDep) ObserveCredit(ctx context.Context, userID, chain, txHash, asset, memo, addr string) (*ledger.Transfer, error) {
	  return f.tr, nil
   }

// TestMemoRecoverApproveCredits 错漏 Memo 审批通过后绑定并入账（金额取自观察适配，不取工单自报）。
  func TestMemoRecoverApproveCredits(t *testing.T) {
	   rt := openMem(t)
	  ctx := context.Background()
	  uid := seedUser(rt)
	   tk, err := rt.FundSurf.OpenMemoTicket(ctx, uid, "ETH", "0xmemo1", "tag-1", dest(uid, "ETH"))
	  if err != nil {
		   t.Fatal(err)
	  }
	   fake := &fakeMemoDep{tr: &ledger.Transfer{
		   ID: "memo-tr-1", UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("3"),
		  State: ledger.StateCredited,
	   }}
	  got, err := rt.FundSurf.ApproveMemoCredit(ctx, tk.ID, "USDT", fake)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if got.Status != "approved" || !fake.bound {
		   t.Fatalf("approve %+v bound=%v", got, fake.bound)
	  }
	  if _, err := rt.FundSurf.ApproveMemoCredit(ctx, tk.ID, "USDT", fake); !errors.Is(err, fundsurf.ErrMemoPending) {
		   t.Fatalf("replay want pending err got %v", err)
	  }
  }
