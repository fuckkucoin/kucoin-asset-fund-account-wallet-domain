package gate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/freeze"
	"account-assets.kucoin.net/funding/internal/margin"
)

// Decision Evaluate 结果。

type Decision struct {
	Verdict    margin.Verdict
	Required   string
	Ratio      string
	Frozen     bool
	Already    bool
	LockPosted bool
}

// Config 门控配置。阈值必须来自配置，禁止在门控路径写死常量。
type Config struct {
	Params    margin.Params
	LockAsset string
}

// Gate 维持保证金强平门控。

type Gate struct {
	mu sync.Mutex
	cfg    Config
	freeze freeze.API
	locks  LockPoster
	last    map[string]Decision
	blocked map[string]bool
}

   func New(cfg Config, fr freeze.API, locks LockPoster) *Gate {
	   return &Gate{
		  cfg:    cfg,
		   freeze: fr,
		   locks: locks,
		   last:    map[string]Decision{},
		  blocked: map[string]bool{},
	   }
  }

  func (g *Gate) Params() margin.Params { return g.cfg.Params }

// Evaluate 计算组合维持额并与权益比较。Breach 时冻结一次并写锁仓分录。

  func (g *Gate) Evaluate(acc margin.Account, prices map[string]string) (Decision, error) {
	   if g == nil {
		   return Decision{}, fmt.Errorf("gate unavailable")
	  }
	  if strings.TrimSpace(acc.ID) == "" {
		   return Decision{}, fmt.Errorf("account id required")
	  }
	  v, req, ratio, err := margin.EvaluateAccount(acc, prices, g.cfg.Params)
	  if err != nil {
		   return Decision{}, err
	  }
	   d := Decision{Verdict: v, Required: req, Ratio: ratio}
	  g.mu.Lock()
	  wasBlocked := g.blocked[acc.ID]
	   g.mu.Unlock()
	   if v != margin.VerdictBreach {
		  d.Frozen = wasBlocked
		  d.Already = wasBlocked
		   g.mu.Lock()
		   g.last[acc.ID] = d
		   g.mu.Unlock()
		   return d, nil
	  }
	   already := false
	   if g.freeze != nil && g.freeze.Frozen(acc.ID) {
		   already = true
	   }
	   if wasBlocked {
		   already = true
	  }
	  d.Already = already
	   d.Frozen = true
	  if g.locks != nil && !already {
		  asset, err := lockAsset(acc, g.cfg)
		  if err != nil {
			  return d, err
		   }
		  amt, err := lockAmount(req)
		  if err != nil {
			  return d, err
		   }
		  if err := g.locks.PostLock(context.Background(), acc.ID, asset, amt, LockIdemKey(acc.ID)); err != nil {
			   return d, err
		  }
		  d.LockPosted = true
	   } else if g.locks != nil && already {
		  d.LockPosted = false
	  }
	   if g.freeze != nil {
		   if err := g.freeze.Freeze(acc.ID, freeze.ReasonMaintenanceBreach); err != nil {
			   return d, err
		  }
		  d.Frozen = g.freeze.Frozen(acc.ID)
	   }
	   g.mu.Lock()
	   g.blocked[acc.ID] = true
	  g.last[acc.ID] = d
	  g.mu.Unlock()
	   return d, nil
   }

   func lockAsset(acc margin.Account, cfg Config) (string, error) {
	  if strings.TrimSpace(acc.LockAsset) != "" {
		   return acc.LockAsset, nil
	   }
	  if strings.TrimSpace(cfg.LockAsset) != "" {
		   return cfg.LockAsset, nil
	   }
	   return "", fmt.Errorf("lock asset must come from account or config")
  }

// Allow 门控态拒绝加仓与外提。
   func (g *Gate) Allow(accountID string, op RiskOp) error {
	  if g == nil {
		   return fmt.Errorf("gate unavailable")
	  }
	   _, err := ParseRiskOp(string(op))
	   if err != nil {
		  return err
	  }
	   g.mu.Lock()
	  blocked := g.blocked[accountID]
	   g.mu.Unlock()
	   if g.freeze != nil && g.freeze.Frozen(accountID) {
		   blocked = true
	  }
	  if blocked && op.RiskIncreasing() {
		   return fmt.Errorf("%w: %s", ErrGated, op)
	  }
	  return nil
  }

// Last 最近一次判定。

   func (g *Gate) Last(accountID string) (Decision, bool) {
	  g.mu.Lock()
	  defer g.mu.Unlock()
	   d, ok := g.last[accountID]
	  return d, ok
   }

// Blocked 是否处于强平门控态。
  func (g *Gate) Blocked(accountID string) bool {
	   g.mu.Lock()
	  defer g.mu.Unlock()
	   return g.blocked[accountID]
   }

// TryRelease 解除条件未满足时保持冻结。权益回到阈值以上也不自动解冻。

  func (g *Gate) TryRelease(acc margin.Account, prices map[string]string) error {
	   v, _, _, err := margin.EvaluateAccount(acc, prices, g.cfg.Params)
	  if err != nil {
		  return err
	  }
	   if v == margin.VerdictBreach {
		   return fmt.Errorf("%w: release conditions unmet", ErrGated)
	   }
	   return fmt.Errorf("%w: unfreeze product not enabled", ErrGated)
   }
