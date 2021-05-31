package flow

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/pricepoison"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// ——————————————————————————————————
// 辅助：固定时钟
// ——————————————————————————————————

var (
	t0 = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
)

  func fixClock() func() time.Time { return func() time.Time { return t0 } }

// ——————————————————————————————————
// 1. Spike 检测器
// ——————————————————————————————————

   func TestSpikeDetector(t *testing.T) {
	  type row struct {
		  name string
		   refPrice string
		  tickPrice string
		  wantGrade pricepoison.PoisonGrade
	   }
	   rows := []row{
		   {"clean", "60000", "60000", pricepoison.GradeOK},
		  {"within_up", "60000", "110000", pricepoison.GradeOK}, // ratio=1.83 < 2.0
		  {"spike_up", "60000", "150000", pricepoison.GradeSpike}, // ratio=2.5 > 2.0
		   {"spike_down", "60000", "20000", pricepoison.GradeSpike}, // ratio=0.33 < 0.5
		   {"border_up", "60000", "119999", pricepoison.GradeOK},    // 1.999 < 2.0
		  {"border_down", "60000", "30001", pricepoison.GradeOK}, // 0.500 >= 0.5 (barely)
		   {"no_ref", "", "60000", pricepoison.GradeOK},           // 无参考价，跳过
	  }
	   for _, r := range rows {
		  r := r
		  t.Run(r.name, func(t *testing.T) {
			  priceFn := func(pair string) (string, error) {
				   if r.refPrice == "" {
					  return "", pricepoison.ErrRegistryUnavailable
				  }
				  return r.refPrice, nil
			  }
			  d := pricepoison.NewSpikeDetector(pricepoison.DefaultSpikeConfig, priceFn)
			   detail, err := d.Detect("BTC/USDT", r.tickPrice, t0)
			  if err != nil {
				   t.Fatalf("detect err: %v", err)
			   }
			  if detail.Grade != r.wantGrade {
				   t.Errorf("grade: want=%s got=%s detail=%s", r.wantGrade.Label(), detail.Grade.Label(), detail.String())
			   }
		  })
	   }
  }

// ——————————————————————————————————
// 2. Invert 检测器
// ——————————————————————————————————

   func TestInvertDetector(t *testing.T) {
	  type row struct {
		  name string
		  pair string
		  floor string
		  tickPrice string
		  wantGrade pricepoison.PoisonGrade
	   }
	  rows := []row{
		  {"clean", "BTC/USDT", "100", "60000", pricepoison.GradeOK},
		  {"at_floor", "BTC/USDT", "100", "100", pricepoison.GradeOK},
		  {"below_floor", "BTC/USDT", "100", "0.5", pricepoison.GradeInvert},
		  {"unknown_pair", "XRP/USDT", "100", "0.001", pricepoison.GradeOK}, // 未登记，跳过
		   {"eth_invert", "ETH/USDT", "1", "0.001", pricepoison.GradeInvert},
	   }
	   for _, r := range rows {
		   r := r
		   t.Run(r.name, func(t *testing.T) {
			  floors := []pricepoison.InvertFloor{{Pair: "BTC/USDT", Floor: "100"}, {Pair: "ETH/USDT", Floor: "1"}}
			   d := pricepoison.NewInvertDetector(floors)
			  detail, err := d.Detect(r.pair, r.tickPrice, t0)
			  if err != nil {
				   t.Fatalf("detect err: %v", err)
			  }
			  if detail.Grade != r.wantGrade {
				  t.Errorf("grade: want=%s got=%s detail=%s", r.wantGrade.Label(), detail.Grade.Label(), detail.String())
			  }
		  })
	  }
   }

// ——————————————————————————————————
// 3. CrossDev 检测器
// ——————————————————————————————————

   func TestCrossDevDetector(t *testing.T) {
	   type row struct {
		   name string
		  synPrice string
		  tickPrice string
		  maxBPS    int
		   wantGrade pricepoison.PoisonGrade
	   }
	  rows := []row{
		   {"clean_100bps", "3000", "3030", 200, pricepoison.GradeOK}, // 100 bps < 200
		  {"dev_250bps", "3000", "3075", 200, pricepoison.GradeCrossDev}, // 250 bps > 200
		  {"no_syn", "", "3000", 200, pricepoison.GradeOK}, // 无合成价，跳过
		  {"exact_200bps", "3000", "3060", 200, pricepoison.GradeOK}, // 200 bps == 200 (不超过)
		   {"201bps", "3000", "3060.3", 200, pricepoison.GradeCrossDev}, // 201 bps > 200
	  }
	  for _, r := range rows {
		   r := r
		   t.Run(r.name, func(t *testing.T) {
			  synFn := func(pair string) (string, error) {
				  if r.synPrice == "" {
					  return "", pricepoison.ErrRegistryUnavailable
				  }
				   return r.synPrice, nil
			   }
			   cfg := pricepoison.CrossDevConfig{MaxBPS: r.maxBPS}
			  d := pricepoison.NewCrossDevDetector(cfg, synFn)
			  detail, err := d.Detect("ETH/USDT", r.tickPrice, t0)
			   if err != nil {
				  t.Fatalf("detect err: %v", err)
			   }
			   if detail.Grade != r.wantGrade {
				  t.Errorf("grade: want=%s got=%s detail=%s", r.wantGrade.Label(), detail.Grade.Label(), detail.String())
			  }
		  })
	   }
  }

