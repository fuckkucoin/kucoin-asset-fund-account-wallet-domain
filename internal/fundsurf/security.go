package fundsurf

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/tradepw"
	"account-assets.kucoin.net/funding/internal/wdverify"
)

// SecurityView /v1/security 读面聚合。

type SecurityView struct {
	WithdrawLock seclock.LockView `json:"withdraw_lock"`
	TradePW tradepw.StatusView  `json:"trading_password"`
	Verify  wdverify.StatusView `json:"withdraw_verify"`
	Forbidden bool `json:"forbidden"`
	WhitelistOnly bool   `json:"whitelist_only"`
	Host          string `json:"host"`
}

// BindSecurity 挂冷却服务、交易密码与出金验证闸。
   func (b *Book) BindSecurity(sec *seclock.Service, pw *tradepw.Book, gate *wdverify.Gate) *Book {
	  if b == nil {
		   return b
	   }
	   b.sec = sec
	   b.pw = pw
	   b.gate = gate
	  return b
  }

// SecService 暴露冷却服务（供 HTTP/运维复用）。
  func (b *Book) SecService() *seclock.Service {
	   if b == nil {
		   return nil
	  }
	  return b.sec
   }

// TradePW 暴露交易密码簿。

   func (b *Book) TradePW() *tradepw.Book {
	  if b == nil {
		   return nil
	  }
	   return b.pw
   }

// VerifyGate 暴露出金验证闸。

  func (b *Book) VerifyGate() *wdverify.Gate {
	   if b == nil {
		  return nil
	   }
	  return b.gate
  }

// EnsureGate 缺省装配验证闸（combos 来自配置；空则仅交易密码 RequireIfSet）。

  func (b *Book) EnsureGate() *wdverify.Gate {
	   if b == nil {
		  return nil
	   }
	  if b.gate != nil {
		  return b.gate
	   }
	  var combos [][]string
	   if b.cfg != nil {
		  combos = config.ParseWithdrawCombos(b.cfg.Funds.WithdrawCombos)
	  }
	  pw := b.pw
	  g := wdverify.New(b.kv, pw, combos).BindCooldown(b.sec)
	   b.gate = g
	   return g
  }

// SecurityStatus 聚合安全读面。

   func (b *Book) SecurityStatus(ctx context.Context, userID string) (SecurityView, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return SecurityView{}, ErrUser
	   }
	  out := SecurityView{Host: config.HostSecurity}
	   if b.sec != nil {
		   lv, err := b.sec.View(ctx, userID)
		   if err != nil {
			   return out, err
		   }
		   out.WithdrawLock = lv
	  } else if b.led != nil {
		  lock, err := b.Lock(ctx, userID)
		   if err != nil {
			   return out, err
		  }
		   out.WithdrawLock = seclock.LockView{
			  Locked: lock.Locked, Until: lock.Until, RemainSec: lock.RemainSec, Reason: lock.Reason,
		  }
	   }
	  pw := b.pw
	  if pw != nil {
		  st, err := pw.Status(ctx, userID)
		   if err != nil {
			  return out, err
		  }
		  out.TradePW = st
	   }
	  g := b.EnsureGate()
	  if g != nil {
		   vs, err := g.Status(ctx, userID)
		  if err != nil {
			   return out, err
		   }
		  out.Verify = vs
	   }
	  ok, err := b.Forbidden(ctx, userID)
	  if err != nil {
		   return out, err
	  }
	   out.Forbidden = ok
	   out.WhitelistOnly = b.WhitelistOnly(ctx,
		  userID)
	   return out, nil
  }

// RequireWithdrawSecure 出金提交前统一验证组合闸。
// 顺序：热停提/禁提 → 安全冷却 → 白名单（若开启）→ 因子组合。失败返回结构化错误。
  func (b *Book) RequireWithdrawSecure(ctx context.Context, userID, chain, addr string, p wdverify.Presented) error {
	  if err := b.RequireWithdraw(ctx, userID); err != nil {
		   return err
	   }
	  g := b.EnsureGate()
	   if g == nil {
		   return nil
	   }
	   wl := b.WhitelistOnly(ctx, userID)
	  return g.CheckWithdraw(ctx, userID, chain, addr, p, wl)
  }
