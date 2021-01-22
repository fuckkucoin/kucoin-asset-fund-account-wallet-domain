package custodyhalt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Scope 熔断作用域。全局挡全部充提入口；链/资产仅挡匹配目标。

type Scope string

const (
	ScopeGlobal Scope = "global"
	ScopeChain  Scope = "chain"
	ScopeAsset Scope = "asset"
)

var (
	ErrScope  = errors.New("halt scope invalid")
	ErrTarget = errors.New("halt scope target required")
)

// ScopedFlag 带作用域的熔断快照。

type ScopedFlag struct {
	Scope Scope `json:"scope"`
	Chain string `json:"chain,omitempty"`
	Asset  string `json:"asset,omitempty"`
	On     bool   `json:"on"`
	Actor  string `json:"actor,omitempty"`
	Reason string `json:"reason,omitempty"`
	At time.Time `json:"at,omitempty"`
	Key string `json:"key"`
}

  func scopeKey(scope Scope, chain, asset string) (string, error) {
	   switch scope {
	  case ScopeGlobal, "":
		   return KeyGlobal, nil
	  case ScopeChain:
		  chain = strings.ToUpper(strings.TrimSpace(chain))
		  if chain == "" {
			   return "", ErrTarget
		  }
		   return "chain:" + chain, nil
	  case ScopeAsset:
		   asset = strings.ToUpper(strings.TrimSpace(asset))
		   if asset == "" {
			  return "", ErrTarget
		  }
		  return "asset:" + asset, nil
	  case ScopePair:
		  return PairKey(chain, asset)
	   default:
		  return "", ErrScope
	   }
  }

   func parseScopeKey(key string) (Scope, string, string) {
	   key = strings.TrimSpace(key)
	   if key == "" || key == KeyGlobal {
		   return ScopeGlobal, "", ""
	  }
	   if strings.HasPrefix(key, "chain:") {
		   return ScopeChain, strings.TrimPrefix(key, "chain:"), ""
	  }
	   if strings.HasPrefix(key, "asset:") {
		   return ScopeAsset, "", strings.TrimPrefix(key, "asset:")
	   }
	   if ch, as, ok := parsePairKey(key); ok {
		   return ScopePair, ch, as
	  }
	  return ScopeGlobal, "", ""
  }

// EngageScope 按作用域打开熔断。actor/reason 必填。
// 事故台可只停某链或某资产，降低全局误伤。键位落 KV，与全局共用 kind；读失败由 CheckScope 失败即关。作用域拼写错误会写成新键；值班须核对 ListFlags。
  func (g *Gate) EngageScope(ctx context.Context, scope Scope, chain, asset, actor, reason string) error {
	   return g.setScope(ctx,
		  true, scope, chain, asset, actor, reason, false)
  }

// ClearScope 仅人工按作用域解除。对账路径不得调用。

  func (g *Gate) ClearScope(ctx context.Context, scope Scope, chain, asset, actor, reason string) error {
	  return g.setScope(ctx, false, scope, chain, asset, actor, reason, false)
  }

// ClearScopeForced 人工强制按作用域解除；跳过 ClearChecker。
   func (g *Gate) ClearScopeForced(ctx context.Context, scope Scope, chain, asset, actor, reason string) error {
	  return g.setScope(ctx, false, scope, chain, asset, actor, reason, true)
  }

   func (g *Gate) setScope(ctx context.Context, on bool, scope Scope, chain, asset, actor, reason string, force bool) error {
	   if g == nil || g.kv == nil {
		   return ErrStore
	  }
	   if !on {
		  if err := g.guardClear(ctx, force); err != nil {
			  return err
		  }
	  }
	  actor = strings.TrimSpace(actor)
	   reason = strings.TrimSpace(reason)
	   if actor == "" {
		   return ErrActor
	  }
	  if reason == "" {
		   return ErrReason
	   }
	   if scope == "" {
		  scope = ScopeGlobal
	  }
	   key,
		  err := scopeKey(scope, chain, asset)
	   if err != nil {
		  return err
	  }
	  sc, ch, as := parseScopeKey(key)
	   now := time.Now().UTC()
	  var body []byte
	   if key == KeyGlobal {
		  body,
			  _ = json.Marshal(Flag{On: on, Actor: actor, Reason: reason, At: now})
	  } else {
		  body, _ = json.Marshal(ScopedFlag{
			   Scope: sc, Chain: ch, Asset: as, On: on,
			   Actor: actor, Reason: reason, At: now, Key: key,
		  })
	   }
	   cur, err := g.kv.Get(ctx, Kind, key)
	  exp := persist.Fence{}
	   if err == nil {
		  exp = cur.Fence
	  } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	  }
	  return g.kv.Put(ctx, persist.Row{Kind: Kind, Key: key, Body: body}, exp)
   }