// ——————————————————————————————————
// 4. StaleMask 检测器
// ——————————————————————————————————

   func TestStaleMaskDetector(t *testing.T) {
	   type row struct {
		   name string
		  tickAge   time.Duration // now - tickAt
		   wantGrade pricepoison.PoisonGrade
	  }
	  rows := []row{
		  {"fresh", 5 * time.Second, pricepoison.GradeOK},
		  {"at_limit", 30 * time.Second, pricepoison.GradeOK}, // 恰好等于 MaxAge，不超
		  {"stale_31s", 31 * time.Second, pricepoison.GradeStaleMask},
		   {"very_stale", 10 * time.Minute, pricepoison.GradeStaleMask},
		   {"zero_time", 0, pricepoison.GradeOK}, // 零值 Duration = fresh
	   }
	  for _, r := range rows {
		  r := r
		  t.Run(r.name, func(t *testing.T) {
			   cfg := pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}
			  d := pricepoison.NewStaleMaskDetector(cfg, fixClock())
			  tickAt := t0.Add(-r.tickAge)
			  detail, err := d.Detect("SOL/USDT", "140", tickAt)
			  if err != nil {
				   t.Fatalf("detect err: %v", err)
			   }
			   if detail.Grade != r.wantGrade {
				  t.Errorf("grade: want=%s got=%s detail=%s", r.wantGrade.Label(), detail.Grade.Label(), detail.String())
			   }
		  })
	  }
  }

// ——————————————————————————————————
// 5. TokenRegistry 代次令牌
// ——————————————————————————————————

  func TestTokenRegistry(t *testing.T) {
	   reg := pricepoison.NewTokenRegistry(60*time.Second, fixClock())
	  // 签发干净令牌
	   tok, err := reg.Issue("BTC/USDT", "60000", t0, pricepoison.GradeOK, "credit")
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	  if tok.Generation != 1 {
		   t.Fatalf("gen: want=1 got=%d", tok.Generation)
	  }
	  if err := reg.Validate(tok); err != nil {
		  t.Fatalf("validate: %v", err)
	  }
	  // 毒价不得签发
	   _, err = reg.Issue("BTC/USDT", "150000", t0, pricepoison.GradeSpike, "credit")
	  if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		  t.Fatalf("expected ErrPoisonedTick, got: %v", err)
	  }
	   // 撤销后令牌失效
	  revokedGen := reg.Revoke("BTC/USDT")
	   if revokedGen != 1 {
		   t.Fatalf("revoked gen: want=1 got=%d", revokedGen)
	  }
	  if err := reg.Validate(tok); !errors.Is(err, pricepoison.ErrTokenRevoked) {
		  t.Fatalf("expected ErrTokenRevoked, got: %v", err)
	  }
	  // 撤销后恢复：新 tick 签发新代次
	  tok2, err := reg.Issue("BTC/USDT", "61000", t0, pricepoison.GradeOK, "credit")
	   if err != nil {
		   t.Fatalf("issue2: %v", err)
	   }
	  if tok2.Generation <= tok.Generation {
		  t.Fatalf("gen2 must be > gen1: %d <= %d", tok2.Generation, tok.Generation)
	   }
	   if err := reg.Validate(tok2); err != nil {
		   t.Fatalf("validate2: %v", err)
	   }
  }

   func TestTokenRegistryExpiry(t *testing.T) {
	  var now = t0
	   clock := func() time.Time { return now }
	  reg := pricepoison.NewTokenRegistry(10*time.Second, clock)
	   tok, err := reg.Issue("ETH/USDT", "3000", t0, pricepoison.GradeOK, "*")
	   if err != nil {
		  t.Fatalf("issue: %v", err)
	  }
	   // 未过期
	   if err := reg.Validate(tok); err != nil {
		  t.Fatalf("fresh: %v", err)
	  }
	  // 过期
	  now = t0.Add(11 * time.Second)
	  if err := reg.Validate(tok); !errors.Is(err, pricepoison.ErrTokenExpired) {
		   t.Fatalf("expected ErrTokenExpired, got: %v", err)
	  }
  }

// ——————————————————————————————————
// 6. PoisonEngine 流水线
// ——————————————————————————————————

  func TestPoisonEngineIngest(t *testing.T) {
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, fixClock())
	   // 注册 StaleMask 检测器（最简单，不需要外部参考）
	   staleDet := pricepoison.NewStaleMaskDetector(
		   pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		  fixClock(),
	   )
	   eng.BindDetector("stale", staleDet)
	   // 干净 tick
	   freshAt := t0.Add(-5 * time.Second)
	  tok, err := eng.Ingest("BTC/USDT", "60000", freshAt, "credit")
	  if err != nil {
		  t.Fatalf("clean ingest: %v", err)
	   }
	   if tok.Pair != "BTC/USDT" {
		   t.Fatalf("pair: want=BTC/USDT got=%s", tok.Pair)
	   }
	  // 毒价 tick（过期）
	  staleAt := t0.Add(-2 * time.Minute)
	  _, err = eng.Ingest("BTC/USDT", "60000", staleAt, "credit")
	   if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		  t.Fatalf("expected ErrPoisonedTick, got: %v", err)
	  }
	   // 事件仓库应有 1 条记录
	  events := eng.EventStore().List()
	   if len(events) != 1 {
		  t.Fatalf("event count: want=1 got=%d", len(events))
	   }
	   if events[0].Pair != "BTC/USDT" {
		  t.Fatalf("event pair: %s", events[0].Pair)
	  }
	   if events[0].Detail.Grade != pricepoison.GradeStaleMask {
		   t.Fatalf("event grade: %s", events[0].Detail.Grade.Label())
	  }
	   // 毒价后已有令牌失效，旧 tok 应该不通过
	  if err := eng.Validate(tok); !errors.Is(err, pricepoison.ErrTokenRevoked) {
		   t.Fatalf("expected ErrTokenRevoked after poison, got: %v", err)
	  }
   }

   func TestPoisonEngineMultiDetector(t *testing.T) {
	  eng := pricepoison.NewPoisonEngine(60*time.Second,
		   100, fixClock())
	   // 挂载多个检测器
	  spikeDet := pricepoison.NewSpikeDetector(pricepoison.DefaultSpikeConfig, func(pair string) (string, error) {
		   return "60000", nil
	  })
	   invertDet := pricepoison.NewInvertDetector([]pricepoison.InvertFloor{{Pair: "BTC/USDT", Floor: "100"}})
	  staleDet := pricepoison.NewStaleMaskDetector(pricepoison.DefaultStaleMaskConfig,
		   fixClock())
	   eng.BindDetector("spike", spikeDet)
	  eng.BindDetector("invert", invertDet)
	   eng.BindDetector("stale", staleDet)
	  // 干净 tick：所有检测器通过
	   _, err := eng.Ingest("BTC/USDT", "61000", t0.Add(-1*time.Second), "occupancy")
	   if err != nil {
		  t.Fatalf("clean: %v", err)
	  }
	   // Spike：价格过高
	   _, err = eng.Ingest("BTC/USDT", "200000", t0.Add(-1*time.Second), "occupancy")
	   if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		  t.Fatalf("spike: expected ErrPoisonedTick, got: %v", err)
	  }
	  evts := eng.EventStore().ListByPair("BTC/USDT")
	  if len(evts) == 0 {
		  t.Fatal("no events after spike")
	   }
	   last := evts[len(evts)-1]
	   if last.Detail.Grade != pricepoison.GradeSpike {
		  t.Fatalf("grade: want=spike got=%s", last.Detail.Grade.Label())
	   }
   }

