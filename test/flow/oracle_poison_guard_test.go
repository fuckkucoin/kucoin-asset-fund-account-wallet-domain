package flow

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/pricepoison"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// ——————————————————————————————————
// oracle PoisonGuard
// ——————————————————————————————————

  func newGuardWithStaleDetector(clock func() time.Time) (*oracle.Guard,
	*pricepoison.PoisonEngine) {
	feed := oracle.NewFeed(30 * time.Second).WithClock(clock)
	guard := oracle.NewGuard(feed, 3, 5*time.Second)
	eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		clock,
	))
	return guard, eng
}

  func TestOraclePoisonGuard_CleanTick(t *testing.T) {
	  clock := fixClock()
	   guard, eng := newGuardWithStaleDetector(clock)
	   probe := writegate.NewProbeMutex(clock)
	   pg := oracle.NewPoisonGuard(guard, eng, probe)
	   // 先放入一个干净 tick
	  _ = guard.Feed().Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: t0.Add(-1 * time.Second)})
	  tick, tok, err := pg.GetGated("BTC", "USDT", string(writegate.ScopeCredit))
	   if err != nil {
		  t.Fatalf("clean GetGated: %v", err)
	  }
	   if tick.Price != "60000" {
		  t.Fatalf("price: want=60000 got=%s", tick.Price)
	   }
	   if tok.Pair != "BTC/USDT" {
		   t.Fatalf("token pair: %s", tok.Pair)
	   }
	   if tok.Generation == 0 {
		  t.Fatal("token generation=0")
	   }
  }

   func TestOraclePoisonGuard_PoisonedTick(t *testing.T) {
	  clock := fixClock()
	   feed := oracle.NewFeed(30 * time.Second).WithClock(clock)
	  guard := oracle.NewGuard(feed,
		   3, 5*time.Second)
	  // 使用 Spike 检测器（独立于 oracle 的 stale 检查）
	  eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	  eng.BindDetector("spike", pricepoison.NewSpikeDetector(
		   pricepoison.DefaultSpikeConfig,
		   func(pair string) (string, error) { return "60000", nil },
	   ))
	  probe := writegate.NewProbeMutex(clock)
	  pg := oracle.NewPoisonGuard(guard, eng, probe)
	   // 放入价格异常的 tick（Spike）
	  _ = guard.Feed().Put(oracle.Tick{Asset: "ETH", Quote: "USDT", Price: "999999", At: t0.Add(-1 * time.Second)})
	  _, _, err := pg.GetGated("ETH", "USDT", string(writegate.ScopeCredit))
	   if err == nil {
		  t.Fatal("expected error for poisoned (spike) tick")
	  }
	  // 事件仓库应记录毒价
	  evts := eng.EventStore().ListByPair("ETH/USDT")
	  if len(evts) == 0 {
		   t.Fatal("no poison events after spike tick")
	   }
	  if evts[0].Detail.Grade != pricepoison.GradeSpike {
		  t.Fatalf("grade: want=spike got=%s", evts[0].Detail.Grade.Label())
	   }
   }

   func TestOraclePoisonGuard_MissingTick(t *testing.T) {
	  clock := fixClock()
	  guard, eng := newGuardWithStaleDetector(clock)
	  probe := writegate.NewProbeMutex(clock)
	   pg := oracle.NewPoisonGuard(guard, eng, probe)
	  // 不放 tick → Get 应返回 ErrMissing
	  _, _, err := pg.GetGated("SOL", "USDT", string(writegate.ScopeOccupancy))
	   if err == nil {
		  t.Fatal("expected error for missing tick")
	  }
  }

  func TestOraclePoisonGuard_ProbeMarking(t *testing.T) {
	   clock := fixClock()
	   guard, eng := newGuardWithStaleDetector(clock)
	   probe := writegate.NewProbeMutex(clock)
	  gate := writegate.NewWriteGate(eng,
		  probe)
	   pg := oracle.NewPoisonGuard(guard, eng, probe)
	  _ = guard.Feed().Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: t0.Add(-1 * time.Second)})
	  // 正常状态：获取令牌 + 写门放行
	   _, tok, err := pg.GetGated("BTC", "USDT", string(writegate.ScopeCredit))
	   if err != nil {
		   t.Fatalf("clean: %v", err)
	  }
	  if gErr := gate.Check(writegate.ScopeCredit, tok); gErr != nil {
		  t.Fatalf("gate clean: %v", gErr)
	  }
	  // 手动 half-open 状态：在 GetGated 前手动 MarkProbe
	  // （业务场景下由 PoisonGuard 在 half-open 时自动 MarkProbe）
	  probe.MarkProbe("BTC/USDT")
	   // 令牌在探针期间应被写门拒绝
	  if gErr := gate.Check(writegate.ScopeCredit, tok); !errors.Is(gErr, writegate.ErrProbeLocked) {
		   t.Fatalf("probe: want ErrProbeLocked, got: %v", gErr)
	  }
	  probe.ClearProbe("BTC/USDT")
	  // 清除后写门恢复
	  if gErr := gate.Check(writegate.ScopeCredit, tok); gErr != nil {
		   t.Fatalf("after clear: %v", gErr)
	  }
  }

