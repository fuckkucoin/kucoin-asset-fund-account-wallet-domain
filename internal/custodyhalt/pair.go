package custodyhalt

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const ScopePair Scope = "pair"

// PairKey 生成链+资产组合键；链与资产均必填。
  func PairKey(chain, asset string) (string, error) {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if chain == "" || asset == "" {
		  return "", ErrTarget
	  }
	  return "pair:" + chain + ":" + asset, nil
  }

  func parsePairKey(key string) (chain, asset string, ok bool) {
	  key = strings.TrimSpace(key)
	   if !strings.HasPrefix(key, "pair:") {
		   return "", "", false
	   }
	   rest := strings.TrimPrefix(key, "pair:")
	   parts := strings.SplitN(rest, ":", 2)
	   if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		   return "", "", false
	  }
	  return parts[0], parts[1], true
   }

// EngagePair 打开链+资产组合熔断。actor/reason 必填。
// 事故台可只停某链某币，避免误伤同链其他资产。键位落同一 KV kind；读失败由 CheckPair/CheckScope 失败即关。拼写错误会落成新键；解除前须 ListFlags 核对。

   func (g *Gate) EngagePair(ctx context.Context, chain, asset, actor, reason string) error {
	   return g.setScope(ctx, true, ScopePair, chain, asset, actor, reason, false)
  }

// ClearPair 仅人工解除组合作用域。对账路径不得调用。

  func (g *Gate) ClearPair(ctx context.Context, chain, asset, actor, reason string) error {
	  return g.setScope(ctx, false, ScopePair, chain, asset, actor, reason, false)
  }

// CheckPair 充提前闸：全局/链/资产/组合任一 On 或读失败均失败即关。

  func (g *Gate) CheckPair(ctx context.Context, chain, asset string) error {
	   if err := g.CheckScope(ctx, chain, asset); err != nil {
		  return err
	  }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if chain == "" || asset == "" {
		   return nil
	   }
	   if g == nil || g.kv == nil {
		  return errors.Join(ErrHalted, ErrStore)
	   }
	  sf, err := g.SnapshotScope(ctx, ScopePair, chain, asset)
	  if err != nil {
		  return errors.Join(ErrHalted, err)
	  }
	   if sf.On {
		  return fmt.Errorf("%w: pair %s/%s", ErrHalted, chain, asset)
	  }
	  return nil
   }

// ActivePairs 仅返回 On=true 的组合作用域。
   func (g *Gate) ActivePairs(ctx context.Context) ([]ScopedFlag, error) {
	   all, err := g.ListFlags(ctx, 200)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]ScopedFlag, 0)
	   for _, f := range all {
		   if f.On && f.Scope == ScopePair {
			  out = append(out, f)
		  }
	  }
	   return out, nil
   }