// ——————————————————————————————————
// 7. WriteGate fail-closed 校验
// ——————————————————————————————————

  func newTestEngine(clock func() time.Time) *pricepoison.PoisonEngine {
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	   staleDet := pricepoison.NewStaleMaskDetector(
		pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		clock,
	)
	eng.BindDetector("stale", staleDet)
	return eng
}

   func TestWriteGate_Clean(t *testing.T) {
	  eng := newTestEngine(fixClock())
	  probe := writegate.NewProbeMutex(fixClock())
	   gate := writegate.NewWriteGate(eng, probe)
	  tok, err := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
	  if err != nil {
		  t.Fatalf("ingest: %v", err)
	   }
	  for _, scope := range []writegate.WriteScope{writegate.ScopeCredit, writegate.ScopeOccupancy, writegate.ScopeLiquidationTrigger} {
		  // 令牌 scope="credit"，只有 ScopeCredit 通过；其余应报 scope_mismatch
		   tokCopy := tok
		   tokCopy.Scope = string(scope)
		   // 重新签发对应 scope 的令牌
		  newTok, err := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), string(scope))
		  if err != nil {
			  t.Fatalf("ingest scope=%s: %v", scope, err)
		   }
		  if err := gate.Check(scope, newTok); err != nil {
			   t.Fatalf("gate check scope=%s: %v", scope, err)
		  }
	   }
   }

  func TestWriteGate_Blocked(t *testing.T) {
	  type row struct {
		   name string
		  setupFn func(eng *pricepoison.PoisonEngine,
			  probe *writegate.ProbeMutex) pricepoison.GenToken
		  scope writegate.WriteScope
		  wantErr error
	   }
	   clock := fixClock()
	   eng := newTestEngine(clock)
	   probe := writegate.NewProbeMutex(clock)
	   gate := writegate.NewWriteGate(eng, probe)
	  rows := []row{
		   {
			   name:  "token_revoked",
			   scope: writegate.ScopeCredit,
			   setupFn: func(eng *pricepoison.PoisonEngine, probe *writegate.ProbeMutex) pricepoison.GenToken {
				  tok, _ := eng.Ingest("ETH/USDT", "3000", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
				  eng.Revoke("ETH/USDT") // 手动撤销
				   return tok
			  },
			   wantErr: writegate.ErrTokenInvalid,
		   },
		   {
			   name:  "probe_locked",
			  scope: writegate.ScopeOccupancy,
			   setupFn: func(eng *pricepoison.PoisonEngine, probe *writegate.ProbeMutex) pricepoison.GenToken {
				  tok, _ := eng.Ingest("SOL/USDT", "140", t0.Add(-1*time.Second), string(writegate.ScopeOccupancy))
				  probe.MarkProbe("SOL/USDT") // half-open 探针
				  return tok
			  },
			  wantErr: writegate.ErrProbeLocked,
		   },
		  {
			  name: "scope_mismatch",
			   scope: writegate.ScopeLiquidationTrigger,
			  setupFn: func(eng *pricepoison.PoisonEngine, probe *writegate.ProbeMutex) pricepoison.GenToken {
				   tok, _ := eng.Ingest("BNB/USDT", "400", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
				   return tok // tok.Scope = credit, check = liquidation_trigger
			   },
			   wantErr: writegate.ErrScopeMismatch,
		  },
		  {
			  name:  "unknown_scope",
			  scope: writegate.WriteScope("invalid_scope"),
			   setupFn: func(eng *pricepoison.PoisonEngine, probe *writegate.ProbeMutex) pricepoison.GenToken {
				   tok, _ := eng.Ingest("XRP/USDT", "0.5", t0.Add(-1*time.Second), "invalid_scope")
				   return tok
			  },
			   wantErr: writegate.ErrUnknownScope,
		  },
	  }
	   for _, r := range rows {
		  r := r
		  t.Run(r.name, func(t *testing.T) {
			  tok := r.setupFn(eng, probe)
			   err := gate.Check(r.scope, tok)
			   if !errors.Is(err, r.wantErr) {
				  t.Errorf("want err=%v got=%v", r.wantErr, err)
			   }
		  })
	  }
  }

// ——————————————————————————————————
// 8. ProbeMutex 探针互斥锁
// ——————————————————————————————————

   func TestProbeMutex(t *testing.T) {
	  probe := writegate.NewProbeMutex(fixClock())
	  pair := "BTC/USDT"
	  if probe.IsProbing(pair) {
		  t.Fatal("initial: expected not probing")
	   }
	  probe.MarkProbe(pair)
	  if !probe.IsProbing(pair) {
		  t.Fatal("after mark: expected probing")
	   }
	  // 另一个 pair 不受影响
	  if probe.IsProbing("ETH/USDT") {
		   t.Fatal("other pair: expected not probing")
	   }
	   // 清除
	   probe.ClearProbe(pair)
	  if probe.IsProbing(pair) {
		  t.Fatal("after clear: expected not probing")
	  }
	   // 幂等清除
	  probe.ClearProbe(pair)
	  if probe.IsProbing(pair) {
		  t.Fatal("double clear: expected not probing")
	  }
   }

  func TestWriteGate_ProbeBlocksWrite(t *testing.T) {
	  eng := newTestEngine(fixClock())
	   probe := writegate.NewProbeMutex(fixClock())
	   gate := writegate.NewWriteGate(eng, probe)
	   tok, err := eng.Ingest("LTC/USDT", "80", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
	  if err != nil {
		   t.Fatalf("ingest: %v", err)
	  }
	   // 标记探针
	  probe.MarkProbe("LTC/USDT")
	   // 即使令牌有效，探针期间也应被拒
	  if err := gate.Check(writegate.ScopeCredit, tok); !errors.Is(err, writegate.ErrProbeLocked) {
		  t.Fatalf("expected ErrProbeLocked, got: %v", err)
	   }
	  // 清除后恢复
	   probe.ClearProbe("LTC/USDT")
	  if err := gate.Check(writegate.ScopeCredit, tok); err != nil {
		   t.Fatalf("after clear: expected nil, got: %v", err)
	   }
  }

// ——————————————————————————————————
// 9. PoisonProof 摘要构造与验证
// ——————————————————————————————————

  func TestPoisonProof(t *testing.T) {
	  eng := newTestEngine(fixClock())
	   // 制造一个干净令牌用于构造证明
	  tok, err := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
	   if err != nil {
		  t.Fatalf("ingest: %v", err)
	   }
	   detail := pricepoison.GradeDetail{
		   Grade: pricepoison.GradeSpike,
		   Rule:      "spike_ratio",
		  Observed:  "ratio=2.500000",
		   Threshold: "up=2.00 down=0.50",
	  }
	   proof, err := writegate.BuildPoisonProof(tok, detail, writegate.ScopeCredit, t0)
	   if err != nil {
		  t.Fatalf("build proof: %v", err)
	   }
	  if proof.Pair != "BTC/USDT" {
		   t.Fatalf("pair: %s", proof.Pair)
	  }
	  if proof.Digest == "" {
		  t.Fatal("empty digest")
	  }
	  // 验证通过
	   if err := proof.Verify(); err != nil {
		  t.Fatalf("verify: %v", err)
	   }
	   // 篡改后验证失败
	   tampered := proof
	   tampered.Rule = "different_rule"
	  if err := tampered.Verify(); err == nil {
		  t.Fatal("expected verify failure after tamper")
	   }
	  // Scope 改变后验证失败
	  tampered2 := proof
	   tampered2.WriteScope = "occupancy"
	   if err := tampered2.Verify(); err == nil {
		   t.Fatal("expected verify failure after scope tamper")
	   }
  }

  func TestPoisonProofDeterministic(t *testing.T) {
	   eng := newTestEngine(fixClock())
	   tok, _ := eng.Ingest("ETH/USDT", "3000", t0.Add(-2*time.Second), "occupancy")
	  detail := pricepoison.GradeDetail{
		   Grade: pricepoison.GradeInvert, Rule: "invert_floor",
		  Observed: "0.5", Threshold: "100",
	   }
	  p1,
		  _ := writegate.BuildPoisonProof(tok, detail, writegate.ScopeOccupancy, t0)
	  p2, _ := writegate.BuildPoisonProof(tok, detail, writegate.ScopeOccupancy, t0)
	  if p1.Digest != p2.Digest {
		  t.Fatalf("non-deterministic digest: %s != %s", p1.Digest, p2.Digest)
	   }
   }

// ——————————————————————————————————
// 10. EventStore FIFO 淘汰
// ——————————————————————————————————

  func TestEventStoreFIFO(t *testing.T) {
	   store := pricepoison.NewEventStore(3) // 容量=3
	  for i := 0; i < 5; i++ {
		   store.Append(pricepoison.PoisonEvent{
			  Pair: "BTC/USDT",
			   TickPrice: "60000",
			  TickAt: t0,
			  Detail: pricepoison.GradeDetail{Grade: pricepoison.GradeSpike, Rule: "spike_ratio"},
			  DetectedAt: t0,
		   })
	  }
	  if store.Len() != 3 {
		  t.Fatalf("capacity: want=3 got=%d", store.Len())
	   }
	   events := store.List()
	  // 应保留最新的 3 条（ID 3,4,5）
	   if events[0].ID != 3 {
		  t.Fatalf("first id: want=3 got=%d", events[0].ID)
	  }
	  if events[2].ID != 5 {
		  t.Fatalf("last id: want=5 got=%d", events[2].ID)
	  }
  }

// ——————————————————————————————————
// 11. CheckBatch 批量门控
// ——————————————————————————————————

  func TestWriteGateCheckBatch(t *testing.T) {
	   eng := newTestEngine(fixClock())
	   probe := writegate.NewProbeMutex(fixClock())
	   gate := writegate.NewWriteGate(eng, probe)
	   tok1, _ := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
	   tok2, _ := eng.Ingest("ETH/USDT", "3000", t0.Add(-2*time.Second), string(writegate.ScopeCredit))
	   // 全部有效
	  if err := gate.CheckBatch(writegate.ScopeCredit, []pricepoison.GenToken{tok1, tok2}); err != nil {
		   t.Fatalf("batch all valid: %v", err)
	  }
	   // 撤销一个后批量失败
	   eng.Revoke("ETH/USDT")
	  if err := gate.CheckBatch(writegate.ScopeCredit, []pricepoison.GenToken{tok1, tok2}); err == nil {
		  t.Fatal("batch with revoked: expected error")
	  }
  }

// ——————————————————————————————————
// 12. Golden 文件
// ——————————————————————————————————

  func readGolden(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	path := "../../testdata/golden/price_poison_writegate/" + name
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse golden %s: %v", name, err)
	}
	return m
}

  func TestGolden_Spike(t *testing.T) {
	   g := readGolden(t, "spike.json")
	   if g["grade"] != "spike" {
		   t.Fatalf("golden grade: want=spike got=%v", g["grade"])
	   }
	   if g["token_issued"] != false {
		   t.Fatalf("golden token_issued: want=false got=%v", g["token_issued"])
	  }
	   priceFn := func(pair string) (string, error) { return g["ref_price"].(string), nil }
	   d := pricepoison.NewSpikeDetector(pricepoison.DefaultSpikeConfig, priceFn)
	  detail, err := d.Detect(g["pair"].(string), g["tick_price"].(string), t0)
	   if err != nil {
		   t.Fatalf("detect: %v", err)
	  }
	  if detail.Grade.Label() != g["grade"] {
		   t.Fatalf("grade: want=%v got=%s", g["grade"], detail.Grade.Label())
	  }
   }

   func TestGolden_Invert(t *testing.T) {
	  g := readGolden(t, "invert.json")
	   floors := []pricepoison.InvertFloor{{Pair: g["pair"].(string), Floor: g["floor"].(string)}}
	  d := pricepoison.NewInvertDetector(floors)
	  detail, err := d.Detect(g["pair"].(string), g["tick_price"].(string), t0)
	   if err != nil {
		  t.Fatalf("detect: %v", err)
	  }
	   if detail.Grade.Label() != g["grade"].(string) {
		  t.Fatalf("grade: want=%v got=%s", g["grade"], detail.Grade.Label())
	   }
   }

  func TestGolden_CrossDev(t *testing.T) {
	   g := readGolden(t, "crossdev.json")
	  synFn := func(pair string) (string, error) { return g["synthetic_price"].(string), nil }
	  cfg := pricepoison.CrossDevConfig{MaxBPS: int(g["max_bps"].(float64))}
	   d := pricepoison.NewCrossDevDetector(cfg, synFn)
	   detail, err := d.Detect(g["pair"].(string), g["tick_price"].(string), t0)
	   if err != nil {
		  t.Fatalf("detect: %v", err)
	   }
	   if detail.Grade.Label() != g["grade"].(string) {
		   t.Fatalf("grade: want=%v got=%s", g["grade"], detail.Grade.Label())
	  }
  }

  func TestGolden_StaleMask(t *testing.T) {
	   g := readGolden(t, "stalemask.json")
	   ageSeconds := int64(g["tick_age_seconds"].(float64))
	   maxAgeSeconds := int64(g["max_age_seconds"].(float64))
	   staleAt := t0.Add(-time.Duration(ageSeconds) * time.Second)
	   cfg := pricepoison.StaleMaskConfig{MaxAge: time.Duration(maxAgeSeconds) * time.Second}
	  d := pricepoison.NewStaleMaskDetector(cfg, fixClock())
	   detail, err := d.Detect(g["pair"].(string), g["tick_price"].(string), staleAt)
	  if err != nil {
		   t.Fatalf("detect: %v", err)
	  }
	  if detail.Grade.Label() != g["grade"].(string) {
		   t.Fatalf("grade: want=%v got=%s", g["grade"], detail.Grade.Label())
	   }
   }

   func TestGolden_GateBlocked(t *testing.T) {
	  g := readGolden(t, "gate_blocked.json")
	   eng := newTestEngine(fixClock())
	  probe := writegate.NewProbeMutex(fixClock())
	  gate := writegate.NewWriteGate(eng, probe)
	  tok, _ := eng.Ingest(g["pair"].(string), g["tick_price"].(string), t0.Add(-1*time.Second), g["write_scope"].(string))
	  eng.Revoke(g["pair"].(string))
	  scope := writegate.WriteScope(g["write_scope"].(string))
	  err := gate.Check(scope, tok)
	   if err == nil {
		  t.Fatal("expected gate blocked, got nil")
	   }
	   if g["gate_allowed"].(bool) != false {
		  t.Fatalf("golden gate_allowed: want=false")
	  }
   }

// ——————————————————————————————————
// 13. PoisonEngine.GradeLevel 与 GradeAll
// ——————————————————————————————————

   func TestPoisonEngineGradeLevel(t *testing.T) {
	   type row struct {
		   name      string
		  tickPrice string
		  tickAge   time.Duration
		  refPrice  string
		   wantGrade pricepoison.PoisonGrade
	  }
	  rows := []row{
		  {"clean", "60000", 5 * time.Second, "60000", pricepoison.GradeOK},
		  {"spike_only", "150000", 5 * time.Second, "60000", pricepoison.GradeSpike},
		   {"stale_only", "60000", 2 * time.Minute, "60000", pricepoison.GradeStaleMask},
		   // Spike 和 Stale 同时出现时取最严重（Spike > StaleMask）
		  {"spike_and_stale", "150000", 2 * time.Minute, "60000", pricepoison.GradeSpike},
	   }
	   for _, r := range rows {
		   r := r
		   t.Run(r.name, func(t *testing.T) {
			   clock := fixClock()
			   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
			  eng.BindDetector("spike", pricepoison.NewSpikeDetector(
				   pricepoison.DefaultSpikeConfig,
				  func(pair string) (string, error) { return r.refPrice, nil },
			  ))
			  eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
				  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}, clock,
			  ))
			  tickAt := t0.Add(-r.tickAge)
			  got := eng.GradeLevel("BTC/USDT", r.tickPrice, tickAt)
			  if got != r.wantGrade {
				  t.Errorf("want=%s got=%s", r.wantGrade.Label(), got.Label())
			  }
		  })
	   }
   }

   func TestPoisonEngineGradeAll(t *testing.T) {
	   clock := fixClock()
	  eng := pricepoison.NewPoisonEngine(60*time.Second,
		   100, clock)
	   eng.BindDetector("spike", pricepoison.NewSpikeDetector(
		  pricepoison.DefaultSpikeConfig,
		   func(pair string) (string, error) { return "60000", nil },
	   ))
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		   pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}, clock,
	  ))
	   // 只触发 spike（新鲜时刻）
	  details := eng.GradeAll("BTC/USDT", "200000", t0.Add(-5*time.Second))
	   if len(details) != 1 {
		  t.Fatalf("want 1 poison detail, got %d", len(details))
	   }
	  if details[0].Grade != pricepoison.GradeSpike {
		   t.Fatalf("want spike, got %s", details[0].Grade.Label())
	  }
	  // 同时触发 spike+stale
	   details2 := eng.GradeAll("BTC/USDT", "200000", t0.Add(-2*time.Minute))
	   if len(details2) < 2 {
		   t.Fatalf("want >=2 poison details, got %d", len(details2))
	   }
   }

