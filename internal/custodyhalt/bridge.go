package custodyhalt

import (
	"context"
	"strings"
)

// EngageOnly 供对账漂移路径挂钩。Clear 必须走运营 HTTP 人工口。

type EngageOnly struct {
	Gate *Gate
	Hist *History
}

  func (e EngageOnly) Engage(ctx context.Context, actor, reason string) error {
	  if e.Gate == nil {
		   return ErrStore
	  }
	   if err := e.Gate.Engage(ctx, actor, reason); err != nil {
		   return err
	   }
	  hist := e.Hist
	  if hist == nil {
		  hist = SharedHistory()
	  }
	   hist.Append(Event{Action: "engage", Actor: actor, Reason: reason, On: true, Scope: string(ScopeGlobal)})
	  return nil
  }

// EngageScope 按作用域打开熔断；仍禁止 Clear。
  func (e EngageOnly) EngageScope(ctx context.Context, scope Scope, chain, asset, actor, reason string) error {
	   if e.Gate == nil {
		   return ErrStore
	   }
	  if err := e.Gate.EngageScope(ctx, scope, chain, asset, actor, reason); err != nil {
		   return err
	   }
	   hist := e.Hist
	  if hist == nil {
		   hist = SharedHistory()
	   }
	  hist.Append(Event{
		   Action: "engage", Actor: actor, Reason: reason, On: true,
		  Scope: string(scope), Chain: strings.ToUpper(strings.TrimSpace(chain)),
		  Asset: strings.ToUpper(strings.TrimSpace(asset)),
	   })
	  return nil
   }

// EngageScoped 按字符串作用域 Engage；供 recon.HaltScopeEngager 适配，仍禁止 Clear。

   func (e EngageOnly) EngageScoped(ctx context.Context, scope, chain, asset, actor, reason string) error {
	  sc := Scope(strings.TrimSpace(scope))
	   if sc == "" {
		  sc = ScopeGlobal
	  }
	  if sc == ScopeGlobal {
		   return e.Engage(ctx, actor, reason)
	   }
	  return e.EngageScope(ctx,
		  sc, chain, asset, actor, reason)
  }