// SnapshotScope 读指定作用域标志。
  func (g *Gate) SnapshotScope(ctx context.Context, scope Scope, chain, asset string) (ScopedFlag, error) {
	   if g == nil || g.kv == nil {
		  return ScopedFlag{}, ErrStore
	   }
	  if scope == "" {
		   scope = ScopeGlobal
	   }
	  key, err := scopeKey(scope, chain, asset)
	  if err != nil {
		   return ScopedFlag{}, err
	   }
	   row, err := g.kv.Get(ctx, Kind, key)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  sc, ch, as := parseScopeKey(key)
			  return ScopedFlag{Scope: sc,
				   Chain: ch, Asset: as, Key: key}, nil
		   }
		  return ScopedFlag{}, err
	  }
	  return decodeFlag(row.Key, row.Body), nil
   }

  func decodeFlag(key string, body []byte) ScopedFlag {
	  sc, ch, as := parseScopeKey(key)
	  var sf ScopedFlag
	   if err := json.Unmarshal(body, &sf); err == nil && (sf.Scope != "" || sf.On || sf.Actor != "") {
		  if sf.Scope == "" {
			  sf.Scope = sc
		   }
		  if sf.Key == "" {
			  sf.Key = key
		   }
		   if sf.Chain == "" {
			  sf.Chain = ch
		   }
		   if sf.Asset == "" {
			   sf.Asset = as
		   }
		   return sf
	   }
	  var f Flag
	   _ = json.Unmarshal(body, &f)
	   return ScopedFlag{Scope: sc, Chain: ch, Asset: as, On: f.On, Actor: f.Actor, Reason: f.Reason, At: f.At, Key: key}
  }

// ListFlags 列出 kind 下全部熔断键。

  func (g *Gate) ListFlags(ctx context.Context, limit int) ([]ScopedFlag, error) {
	  if g == nil || g.kv == nil {
		  return nil, ErrStore
	   }
	  if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	   rows, err := g.kv.List(ctx, Kind, limit)
	  if err != nil {
		   return nil, err
	   }
	  out := make([]ScopedFlag, 0, len(rows))
	  for _, r := range rows {
		   out = append(out, decodeFlag(r.Key, r.Body))
	  }
	  return out, nil
   }

// CheckScope 充提前闸：全局 On、匹配链/资产 On、或读库失败均失败即关。
   func (g *Gate) CheckScope(ctx context.Context, chain, asset string) error {
	  if g == nil || g.kv == nil {
		   return errors.Join(ErrHalted, ErrStore)
	  }
	  gf, err := g.Snapshot(ctx)
	   if err != nil {
		   return errors.Join(ErrHalted, err)
	  }
	  if gf.On {
		   return ErrHalted
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if chain != "" {
		   sf, err := g.SnapshotScope(ctx, ScopeChain, chain, "")
		   if err != nil {
			  return errors.Join(ErrHalted, err)
		   }
		   if sf.On {
			   return fmt.Errorf("%w: chain %s", ErrHalted, chain)
		  }
	  }
	   if asset != "" {
		  sf, err := g.SnapshotScope(ctx, ScopeAsset, "", asset)
		   if err != nil {
			  return errors.Join(ErrHalted, err)
		  }
		  if sf.On {
			   return fmt.Errorf("%w: asset %s", ErrHalted, asset)
		   }
	   }
	   if chain != "" && asset != "" {
		   sf,
			   err := g.SnapshotScope(ctx, ScopePair, chain, asset)
		  if err != nil {
			   return errors.Join(ErrHalted, err)
		   }
		   if sf.On {
			   return fmt.Errorf("%w: pair %s/%s", ErrHalted, chain, asset)
		   }
	   }
	  return nil
  }

// CheckActive 任一作用域处于 On 即挡；供清扫/运营入口在不知具体链资产时失败即关。

   func (g *Gate) CheckActive(ctx context.Context) error {
	   flags, err := g.ListFlags(ctx, 200)
	   if err != nil {
		   return errors.Join(ErrHalted,
			  err)
	   }
	  for _, f := range flags {
		   if f.On {
			   return fmt.Errorf("%w: scope=%s key=%s", ErrHalted, f.Scope, f.Key)
		   }
	  }
	  return nil
  }

// ActiveFlags 仅返回 On=true 的作用域，供运营读面。

  func (g *Gate) ActiveFlags(ctx context.Context) ([]ScopedFlag, error) {
	  all, err := g.ListFlags(ctx, 200)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]ScopedFlag, 0)
	  for _, f := range all {
		   if f.On {
			  out = append(out, f)
		  }
	  }
	   return out, nil
  }
