package destentrygate

import "strings"

// Policy 条目租约策略快照（可配置 TTL / fail-closed）。

type Policy struct {
	// DefaultTTL 默认租约秒数。
	DefaultTTL int64 `json:"default_ttl_seconds"`
	// MaxTTL 上限。
	MaxTTL int64 `json:"max_ttl_seconds"`
	// FailClosedOnWhitelistOnly WhitelistOnly 开启时缺租约拒出金。
	FailClosedOnWhitelistOnly bool `json:"fail_closed_on_whitelist_only"`
	// RequireUnchangedGen 出金复验必须 entry_gen 一致。
	RequireUnchangedGen bool `json:"require_unchanged_gen"`
	// RejectRestoreNotFull Restore 冷却未满禁止续租。
	RejectRestoreNotFull bool `json:"reject_restore_not_full"`
	// AllowBreakglassIssue 允许破窗模式签发。
	AllowBreakglassIssue bool `json:"allow_breakglass_issue"`
}

// DefaultPolicy 默认：全部 fail-closed 开关打开。

   func DefaultPolicy() Policy {
	   return Policy{
		   DefaultTTL: DefaultTTLSeconds,
		   MaxTTL:                    MaxTTLSeconds,
		  FailClosedOnWhitelistOnly: true,
		  RequireUnchangedGen:       true,
		   RejectRestoreNotFull: true,
		  AllowBreakglassIssue: true,
	  }
   }

// ClampTTL 将请求 TTL 收口到策略区间。

   func (p Policy) ClampTTL(req int64) int64 {
	  def := p.DefaultTTL
	  if def <= 0 {
		   def = DefaultTTLSeconds
	   }
	  max := p.MaxTTL
	   if max <= 0 {
		   max = MaxTTLSeconds
	  }
	   if req <= 0 {
		  return def
	  }
	  if req > max {
		  return max
	   }
	   return req
   }

// MustHaveLease WhitelistOnly 开启时必须持有租约。

  func (p Policy) MustHaveLease(whitelistOnly bool, leaseID string) error {
	   if !p.FailClosedOnWhitelistOnly {
		  if strings.TrimSpace(leaseID) == "" {
			  return ErrNoLease
		  }
		   return nil
	   }
	  if whitelistOnly && strings.TrimSpace(leaseID) == "" {
		  return ErrWhitelistOnly
	  }
	   if strings.TrimSpace(leaseID) == "" {
		   return ErrNoLease
	   }
	  return nil
  }

// CheckRenew 续租前置：冷却必须已满且条目 live。
  func (p Policy) CheckRenew(live LiveEntry) error {
	   if !p.RejectRestoreNotFull {
		   return nil
	   }
	   if !live.Live || !live.CooldownFull {
		   return ErrRenewDenied
	   }
	  return nil
   }

// CheckGen 复验世代。

  func (p Policy) CheckGen(bind, live int64) error {
	   if !p.RequireUnchangedGen {
		  return nil
	  }
	   if bind != live {
		   return ErrEntryGenDrift
	   }
	  return nil
   }
