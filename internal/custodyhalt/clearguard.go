package custodyhalt

import (
	"context"
	"errors"
)

// ErrClearBlocked 解熔断被挡（通常仍有未关对账工单）。
var ErrClearBlocked = errors.New("halt clear blocked: open reconcile tickets remain")

// ClearChecker 解熔断前只读闸；返回非 nil 则拒绝 Clear。

type ClearChecker interface {
	AllowClear(ctx context.Context) error
}

// ClearDetail 解熔断前检读面；供运营一眼看到挡因与未关数。
type ClearDetail struct {
	Allow bool `json:"allow_clear"`
	Reason     string `json:"reason,omitempty"`
	ReconOpen  int    `json:"recon_open"`
	ReconAcked int    `json:"recon_acked"`
	ForceHint  string `json:"force_hint,omitempty"`
	Policy     string `json:"policy"`
	Host string `json:"host"`
}

// ClearDetailer 可选；实现方可附带未关工单计数。
type ClearDetailer interface {
	ClearDetail(ctx context.Context) ClearDetail
}

// BindClearCheck 装配解熔断前闸；对账路径不得经此 Clear。
// 人工 Clear 前核对未关差异。仅 Clear/ClearScope 触发；Engage 不检查。未装配时 Clear 不挡，值班须自行核对工单。
  func (g *Gate) BindClearCheck(c ClearChecker) *Gate {
	  if g != nil {
		  g.clearCheck = c
	   }
	  return g
   }

  func (g *Gate) guardClear(ctx context.Context, force bool) error {
	  if force || g == nil || g.clearCheck == nil {
		   return nil
	   }
	   if err := g.clearCheck.AllowClear(ctx); err != nil {
		   if errors.Is(err, ErrClearBlocked) {
			   return err
		  }
		  return errors.Join(ErrClearBlocked, err)
	  }
	   return nil
   }

// ProbeClear 解熔断前检读面；先走 ClearDetailer，否则仅 Allow/挡因。
// 功能：clear-check 与结案后提示共用，避免值班再查一层工单。
// 重点：不改熔断状态；force 仅提示可用性。
// 风险点：未装配 ClearChecker 时 allow=true，不得据此跳过人工核对热钱包。

   func (g *Gate) ProbeClear(ctx context.Context) ClearDetail {
	   d := ClearDetail{
		   Allow: true,
		  Policy: "human_only",
		   Host:      "custody-boundary-lab.kucoin.net",
		   ForceHint: "set force=true only after recon tickets closed and hot-wallet verified",
	   }
	  if g == nil || g.clearCheck == nil {
		  return d
	  }
	  if cd, ok := g.clearCheck.(ClearDetailer); ok {
		   got := cd.ClearDetail(ctx)
		   if got.Policy == "" {
			   got.Policy = d.Policy
		  }
		  if got.Host == "" {
			  got.Host = d.Host
		   }
		   if got.ForceHint == "" {
			   got.ForceHint = d.ForceHint
		   }
		  return got
	   }
	   if err := g.guardClear(ctx, false); err != nil {
		  d.Allow = false
		  d.Reason = err.Error()
	  }
	   return d
   }
