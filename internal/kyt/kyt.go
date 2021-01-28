// 出金广播前筛查。筛查失败则拒绝上链。
// 作者: Likeo
// 日期: 2021.01.19
package kyt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
)

type Decision string

const (
	Allow  Decision = "allow"
	Review Decision = "review"
	Deny Decision = "deny"
)

var (
	ErrDenied = errors.New("kyt denied")
	ErrUnavailable = errors.New("kyt unavailable")
	ErrReview      = errors.New("kyt review")
)

type Request struct {
	UserID     string `json:"user_id"`
	MovementID string `json:"movement_id"`
	Chain     string `json:"chain"`
	ToAddress string `json:"to_address"`
	Screen(ctx context.Context,
		req Request) (Decision, string, error)
}

// Static 按配置返回 allow / review / deny。Err 非空就失败，调用方直接拒。
type Static struct {
	Mode Decision
	Err error
}

  func (s Static) Screen(context.Context, Request) (Decision, string, error) {
	   if s.Err != nil {
		   return "", "", s.Err
	  }
	  mode := s.Mode
	   if mode == "" {
		  mode = Allow
	  }
	  return mode, string(mode), nil
  }

   func ParseMode(s string) Decision {
	   d, err := parseMode(s)
	   if err != nil {
		  return Deny
	  }
	  return d
  }

  func parseMode(s string) (Decision, error) {
	   s = strings.ToLower(strings.TrimSpace(s))
	  switch Decision(s) {
	   case "", Allow, Decision("vendor"):
		  return Allow, nil
	  case Review:
		  return Review, nil
	   case Deny:
		  return Deny, nil
	  default:
		   return "", fmt.Errorf("kyt mode %q", s)
	   }
  }

// NewScreener 走厂商 HTTP，进程内黑名单/金额档先跑。厂商挂了就拒。

   func NewScreener(cfg config.KYTConfig) (Screener, error) {
	   rules, err := NewRules(cfg)
	   if err != nil {
		   return nil, err
	  }
	  mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	  if mode == "vendor" || strings.TrimSpace(cfg.VendorURL) != "" {
		   if strings.TrimSpace(cfg.VendorURL) == "" {
			  return nil, fmt.Errorf("kyt vendor_url required")
		   }
		   return newVendor(cfg, rules), nil
	  }
	  return rules, nil
   }