// ——————————————————————————————————
// 14. IngestBatch 批量摄入（fail-fast）
// ——————————————————————————————————

   func TestIngestBatch(t *testing.T) {
	   clock := fixClock()
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}, clock,
	  ))
	   ticks := []pricepoison.BatchTick{
		   {Pair: "BTC/USDT", Price: "60000", At: t0.Add(-1 * time.Second), Scope: "credit"},
		  {Pair: "ETH/USDT", Price: "3000", At: t0.Add(-2 * time.Second), Scope: "credit"},
		   {Pair: "SOL/USDT", Price: "140", At: t0.Add(-3 * time.Second), Scope: "credit"},
	  }
	   toks, err := eng.IngestBatch(ticks)
	   if err != nil {
		  t.Fatalf("clean batch: %v", err)
	   }
	  if len(toks) != 3 {
		  t.Fatalf("token count: want=3 got=%d", len(toks))
	  }
	  // 批次中有过期 tick → fail-fast
	   badTicks := []pricepoison.BatchTick{
		  {Pair: "BTC/USDT", Price: "60000", At: t0.Add(-1 * time.Second), Scope: "credit"},
		  {Pair: "ETH/USDT", Price: "3000", At: t0.Add(-5 * time.Minute), Scope: "credit"}, // 过期
		  {Pair: "SOL/USDT", Price: "140", At: t0.Add(-3 * time.Second), Scope: "credit"},
	  }
	  _, err = eng.IngestBatch(badTicks)
	  if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		   t.Fatalf("expected ErrPoisonedTick, got: %v", err)
	  }
  }

