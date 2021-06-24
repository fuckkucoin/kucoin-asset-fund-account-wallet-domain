package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/destlimit"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/wdpath"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

const (
	destA = "0x1111111111111111111111111111111111111111"
	destB = "0x2222222222222222222222222222222222222222"
)

   func flatPolicy(id, max string) destlimit.Policy {
	  return destlimit.Policy{
		   ID: id, Name: id, Asset: "USDT",
		   ClusterMax: max, UserMax: "100000", DestMax: "100000",
		   WindowSec: 86400,
		  LowMul: "1", MidMul: "1", HighMul: "1", SevereMul: "1",
	  }
  }

   func allowMem(t *testing.T, st ledger.Store, uid, addr string) {
	  t.Helper()
	   mem, ok := st.(*ledger.MemStore)
	   if !ok {
		  t.Fatalf("store %T", st)
	   }
	  mem.Allow(uid, "ETH", addr)
	  mem.Allow(uid, "EVM", addr)
   }

  func bindWithdraw(t *testing.T, rtLedger *ledger.Service, eng *destlimit.Engine) *withdraw.Service {
	  t.Helper()
	  wd := withdraw.New(rtLedger)
	   wd.BindLimitAdmitter(limitAdmitter{eng: eng})
	   return wd
  }

type limitAdmitter struct{ eng *destlimit.Engine }

   func (a limitAdmitter) Admit(ctx context.Context, in withdraw.LimitAdmit) (withdraw.LimitVerdict, error) {
	   h, err := a.eng.Admit(ctx, destlimit.AdmitReq{
		  UserID: in.UserID, Asset: in.Asset, Amount: in.Amount,
		   Chain: in.Chain, Dest: in.Dest, Idem: in.Idem, KYTHint: in.KYTHint,
	  })
	   if err != nil {
		  return withdraw.LimitVerdict{},
			  err
	  }
	   return withdraw.LimitVerdict{HoldID: h.ID, ClusterID: h.ClusterID, Tier: h.Tier, Limit: h.Limit, Reused: h.Reused}, nil
   }

   func (a limitAdmitter) ReleaseAdmit(ctx context.Context, holdID, reason string) error {
	  return a.eng.ReleaseAdmit(ctx,
		  holdID, reason)
  }

  func TestSameClusterSecondWithdrawDenied(t *testing.T) {
	   rt := openMem(t)
	   uid := seedUser(rt)
	  allowMem(t, rt.Store, uid, destA)
	   allowMem(t, rt.Store, uid, destB)
	  eng := destlimit.New()
	   if _, err := eng.PutPolicy(flatPolicy("pol-cluster", "100")); err != nil {
		   t.Fatal(err)
	   }
	  if _, err := eng.Graph().Link("ETH", destA, "ETH", destB, "0.40", "explicit", "same desk", time.Hour); err != nil {
		  t.Fatal(err)
	  }
	  wd := bindWithdraw(t,
		   rt.Ledger, eng)
	  ctx := context.Background()
	   first, err := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("60"),
		   Chain: "ETH", ToAddress: destA, IdempotencyKey: "dl-cluster-1",
	   })
	   if err != nil || first == nil {
		   t.Fatalf("first request: %v tr=%v", err, first)
	  }
	   holds := eng.Holds(uid, "USDT", 20)
	  if len(holds) != 1 || holds[0].State != destlimit.HoldReserved {
		  t.Fatalf("holds %+v", holds)
	   }
	  second, err := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("50"),
		  Chain: "ETH", ToAddress: destB, IdempotencyKey: "dl-cluster-2",
	   })
	  if second != nil {
		   t.Fatalf("second transfer created: %+v", second)
	  }
	  if !errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("second err %v", err)
	  }
	   var de *destlimit.DenyError
	  if !errors.As(err, &de) || de.Code != destlimit.CodeCluster {
		   t.Fatalf("deny %+v err %v", de, err)
	   }
	  if errors.Is(err, kyt.ErrDenied) || errors.Is(err, ledger.ErrKYTDenied) {
		  t.Fatalf("limit deny swallowed as kyt: %v", err)
	   }
	   again := eng.Holds(uid, "USDT", 20)
	   if len(again) != 1 {
		  t.Fatalf("hold leaked or missing: %+v", again)
	  }
   }

   func TestEdgeDecayRestoresLimitOnlyAfterThreshold(t *testing.T) {
	  eng := destlimit.New()
	   start := time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	   cur := start
	  eng.WithClock(func() time.Time { return cur })
	  if _, err := eng.PutPolicy(destlimit.Policy{
		  ID: "pol-decay", Name: "decay", Asset: "USDT",
		  ClusterMax: "1000", UserMax: "1000", DestMax: "1000",
		  WindowSec: 86400,
		   LowMul: "1", MidMul: "0.2", HighMul: "0.2", SevereMul: "0.2",
	  }); err != nil {
		  t.Fatal(err)
	   }
	  edge, err := eng.Graph().Link("ETH", destA, "ETH", destB, "0.80", "explicit", "hot link", time.Hour)
	  if err != nil {
		  t.Fatal(err)
	  }
	   ctx := context.Background()
	  try := func() error {
		  _, err := eng.Admit(ctx, destlimit.AdmitReq{
			  UserID: "user-10001", Asset: "USDT", Amount: "300",
			   Chain: "ETH", Dest: destA, Idem: "decay-" + cur.Format("150405"),
		  })
		   return err
	   }
	   if err := try(); !errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("fresh weight should deny, err=%v", err)
	  }
	  cur = start.Add(30 * time.Minute)
	   if _, err := eng.Tick(cur); err != nil {
		  t.Fatal(err)
	  }
	  if err := try(); !errors.Is(err, destlimit.ErrLimitDeny) {
		   t.Fatalf("before half-life should still deny, err=%v", err)
	   }
	  cur = start.Add(time.Hour)
	   if _, err := eng.Tick(cur); err != nil {
		  t.Fatal(err)
	  }
	  if err := try(); !errors.Is(err, destlimit.ErrLimitDeny) {
		   t.Fatalf("one half-life still above low band, err=%v", err)
	   }
	   cur = start.Add(2 * time.Hour)
	  tick, err := eng.Tick(cur)
	   if err != nil {
		  t.Fatal(err)
	  }
	  if tick.Decayed < 1 {
		   t.Fatalf("expected decay tick %+v", tick)
	  }
	   if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		  UserID: "user-10001", Asset: "USDT", Amount: "300",
		   Chain: "ETH", Dest: destA, Idem: "decay-allow",
	  }); err != nil {
		  t.Fatalf("after threshold: %v", err)
	   }
	  audits := eng.Graph().Audits("edge_decay", 20)
	  if len(audits) == 0 {
		   t.Fatal("decay not audited")
	   }
	  saw := false
	   for _, a := range audits {
		  if a.Subject == edge.ID && a.Before != "" && a.After != "" && a.Before != a.After {
			   saw = true
		  }
	   }
	   if !saw {
		  t.Fatalf("missing weight transition audits %+v", audits)
	   }
   }

  func TestDeskOverrideDualControlExpires(t *testing.T) {
	  eng := destlimit.New().SetHoldTTL(24 * time.Hour)
	   start := time.Date(2022, 6, 2, 8, 0, 0, 0, time.UTC)
	   cur := start
	  eng.WithClock(func() time.Time { return cur })
	   if _, err := eng.PutPolicy(flatPolicy("pol-ov", "100")); err != nil {
		  t.Fatal(err)
	   }
	  ctx := context.Background()
	   if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		  UserID: "user-10001", Asset: "USDT", Amount: "80",
		   Chain: "ETH", Dest: destA, Idem: "ov-base",
	  }); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: "user-10001", Asset: "USDT", Amount: "30",
		  Chain: "ETH", Dest: destA, Idem: "ov-before",
	   }); !errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("before override: %v", err)
	   }
	   ov, err := eng.ProposeOverride("user-10001", "USDT", "", "50", "ops-desk-a", "desk lift", time.Hour)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if ov.State != destlimit.OvPending {
		   t.Fatalf("state %s", ov.State)
	   }
	   if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: "user-10001", Asset: "USDT", Amount: "30",
		   Chain: "ETH", Dest: destA, Idem: "ov-pending",
	  }); !errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("pending must not lift limit: %v", err)
	   }
	  if _, err := eng.ConfirmOverride(ov.ID, "ops-desk-a"); !errors.Is(err, destlimit.ErrDual) {
		  t.Fatalf("same actor confirm: %v", err)
	  }
	   got, err := eng.ConfirmOverride(ov.ID, "ops-desk-b")
	  if err != nil {
		  t.Fatal(err)
	   }
	  if got.State != destlimit.OvActive || got.ActorB != "ops-desk-b" {
		   t.Fatalf("confirmed %+v", got)
	   }
	  if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		  UserID: "user-10001", Asset: "USDT", Amount: "30",
		   Chain: "ETH", Dest: destA, Idem: "ov-live",
	  }); err != nil {
		  t.Fatalf("active override: %v", err)
	   }
	   cur = start.Add(2 * time.Hour)
	  if _, err := eng.Tick(cur); err != nil {
		  t.Fatal(err)
	  }
	   view, err := eng.GetOverride(ov.ID)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if view.State != destlimit.OvExpired {
		   t.Fatalf("override still %s", view.State)
	  }
	   if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: "user-10001", Asset: "USDT", Amount: "30",
		  Chain: "ETH", Dest: destA, Idem: "ov-after",
	   }); !errors.Is(err, destlimit.ErrLimitDeny) {
		   t.Fatalf("expired override still lifting: %v", err)
	   }
	  audits := eng.Audits("", 30)
	   need := map[string]bool{destlimit.AuditPropose: false, destlimit.AuditConfirm: false, destlimit.AuditOvExpire: false}
	   for _, a := range audits {
		   if _, ok := need[a.Kind]; ok {
			   need[a.Kind] = true
		   }
	  }
	  for k, ok := range need {
		   if !ok {
			  t.Fatalf("missing audit %s in %+v", k, audits)
		  }
	  }
   }

  func TestKYTDenyDistinctFromLimitDeny(t *testing.T) {
	   if destlimit.Classify(kyt.ErrDenied) != destlimit.CodeKYTDeny {
		   t.Fatal("kyt class")
	  }
	   if destlimit.Classify(&destlimit.DenyError{Code: destlimit.CodeCluster, Reason: "over"}) != destlimit.CodeLimitDeny {
		   t.Fatal("limit class")
	  }
	   if errors.Is(kyt.ErrDenied, destlimit.ErrLimitDeny) || errors.Is(destlimit.ErrLimitDeny, kyt.ErrDenied) {
		   t.Fatal("sentinels overlap")
	   }
	   rt := openMem(t)
	   uid := seedUser(rt)
	  ctx := context.Background()
	   deniedPipe := compliance.New(kyt.Static{Mode: kyt.Deny},
		   policy.NewRegistry(policy.DefaultV1()))
	   wdKYT := withdraw.NewWith(rt.Ledger, deniedPipe)
	  eng := destlimit.New()
	   if _, err := eng.PutPolicy(flatPolicy("pol-kyt", "100000")); err != nil {
		  t.Fatal(err)
	  }
	  wdKYT.BindLimitAdmitter(limitAdmitter{eng: eng})
	  _, err := wdKYT.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("10"),
		   Chain: "ETH", ToAddress: destA, IdempotencyKey: "dl-kyt-deny",
	   })
	  if !errors.Is(err, ledger.ErrRiskRejected) {
		  t.Fatalf("kyt path err %v", err)
	   }
	   if errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("kyt deny swallowed as limit: %v", err)
	   }
	  for _, h := range eng.Holds(uid, "USDT", 10) {
		   if h.State == destlimit.HoldReserved || h.State == destlimit.HoldConsumed {
			  t.Fatalf("kyt deny left occupancy %+v", h)
		   }
	   }
	   wdLim := bindWithdraw(t, rt.Ledger, eng)
	   if _, err := eng.PutPolicy(flatPolicy("pol-tight", "5")); err != nil {
		  t.Fatal(err)
	   }
	   _, err = wdLim.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("10"),
		  Chain: "ETH", ToAddress: destB, IdempotencyKey: "dl-limit-deny",
	   })
	   if !errors.Is(err, destlimit.ErrLimitDeny) {
		   t.Fatalf("limit path err %v", err)
	  }
	  if errors.Is(err, kyt.ErrDenied) || errors.Is(err, ledger.ErrKYTDenied) || errors.Is(err, ledger.ErrRiskRejected) {
		   t.Fatalf("limit deny swallowed: %v", err)
	  }
	  path := wdpath.New()
	  path.BindTravelBook(travelvasp.NewThresholdBook("100000000", nil))
	  if _, err := path.Allow().AddStrict(uid, "ETH", destA, "book"); err != nil {
		  t.Fatal(err)
	   }
	   path.BindKYT(kytsvc.New(kyt.Static{Mode: kyt.Deny}))
	   path.BindLimit(pathGate{eng: eng})
	  _, err = path.Open(ctx, wdpath.OpenReq{
		  UserID: uid, Chain: "ETH", Dest: destA, Asset: "USDT", Amount: "10",
		   Actor: "ops-desk-a", IdempotencyKey: "path-kyt",
	   })
	   if !errors.Is(err, wdpath.ErrKYT) || errors.Is(err, wdpath.ErrLimit) {
		   t.Fatalf("path kyt err %v", err)
	  }
	  path2 := wdpath.New()
	   path2.BindTravelBook(travelvasp.NewThresholdBook("100000000", nil))
	  if _, err := path2.Allow().AddStrict(uid, "ETH", destA, "book"); err != nil {
		   t.Fatal(err)
	   }
	  path2.BindKYT(kytsvc.New(kyt.Static{Mode: kyt.Allow}))
	  tight := destlimit.New()
	   if _, err := tight.PutPolicy(flatPolicy("pol-path", "1")); err != nil {
		  t.Fatal(err)
	   }
	  path2.BindLimit(pathGate{eng: tight})
	  _, err = path2.Open(ctx, wdpath.OpenReq{
		   UserID: uid, Chain: "ETH", Dest: destA, Asset: "USDT", Amount: "10",
		   Actor: "ops-desk-a", IdempotencyKey: "path-limit",
	  })
	  if !errors.Is(err, wdpath.ErrLimit) || errors.Is(err, wdpath.ErrKYT) || errors.Is(err, kyt.ErrDenied) {
		  t.Fatalf("path limit err %v", err)
	  }
	  if !errors.Is(err, destlimit.ErrLimitDeny) {
		  t.Fatalf("path limit lost destlimit code: %v", err)
	  }
  }

