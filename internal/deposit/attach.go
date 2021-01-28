package deposit

import (
	"context"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/depositrails"
	"account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// Attach 挂入金归因 SQL、FamMEMO 共享扫链钩子到 Runtime。
// BindSQL；OpenMem 不绑；SharedMemo 由 address 复用。
// 扫链无备注/未绑归因跳过，不得猜用户。

   func Attach(rt *app.Runtime, svc *Service) *Service {
	  if rt == nil {
		   return svc
	  }
	  if existing := FromRuntime(rt); existing != nil {
		  wireDesk(existing, rt)
		   return existing
	   }
	  if svc == nil {
		   svc = New(rt.Ledger, rt.Wallet)
	  }
	   wireDesk(svc, rt)
	   rt.Deposit = svc
	  return svc
   }

// wireDesk 挂入金异常台；SQL Hydrate 装回 Asset；错误写入 Health/指标。
// BindSQL+Hydrate；OpenMem 仅进程内；一致 wd-track Attach。
// Hydrate 失败不得假装无单。
   func wireDesk(svc *Service, rt *app.Runtime) {
	  if svc == nil || rt == nil {
		  return
	   }
	  if rt.FundSurf != nil {
		   gate := rt.FundSurf.HaltGate()
		  svc.BindHalt(func(ctx context.Context) error {
			   if err := gate.EntryCheck(ctx); err != nil {
				   return err
			   }
			   return gate.CheckActive(ctx)
		   })
		   rt.FundSurf.BindDepReader(func(_,
			  chain, hash string, output int) (fundsurf.DepRead, bool) {
			   face, err := svc.ReadFace(chain, hash, output)
			   if err != nil {
				  return fundsurf.DepRead{}, false
			   }
			   return fundsurf.DepRead{
				  Phase: face.Phase, PhaseLabel: face.PhaseLabel,
				   Confirming: face.Confirming, Reversed: face.Reversed,
				  Explain:     face.Explain,
				  AvailBefore: face.AvailBefore, AvailAfter: face.AvailAfter,
				  ReverseEntry: face.ReverseEntry, Amount: face.Amount, Asset: face.Asset,
				   Chain: face.Chain, TxHash: face.TxHash,
			   }, true
		   })
	   }
	  // 资金域统一熔断总线：入口只读闸，与 dep: 刷充熔断并存（键空间 fc:）。
	  // 入金刷充熔断改用总线同一张占用表，dep: 键形与判定不变，总线只读该表取速度。
	  bus := fundcircuit.Attach(rt)
	   if use := bus.Usage(); use != nil {
		  if circuit := svc.Circuit(); circuit != nil {
			   circuit.BindUsage(use)
		  }
	  }
	  svc.BindFundCircuit(bus.DepositGate())
	  svc.BindRecon(rt.Durable.Recon)
	  desk := svc.Desk()
	  if desk == nil {
		   desk = NewDesk()
		  svc.BindDesk(desk)
	   }
	   if !rt.Mem && rt.SQL != nil {
		  desk.BindSQL(rt.SQL)
		   // Hydrate 错误保留在 Desk.Health，供 ops 探活与指标（不得吞没）。
		  _ = desk.Hydrate(context.Background())
		  EmitDeskHydrateMetrics(desk.Health())
		  if book := svc.Runs(); book != nil {
			   book.BindSQL(rt.SQL)
			  _ = book.Hydrate(context.Background())
		  }
	  }
	  attr := svc.Attributor()
	   if !rt.Mem && rt.SQL != nil {
		   attr.BindSQL(rt.SQL)
	  }
	  rt.DepositAttr = attr
	   var shared *addrpool.SharedMemoPool
	   if rt.SharedMemo != nil {
		   shared, _ = rt.SharedMemo.(*addrpool.SharedMemoPool)
	  }
	   if shared == nil {
		  shared = addrpool.NewSharedMemoPool(nil)
		   if !rt.Mem && rt.SQL != nil {
			   shared.BindSQL(rt.SQL)
		  }
		  rt.SharedMemo = shared
	   }
	   // 拒绝 FamMEMO 种子 Ensure；共享地址仅运营 Register / Hydrate。
	   prod := rt.Cfg != nil && authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  if prod {
		   shared.RefuseSeedEnsure()
	   }
	  if book := svc.Runs(); book != nil {
		  book.MarkProduction(prod)
		  if err := RefuseMemBook(prod, rt.Mem, book.SQLBound()); err != nil {
			  book.NoteAttachErr(err)
		  }
	   }
	   scanner := &SharedScanner{
		   Led: rt.Ledger, Wallet: rt.Wallet, Attr: attr, Pool: shared, Dep: svc,
	  }
	   rt.FamMemoScan = func(ctx context.Context) {
		  _, _ = scanner.ScanOnce(ctx)
	   }
	  MaybeEnableAttribution(svc, rt)
	   side := &CreditedSide{KV: rt.KV, Cfg: rt.Cfg}
	   side.Arrival = depositrails.NewNotifier(fundingbff.NewClient(rt.Dial, rt.Cfg, rt.Mem), rt.Log)
	   svc.BindCreditedSide(side)
  }

// EmitDeskHydrateMetrics desk Hydrate 错误推 Prometheus（一致 wd-track）。
  func EmitDeskHydrateMetrics(h DeskHealth) {
	   ok := 1.0
	   he := 0.0
	  if h.HydrateErr != "" {
		   ok = 0
		   he = 1
	   }
	  if !h.SQLBound {
		   ok = 1
		  he = 0
	   }
	   metrics.DepDeskOK.Set(ok)
	  metrics.DepDeskHydrateErr.Set(he)
	   metrics.DepDeskMemoryN.Set(float64(h.MemoryN))
	   metrics.DepDeskHydratedN.Set(float64(h.HydratedN))
   }

// FromRuntime 取已挂载的入金 Service；未挂载返回 nil。
   func FromRuntime(rt *app.Runtime) *Service {
	  if rt == nil || rt.Deposit == nil {
		  return nil
	  }
	  s, _ := rt.Deposit.(*Service)
	  return s
  }