// ——————————————————————————————————
// 联动：WriteGate + oracle 毒价链路
// ——————————————————————————————————

   func TestOracleWriteGateChain_PoisonFailsClosed(t *testing.T) {
	  clock := fixClock()
	   feed := oracle.NewFeed(30 * time.Second).WithClock(clock)
	   guard := oracle.NewGuard(feed, 3, 5*time.Second)
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 100, clock)
	  // 使用 Spike 检测器（独立于 oracle 的 stale 检查）
	  eng.BindDetector("spike", pricepoison.NewSpikeDetector(
		   pricepoison.DefaultSpikeConfig,
		   func(pair string) (string, error) { return "60000", nil },
	   ))
	  probe := writegate.NewProbeMutex(clock)
	  gate := writegate.NewWriteGate(eng, probe)
	  pg := oracle.NewPoisonGuard(guard, eng, probe)
	   // 先获取一个干净令牌（正常价格）
	  _ = guard.Feed().Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: t0.Add(-1 * time.Second)})
	   _, cleanTok, err := pg.GetGated("BTC", "USDT", string(writegate.ScopeCredit))
	   if err != nil {
		  t.Fatalf("clean: %v", err)
	  }
	  // 写门应放行
	   if gErr := gate.Check(writegate.ScopeCredit, cleanTok); gErr != nil {
		  t.Fatalf("clean gate: %v", gErr)
	  }
	   // 现在注入 spike 毒价
	   _ = guard.Feed().Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "200000", At: t0.Add(-2 * time.Second)})
	   _, _, poisonErr := pg.GetGated("BTC", "USDT", string(writegate.ScopeCredit))
	  if poisonErr == nil {
		  t.Fatal("expected poison error")
	   }
	  // 旧干净令牌应已被撤销，写门拒绝
	   if gErr := gate.Check(writegate.ScopeCredit, cleanTok); !errors.Is(gErr, writegate.ErrTokenInvalid) {
		   t.Fatalf("after poison: want ErrTokenInvalid, got: %v", gErr)
	   }
   }

// ——————————————————————————————————
// 表驱动：多交易对并发安全
// ——————————————————————————————————

  func TestPoisonEngineParallelPairs(t *testing.T) {
	  clock := fixClock()
	   eng := pricepoison.NewPoisonEngine(60*time.Second, 200, clock)
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second},
		  clock,
	  ))
	  pairs := []string{"BTC/USDT", "ETH/USDT", "SOL/USDT", "BNB/USDT", "XRP/USDT"}
	  type result struct {
		  pair string
		  tok  pricepoison.GenToken
		   err  error
	   }
	   ch := make(chan result, len(pairs))
	  for _, pair := range pairs {
		   go func(p string) {
			  tok, err := eng.Ingest(p, "100", t0.Add(-1*time.Second), "credit")
			  ch <- result{pair: p, tok: tok, err: err}
		  }(pair)
	   }
	  for range pairs {
		   r := <-ch
		   if r.err != nil {
			  t.Errorf("pair=%s error: %v", r.pair, r.err)
		  }
	  }
   }

   func TestWriteGateParallelCheck(t *testing.T) {
	  clock := fixClock()
	  eng := pricepoison.NewPoisonEngine(60*time.Second, 200, clock)
	   eng.BindDetector("stale", pricepoison.NewStaleMaskDetector(
		  pricepoison.StaleMaskConfig{MaxAge: 30 * time.Second}, clock,
	   ))
	   probe := writegate.NewProbeMutex(clock)
	   gate := writegate.NewWriteGate(eng, probe)
	   pairs := []string{"BTC/USDT", "ETH/USDT", "SOL/USDT"}
	   tokens := make([]pricepoison.GenToken, len(pairs))
	  for i, pair := range pairs {
		   tok, err := eng.Ingest(pair, "100", t0.Add(-1*time.Second), string(writegate.ScopeCredit))
		  if err != nil {
			   t.Fatalf("ingest %s: %v", pair, err)
		  }
		  tokens[i] = tok
	  }
	  errCh := make(chan error,
		  len(tokens))
	   for _, tok := range tokens {
		   go func(tok pricepoison.GenToken) {
			  errCh <- gate.Check(writegate.ScopeCredit,
				   tok)
		   }(tok)
	   }
	   for range tokens {
		   if err := <-errCh; err != nil {
			  t.Errorf("parallel gate check: %v", err)
		  }
	   }
   }