// ——————————————————————————————————
// 15. Engine Snapshot 与 Health
// ——————————————————————————————————

   func TestPoisonEngineSnapshot(t *testing.T) {
	  clock := fixClock()
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	  eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}, clock,
	   ))
	  // 无毒价记录时健康
	   hc := eng.Health()
	   if !hc.Healthy {
		   t.Fatal("expected healthy before any poison")
	   }
	  // 注入一条毒价
	   _, _ = eng.Ingest("BTC/USDT", "60000", t0.Add(-2*time.Minute), "credit")
	   snap := eng.Snapshot()
	  if snap.TotalEvents != 1 {
		  t.Fatalf("total_events: want=1 got=%d", snap.TotalEvents)
	   }
	   if len(snap.PairStats) != 1 {
		  t.Fatalf("pair_stats: want=1 got=%d", len(snap.PairStats))
	  }
	   ps := snap.PairStats[0]
	   if ps.Pair != "BTC/USDT" {
		  t.Fatalf("pair: %s", ps.Pair)
	  }
	  if !ps.Revoked {
		  t.Fatal("expected revoked=true after poison")
	   }
	   if ps.PoisonCount != 1 {
		  t.Fatalf("poison_count: want=1 got=%d", ps.PoisonCount)
	   }
	   // 健康检查应报告不健康
	   hc2 := eng.Health()
	  if hc2.Healthy {
		  t.Fatal("expected unhealthy after poison")
	   }
	  if len(hc2.RevokedPairs) == 0 {
		  t.Fatal("expected revoked pairs in health check")
	   }
  }

   func TestPoisonEngineRevokeAll(t *testing.T) {
	   clock := fixClock()
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		   pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		  clock,
	   ))
	   // 注入两个交易对的毒价
	  _, _ = eng.Ingest("BTC/USDT", "60000", t0.Add(-2*time.Minute), "credit")
	   _, _ = eng.Ingest("ETH/USDT", "3000", t0.Add(-3*time.Minute), "credit")
	  revoked := eng.RevokeAll()
	  if len(revoked) < 2 {
		  t.Fatalf("want >=2 revoked pairs, got %d", len(revoked))
	   }
  }

