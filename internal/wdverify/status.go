package wdverify

import (
	"context"
	"strings"
)

// StatusView 出金验证设置/读面。

type StatusView struct {
	Combos     [][]string `json:"combos"`
	TradePWSet bool       `json:"tradepw_set"`
	TOTPBound bool `json:"totp_bound"`
	FailLocked bool  `json:"fail_locked"`
	RemainSec  int64 `json:"remain_sec,omitempty"`
	LastFactor  string `json:"last_factor,omitempty"`
	AllowHooked bool   `json:"allow_hooked"`
}

// Status 组装用户出金验证因子状态。

   func (g *Gate) Status(ctx context.Context, userID string) (StatusView, error) {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return StatusView{}, ErrUser
	   }
	   out := StatusView{
		   Combos:      g.Combos(),
		   AllowHooked: g != nil && g.allow != nil,
	   }
	   if g != nil && g.pw != nil {
		   out.TradePWSet = g.pw.Setted(ctx, userID)
	  }
	   if g != nil {
		   out.TOTPBound = g.TOTPBound(ctx, userID)
		  locked, remain, factor, err := g.FailLocked(ctx, userID)
		  if err != nil {
			  return out, err
		   }
		  out.FailLocked = locked
		  out.RemainSec = remain
		  out.LastFactor = factor
	   }
	  return out, nil
  }

// DefaultCombos 缺省双轨：短信+交易密码，或邮箱+TOTP+交易密码。

  func DefaultCombos() [][]string {
	   return [][]string{
		   {FactorSMS, FactorPW},
		   {FactorEmail, FactorTOTP, FactorPW},
	   }
  }
