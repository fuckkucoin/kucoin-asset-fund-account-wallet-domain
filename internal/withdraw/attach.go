// 功能：出金提现域之附属数据挂载。
// 作者：Kian
// 日期：2021.01.10

package withdraw

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/hotbound"
	"account-assets.kucoin.net/funding/internal/idemstore"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// Attach 把出金 Service 挂到 Runtime，进程内单例复用。
// app 不能直接 import withdraw（withdraw→app 环），字段类型为 any，具体为 *Service。

  func Attach(rt *app.Runtime, svc *Service) *Service {
	   if rt == nil {
		   return svc
	   }
	   if existing := FromRuntime(rt); existing != nil {
		  wireRuntime(existing, rt)
		   return existing
	  }
	   if svc == nil {
		  if rt.Ledger == nil {
			  return nil
		  }
		  svc = New(rt.Ledger)
	  }
	  wireRuntime(svc, rt)
	  rt.Withdraw = svc
	  return svc
  }

// AttachWith 在未挂载时用给定核验管道构造并挂载。

   func AttachWith(rt *app.Runtime, pipe *compliance.Pipeline) *Service {
	   if rt == nil {
		   return NewWith(nil, pipe)
	  }
	   if existing := FromRuntime(rt); existing != nil {
		   wireRuntime(existing, rt)
		   return existing
	   }
	   svc := NewWith(rt.Ledger, pipe)
	   wireRuntime(svc, rt)
	  rt.Withdraw = svc
	   return svc
   }

// wireRuntime 挂停提闸、holdcat、抬价/广播 SQL Hydrate。
// SQL 绑后 Hydrate；OpenMem 不绑；错误记入 Health 面。
// Hydrate 失败仅记空面，不得伪造确认/抬价。

  func wireRuntime(svc *Service, rt *app.Runtime) {
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
	  }
	   // 资金域统一熔断总线：只挡新申请，在途筛查/广播单仍按出金域既有规则处置。
	  // 核验管道的占用表作为出金速度来源，总线只读，不改 wd: 键形与限额判定。
	  bus := fundcircuit.Attach(rt)
	  if pipe := svc.Pipeline(); pipe != nil {
		   bus.AddUsage(pipe.Usage())
	   }
	   svc.BindFundCircuit(bus.WithdrawGate())
	  if rt.Store != nil {
		  hb := hotbound.New().BindStore(rt.Store)
		  svc.BindHotPrecheck(hb.PreSubmit)
	  }
	  if rt.KV != nil {
		   svc.BindHolds(holdcat.New(rt.KV))
		   _ = svc.BindGuardKV(rt.KV)
	  }
	  if rt.SQL != nil && svc.Tickets() != nil {
		  svc.Tickets().BindSQL(rt.SQL)
	   }
	   // OpenSQL：幂等键与冻结跃迁落库；OpenMem 挂内存幂等夹具。
	  if !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		  svc.BindIdem(idemstore.NewSQL(rt.SQL.DB))
		  svc.BindFreezeLog(NewFreezeEventWriter(rt.SQL.DB))
	   } else if svc.idem == nil {
		   svc.BindIdem(idemstore.NewMem())
	  }
	  // 装配 chainad.Registry + custody/HSM 抬价联签，供 encode→签→广播。
	  // 缺省 NewRegistry（mem）；Enc 有则挂 SignCanonical；OpenMem 亦可签测。
	  // 未绑 Registry 时 auto_broadcast 失败关闭。
	  if svc.Registry() == nil {
		   svc.BindRegistry(chainad.NewRegistry())
	  }
	   if rt.Enc != nil && svc.feeBumpSign == nil {
		   enc := rt.Enc
		  svc.BindFeeBumpSigner(enc.SignCanonical)
	  }
	  if rt.SQL != nil {
		   svc.Broadcasts().BindSQL(rt.SQL)
		   svc.Bumper().BindSQL(rt.SQL)
		   // Hydrate 错误保留在 Broadcaster/Bumper.Health，供 ops 探活与指标。
		  _ = svc.Broadcasts().Hydrate(context.Background())
		  _ = svc.Bumper().Hydrate(context.Background())
		   _ = svc.WDTrackHealth(context.Background())
		  if book := svc.Runs(); book != nil && !rt.Mem {
			  book.BindSQL(rt.SQL)
			   _ = book.Hydrate(context.Background())
		   }
	   }
	  prod := rt.Cfg != nil && authn.ProductionProfile(rt.Cfg.Auth.Kind,
		  rt.Cfg.Profile)
	  if book := svc.Runs(); book != nil {
		   book.MarkProduction(prod)
		  if err := RefuseMemBook(prod, rt.Mem, book.SQLBound()); err != nil {
			   book.NoteAttachErr(err)
		   }
	   }
	  // worker 周期推进 reorg→resign/超时失败；超时累计进指标。
	   rt.ReorgSagaTick = func(ctx context.Context) {
		   _ = ctx
		  res := svc.Broadcasts().TickSagas(DefaultSagaTimeout)
		  if res.TimedOut > 0 {
			  metrics.WDReorgSagaTimeoutTotal.Add(float64(res.TimedOut))
		   }
		   // 每轮刷新年龄/相位 gauge，供 ticket vs page 调谐。
		  _ = svc.WDTrackHealth(context.Background())
	  }
   }