type pathGate struct{ eng *destlimit.Engine }

   func (p pathGate) AdmitPath(ctx context.Context, in wdpath.LimitCheck) error {
	   _, err := p.eng.Admit(ctx, destlimit.AdmitReq{
		  UserID: in.UserID, Asset: in.Asset, Amount: in.Amount,
		   Chain: in.Chain, Dest: in.Dest, Idem: in.Idem, KYTHint: in.KYTHint,
	   })
	   return err
  }

   func TestKYTFeatureTightensLimitWithoutKYTError(t *testing.T) {
	   eng := destlimit.New()
	  if _, err := eng.PutPolicy(destlimit.Policy{
		   ID: "pol-feat", Name: "feat", Asset: "USDT",
		   ClusterMax: "1000", UserMax: "1000", DestMax: "1000",
		   WindowSec: 86400,
		   LowMul:    "1", MidMul: "1", HighMul: "0.2", SevereMul: "0.2",
	   }); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := eng.Graph().Link("ETH", destA, "ETH", destB, "0.40", "explicit", "feature", time.Hour); err != nil {
		   t.Fatal(err)
	   }
	  ctx := context.Background()
	  if _, err := eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: "user-10001", Asset: "USDT", Amount: "300",
		   Chain: "ETH", Dest: destA, Idem: "feat-ok", KYTHint: "allow",
	   }); err != nil {
		  t.Fatalf("allow hint: %v", err)
	  }
	  _, err := eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: "user-20002", Asset: "USDT", Amount: "300",
		   Chain: "ETH", Dest: destB, Idem: "feat-deny", KYTHint: "deny",
	  })
	  if !errors.Is(err, destlimit.ErrLimitDeny) || errors.Is(err, kyt.ErrDenied) {
		  t.Fatalf("feature tighten err %v", err)
	   }
  }
