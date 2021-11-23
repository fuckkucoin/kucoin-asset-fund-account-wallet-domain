package recon

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EngageMode 事故升级时熔断作用域策略。
type EngageMode string

const (
	EngageNone   EngageMode = "none"   // 只落事故，不 Engage
	EngageGlobal EngageMode = "global" // 全局熔断
	EngageAsset  EngageMode = "asset"  // 单资产作用域（多资产退回不 Engage）
	EngagePair EngageMode = "pair" // 链+资产组合；缺链或缺单资产则不 Engage
	EngageAuto EngageMode = "auto" // 有链+单资产→pair；仅单资产→asset；否则 global
)

// HaltScopeEngager 可选分级 Engage；实现方不得暴露 Clear。

type HaltScopeEngager interface {
	EngageScoped(ctx context.Context,
		scope, chain, asset, actor, reason string) error
}

// IncidentPolicy 事故升级策略。
type IncidentPolicy struct {
	Mode     EngageMode // 空等同 auto
	MinLevel int        // 最低升级级别才 Engage；<=0 视为 2
	Chain    string     // 可选；pair/auto 收窄组合作用域时必填链
}

   func (p IncidentPolicy) normalized() IncidentPolicy {
	  if p.Mode == "" {
		  p.Mode = EngageAuto
	   }
	  if p.MinLevel <= 0 {
		  p.MinLevel = 2
	   }
	  p.Chain = strings.ToUpper(strings.TrimSpace(p.Chain))
	  return p
   }

// commonAsset 若全部违约单共有同一非空资产则返回该资产，否则空。

   func commonAsset(tickets []Ticket) string {
	   asset := ""
	  for _, t := range tickets {
		  a := strings.ToUpper(strings.TrimSpace(t.Asset))
		   if a == "" {
			   return ""
		  }
		  if asset == "" {
			   asset = a
			   continue
		  }
		   if asset != a {
			   return ""
		  }
	   }
	   return asset
   }

// pickEngageScope 按策略收敛熔断作用域；ok=false 表示本轮不 Engage。
// 功能：让 Policy.Chain 真正落到 pair 键，避免资产作用域误伤同币全链。
   func pickEngageScope(mode EngageMode, chain, asset string) (scope, outChain, outAsset string, ok bool) {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  switch mode {
	   case EngageNone:
		   return "", "", "", false
	  case EngageGlobal:
		   return "global", "", "", true
	   case EngagePair:
		  if chain == "" || asset == "" {
			   return "", "", "", false
		  }
		  return "pair", chain, asset, true
	   case EngageAsset:
		  if asset == "" {
			   return "", "", "", false
		  }
		  return "asset", chain, asset, true
	  case EngageAuto:
		   if chain != "" && asset != "" {
			   return "pair", chain, asset, true
		   }
		   if asset != "" {
			   return "asset", chain, asset, true
		  }
		   return "global", "", "", true
	   default:
		  return "global", "", "", true
	  }
   }

// RaiseIncidentsPolicy 按策略扫描超 SLA 工单并可选 Engage。
// 功能：级别不够只记事故；单资产可窄化熔断作用域；有链则先 pair。
// 重点：actor 不可为 system；Engage 失败不得假装成功；不得 Clear。值班须确认 MinLevel。
// 风险点：Mode=global 会停全站充提；pair 缺链时 auto 退回 asset 仍可能误伤同币他链。
  func (w *Workflow) RaiseIncidentsPolicy(ctx context.Context, now time.Time, actor string, hooks IncidentHooks, policy IncidentPolicy) ([]Incident, error) {
	   if w == nil {
		  return nil, ErrNotFound
	  }
	   actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return nil, ErrEscalateActor
	  }
	   policy = policy.normalized()
	   if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   breached := w.Breach(now)
	  sla := w.SLA()
	  out := make([]Incident, 0, len(breached))
	   eligible := make([]Ticket, 0, len(breached))
	  for _, t := range breached {
		   age := now.Sub(t.CreatedAt)
		  lvl := EscalationLevel(age, sla)
		   inc := Incident{
			   TicketID: t.ID,
			  UserID:   t.UserID,
			  OpenedBy: t.OpenedBy,
			  Age:      age,
			  Note:     fmt.Sprintf("reconcile SLA breach level=%d", lvl),
		  }
		  out = append(out, inc)
		  if lvl >= policy.MinLevel {
			   eligible = append(eligible, t)
		  }
	   }
	   if len(eligible) == 0 || policy.Mode == EngageNone || hooks.Halt == nil {
		  return out, nil
	  }
	   asset := commonAsset(eligible)
	  scope, chain, as, engageOK := pickEngageScope(policy.Mode, policy.Chain, asset)
	   if !engageOK {
		  skip := "engage skipped: scope unmet"
		  switch policy.Mode {
		  case EngagePair:
			   skip = "engage skipped: pair requires chain+asset"
		  case EngageAsset:
			  skip = "engage skipped: multi-asset"
		   }
		  for i := range out {
			  out[i].Note = out[i].Note + "; " + skip
		  }
		   return out, nil
	   }
	   reason := fmt.Sprintf("recon incident tickets=%d scope=%s chain=%s asset=%s actor=%s", len(eligible), scope, chain, as, actor)
	   var err error
	   if scope != "global" {
		   if sc, ok := hooks.Halt.(HaltScopeEngager); ok {
			  err = sc.EngageScoped(ctx, scope, chain, as, actor, reason)
			   if err != nil {
				   return out, err
			   }
			  markEngaged(out, eligible, reason)
			   return out, nil
		   }
		  // 无分级 Engage 能力时退回全局，避免假装已按作用域停闸。
		   scope, chain, as = "global", "", ""
		   reason = fmt.Sprintf("recon incident tickets=%d scope=%s chain=%s asset=%s actor=%s", len(eligible), scope, chain, as, actor)
	  }
	   err = hooks.Halt.Engage(ctx, actor, reason)
	   if err != nil {
		   return out, err
	  }
	  markEngaged(out, eligible, reason)
	  return out, nil
  }

  func markEngaged(out []Incident, eligible []Ticket, reason string) {
	  elSet := map[string]struct{}{}
	   for _, t := range eligible {
		   elSet[t.ID] = struct{}{}
	  }
	   for i := range out {
		   if _, ok := elSet[out[i].TicketID]; ok {
			  out[i].Engaged = true
			   out[i].Note = reason
		  }
	   }
   }