// ——————————————————————————————————
// 16. WriteGate CheckAudit 审计
// ——————————————————————————————————

   func TestWriteGateCheckAudit(t *testing.T) {
	   clock := fixClock()
	   eng := newTestEngine(clock)
	  probe := writegate.NewProbeMutex(clock)
	   gate := writegate.NewWriteGate(eng, probe)
	  // 干净 tick → AuditAllowed
	  tok, _ := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
	   entry, err := gate.CheckAudit(writegate.ScopeCredit, tok)
	  if err != nil {
		   t.Fatalf("audit clean: %v", err)
	  }
	   if entry.Result != writegate.AuditAllowed {
		  t.Fatalf("result: want=allowed got=%s", entry.Result)
	   }
	  if entry.Pair != "BTC/USDT" {
		   t.Fatalf("pair: %s", entry.Pair)
	  }
	   // 撤销后 → AuditBlockedToken
	  eng.Revoke("BTC/USDT")
	   entry2, err2 := gate.CheckAudit(writegate.ScopeCredit, tok)
	   if err2 == nil {
		   t.Fatal("expected error for revoked token")
	   }
	   if entry2.Result != writegate.AuditBlockedToken {
		   t.Fatalf("result: want=blocked_token got=%s", entry2.Result)
	  }
	  // 探针 → AuditBlockedProbe
	   tok3, _ := eng.Ingest("ETH/USDT", "3000", t0.Add(-1*time.Second), string(writegate.ScopeOccupancy))
	   probe.MarkProbe("ETH/USDT")
	  entry3, err3 := gate.CheckAudit(writegate.ScopeOccupancy, tok3)
	  if err3 == nil {
		  t.Fatal("expected error for probe locked")
	  }
	   if entry3.Result != writegate.AuditBlockedProbe {
		   t.Fatalf("result: want=blocked_probe got=%s", entry3.Result)
	   }
	   probe.ClearProbe("ETH/USDT")
  }