// WDTrackHealth 出金跟踪表 hydrate 健康（广播+抬价）。

  func (s *Service) WDTrackHealth(ctx context.Context) map[string]any {
	  out := map[string]any{
		  "broadcasts": TrackHealth{},
		   "fee_bumps": TrackHealth{},
	  }
	   if s == nil {
		  return out
	  }
	   bc := s.Broadcasts().Health()
	   fb := s.Bumper().Health()
	  out["broadcasts"] = bc
	  out["fee_bumps"] = fb
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   if s.Broadcasts().sqlOn() {
		   store := sqlrepo.NewWDTrack(s.Broadcasts().sql)
		  if n, err := store.CountBroadcasts(ctx); err == nil {
			  out["broadcasts_sql_count"] = n
			  out["broadcasts_drift"] = n != bc.MemoryN && bc.HydrateErr == "" && bc.SQLBound
		   } else {
			   out["broadcasts_sql_err"] = err.Error()
		  }
		  if n, err := store.CountFeeBumps(ctx); err == nil {
			   out["fee_bumps_sql_count"] = n
			   out["fee_bumps_drift"] = n != fb.MemoryN && fb.HydrateErr == "" && fb.SQLBound
		   } else {
			  out["fee_bumps_sql_err"] = err.Error()
		   }
	  }
	  ok := bc.HydrateErr == "" && fb.HydrateErr == "" && bc.LastPersistErr == "" && fb.LastPersistErr == ""
	  out["ok"] = ok
	  sagas := s.Broadcasts().OpenSagas()
	   out["open_sagas"] = len(sagas)
	   phases := map[string]int{}
	  for _, sg := range sagas {
		   phases[sg.SagaPhase]++
	  }
	   out["saga_phases"] = phases
	   ages := s.Broadcasts().SagaMaxAges()
	  out["saga_max_age_sec"] = ages
	   timed := s.Broadcasts().TimedOutSagas()
	   out["timed_out_n"] = len(timed)
	   ids := make([]string, 0, len(timed))
	   for i, row := range timed {
		   if i >= 20 {
			   break
		   }
		   ids = append(ids, row.TransferID)
	  }
	   out["timed_out_ids"] = ids
	   EmitWDTrackMetrics(out, bc, fb, phases, ages)
	   return out
   }

// EmitWDTrackMetrics 把 wd-track 健康推到 Prometheus（超越 JSON 探活）。
// broadcast/fee_bump 双面；drift/hydrate/persist 分项；saga 相位计数与最大年龄。
// 指标仅告警面，不得捏造确认数；年龄告警与超时失败计数分工（ticket vs page）。
  func EmitWDTrackMetrics(body map[string]any, bc, fb TrackHealth, phases map[string]int, ages map[string]float64) {
	   setKind := func(kind string, h TrackHealth, sqlN any, drift any) {
		  ok := 1.0
		   if h.HydrateErr != "" || h.LastPersistErr != "" {
			   ok = 0
		  }
		   if !h.SQLBound {
			   ok = 1 // 未绑 SQL（OpenMem）视为探活无关
		  }
		   metrics.WDTrackOK.WithLabelValues(kind).Set(ok)
		  metrics.WDTrackMemoryN.WithLabelValues(kind).Set(float64(h.MemoryN))
		   n := 0.0
		   if v, ok := sqlN.(int); ok {
			   n = float64(v)
		  }
		  metrics.WDTrackSQLN.WithLabelValues(kind).Set(n)
		   d := 0.0
		   if v, ok := drift.(bool); ok && v {
			  d = 1
		   }
		  metrics.WDTrackDrift.WithLabelValues(kind).Set(d)
		  he, pe := 0.0, 0.0
		  if h.HydrateErr != "" {
			  he = 1
		   }
		  if h.LastPersistErr != "" {
			  pe = 1
		   }
		  metrics.WDTrackHydrateErr.WithLabelValues(kind).Set(he)
		   metrics.WDTrackPersistErr.WithLabelValues(kind).Set(pe)
	  }
	   setKind("broadcast", bc, body["broadcasts_sql_count"], body["broadcasts_drift"])
	  setKind("fee_bump", fb, body["fee_bumps_sql_count"], body["fee_bumps_drift"])
	  for _, ph := range []string{SagaReorged, SagaResigning, SagaResubmitting, "none"} {
		  metrics.WDReorgSaga.WithLabelValues(ph).Set(float64(phases[ph]))
		  age := 0.0
		   if ages != nil {
			   age = ages[ph]
		   }
		  metrics.WDReorgSagaMaxAge.WithLabelValues(ph).Set(age)
	  }
   }

// FromRuntime 取已挂载的出金 Service；未挂载返回 nil。
  func FromRuntime(rt *app.Runtime) *Service {
	   if rt == nil || rt.Withdraw == nil {
		  return nil
	   }
	  s, _ := rt.Withdraw.(*Service)
	   return s
   }