// ——————————————————————————————————
// 17. WriteGate Snapshot
// ——————————————————————————————————

   func TestWriteGateSnapshot(t *testing.T) {
	   clock := fixClock()
	  eng := newTestEngine(clock)
	   probe := writegate.NewProbeMutex(clock)
	  gate := writegate.NewWriteGate(eng,
		  probe)
	  snap := gate.Snapshot()
	   if len(snap.KnownScopes) != 3 {
		   t.Fatalf("known_scopes: want=3 got=%d", len(snap.KnownScopes))
	  }
	  if snap.EngineHealth.EventCount != 0 {
		  t.Fatalf("initial events: want=0 got=%d", snap.EngineHealth.EventCount)
	  }
  }

// ——————————————————————————————————
// 18. ProbeMutex Snapshot
// ——————————————————————————————————

  func TestProbeMutexSnapshot(t *testing.T) {
	   probe := writegate.NewProbeMutex(fixClock())
	  probe.MarkProbe("BTC/USDT")
	  probe.MarkProbe("ETH/USDT")
	   snap := probe.Snapshot()
	   if len(snap) != 2 {
		  t.Fatalf("snapshot len: want=2 got=%d", len(snap))
	   }
	   probe.ClearProbe("BTC/USDT")
	   snap2 := probe.Snapshot()
	   if len(snap2) != 1 {
		  t.Fatalf("after clear: want=1 got=%d", len(snap2))
	  }
   }

// ——————————————————————————————————
// 19. GradeDetail String 整理
// ——————————————————————————————————

  func TestGradeDetailString(t *testing.T) {
	  d := pricepoison.GradeDetail{
		   Grade: pricepoison.GradeSpike,
		  Rule: "spike_ratio",
		   Observed: "ratio=2.5",
		   Threshold: "up=2.00 down=0.50",
	   }
	   s := d.String()
	  if s == "" {
		   t.Fatal("empty String()")
	  }
	  if !strings.Contains(s, "spike") {
		   t.Fatalf("expected 'spike' in: %s", s)
	   }
  }

// ——————————————————————————————————
// 20. 零值 / nil 安全性
// ——————————————————————————————————

   func TestNilSafety(t *testing.T) {
	  // PoisonEngine nil
	   var eng *pricepoison.PoisonEngine
	   _, err := eng.Ingest("BTC/USDT", "60000", t0, "credit")
	   if !errors.Is(err, pricepoison.ErrRegistryUnavailable) {
		   t.Fatalf("nil engine ingest: want ErrRegistryUnavailable, got %v", err)
	  }
	  // WriteGate nil
	  var gate *writegate.WriteGate
	  err = gate.Check(writegate.ScopeCredit,
		  pricepoison.GenToken{})
	   if !errors.Is(err, writegate.ErrGateUnavailable) {
		   t.Fatalf("nil gate check: want ErrGateUnavailable, got %v", err)
	  }
	   // ProbeMutex nil
	  var probe *writegate.ProbeMutex
	   probe.MarkProbe("BTC/USDT")  // should not panic
	  probe.ClearProbe("BTC/USDT") // should not panic
	   if probe.IsProbing("BTC/USDT") {
		  t.Fatal("nil probe: expected false")
	  }
	   // TokenRegistry nil
	   var reg *pricepoison.TokenRegistry
	   _, err = reg.Issue("BTC/USDT", "60000", t0, pricepoison.GradeOK, "credit")
	   if !errors.Is(err, pricepoison.ErrRegistryUnavailable) {
		  t.Fatalf("nil registry issue: want ErrRegistryUnavailable, got %v", err)
	   }
  }

// ——————————————————————————————————
// 21. PoisonEngine 热更换检测器
// ——————————————————————————————————

   func TestDetectorHotSwap(t *testing.T) {
	   clock := fixClock()
	  eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	   // 初始无检测器，任何 tick 均通过
	  tok, err := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), "credit")
	  if err != nil {
		   t.Fatalf("no detector: %v", err)
	  }
	  if tok.Pair == "" {
		  t.Fatal("empty pair in token")
	   }
	   // 挂载严格的 StaleMask（MaxAge=1s）
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 1 * time.Second},
		   clock,
	  ))
	  // 2 秒前的 tick 现在应该被拒绝
	   _, err = eng.Ingest("BTC/USDT", "60000", t0.Add(-2*time.Second), "credit")
	  if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		   t.Fatalf("strict stale: expected ErrPoisonedTick, got: %v", err)
	  }
	  // 卸载检测器，新 tick 应再次通过
	   eng.UnbindDetector("stale")
	  _, err = eng.Ingest("BTC/USDT", "60000", t0.Add(-2*time.Second), "credit")
	   if err != nil {
		   t.Fatalf("after unbind: %v", err)
	  }
   }

// ——————————————————————————————————
// 22. 多交易对独立性
// ——————————————————————————————————

   func TestMultiPairIndependence(t *testing.T) {
	   clock := fixClock()
	  eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		   clock,
	  ))
	   // BTC 干净令牌
	  btcTok, err := eng.Ingest("BTC/USDT", "60000", t0.Add(-1*time.Second), "credit")
	  if err != nil {
		   t.Fatalf("btc clean: %v", err)
	  }
	   // ETH 毒价
	   _, err = eng.Ingest("ETH/USDT", "3000", t0.Add(-2*time.Minute), "credit")
	   if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		  t.Fatalf("eth poison: expected ErrPoisonedTick, got: %v", err)
	   }
	  // BTC 令牌不应受 ETH 毒价影响
	   if err := eng.Validate(btcTok); err != nil {
		   t.Fatalf("btc token invalidated by eth poison: %v", err)
	  }
	  // ETH 的令牌状态应为撤销
	  if !eng.Registry().IsRevoked("ETH/USDT") {
		  t.Fatal("eth should be revoked")
	   }
	  if eng.Registry().IsRevoked("BTC/USDT") {
		  t.Fatal("btc should NOT be revoked")
	  }
   }

// ——————————————————————————————————
// 23. WriteGate nil Engine
// ——————————————————————————————————

   func TestWriteGateNilEngine(t *testing.T) {
	   gate := writegate.NewWriteGate(nil, nil)
	  err := gate.Check(writegate.ScopeCredit, pricepoison.GenToken{})
	   if !errors.Is(err, writegate.ErrGateUnavailable) {
		   t.Fatalf("want ErrGateUnavailable, got: %v", err)
	  }
   }

// ——————————————————————————————————
// 24. SpikeDetector 极端值
// ——————————————————————————————————

   func TestSpikeDetectorEdge(t *testing.T) {
	   priceFn := func(pair string) (string, error) { return "1", nil }
	  d := pricepoison.NewSpikeDetector(pricepoison.DefaultSpikeConfig, priceFn)
	   // 零价格视为 spike
	  detail, err := d.Detect("BTC/USDT", "0", t0)
	  if err != nil {
		  t.Fatalf("zero price: %v", err)
	   }
	  if detail.Grade != pricepoison.GradeSpike {
		  t.Fatalf("zero price grade: want=spike got=%s", detail.Grade.Label())
	   }
	   // 负价格视为 spike
	  detail2, err := d.Detect("BTC/USDT", "-100", t0)
	  if err != nil {
		  t.Fatalf("neg price: %v", err)
	  }
	  if detail2.Grade != pricepoison.GradeSpike {
		  t.Fatalf("neg price grade: want=spike got=%s", detail2.Grade.Label())
	   }
   }

// ——————————————————————————————————
// 25. PoisonProof BuildFromEvent
// ——————————————————————————————————

   func TestBuildProofFromEvent(t *testing.T) {
	  clock := fixClock()
	   eng := newTestEngine(clock)
	  // 注入毒价以产生事件
	   _, err := eng.Ingest("SOL/USDT", "140", t0.Add(-2*time.Minute), "liquidation_trigger")
	   if !errors.Is(err, pricepoison.ErrPoisonedTick) {
		   t.Fatalf("expected poison: %v", err)
	  }
	  events := eng.EventStore().ListByPair("SOL/USDT")
	   if len(events) == 0 {
		  t.Fatal("no events for SOL/USDT")
	  }
	   evt := events[0]
	  proof, err := writegate.BuildProofFromEvent(evt, writegate.ScopeLiquidationTrigger, t0)
	   if err != nil {
		   t.Fatalf("BuildProofFromEvent: %v", err)
	   }
	   if err := proof.Verify(); err != nil {
		  t.Fatalf("verify: %v", err)
	   }
	  if proof.Pair != "SOL/USDT" {
		  t.Fatalf("pair: %s", proof.Pair)
	  }
	   if proof.WriteScope != string(writegate.ScopeLiquidationTrigger) {
		  t.Fatalf("scope: %s", proof.WriteScope)
	   }
  }

// ——————————————————————————————————
// helper import
// ——————————————————————————————————
var _ = strings.Contains // 确保 strings 已导入
