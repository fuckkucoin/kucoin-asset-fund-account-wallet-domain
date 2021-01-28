package acctpol

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPolicyVersionExists 重复发布同一代次时返回。
var ErrPolicyVersionExists = errors.New("acct policy version already exists")

// ErrPolicyVersionNotFound 代次不存在时返回。

var ErrPolicyVersionNotFound = errors.New("acct policy version not found")

// ErrBizNotAllowed 业务类型被当前策略禁止时返回。

var ErrBizNotAllowed = errors.New("biz kind not allowed by policy")

// ErrHopNotAllowed hop 路径不在允许列表中时返回。
var ErrHopNotAllowed = errors.New("subject hop not allowed by policy")

// ErrRollbackInvalid 回滚目标代次无效（不能回滚到高于当前的代次）。

var ErrRollbackInvalid = errors.New("rollback target version invalid")

// BizKind 业务类型标识，对应账本的 Direction 语义分类。

type BizKind string

const (
	BizWithdraw     BizKind = "withdraw"
	BizDeposit      BizKind = "deposit"
	BizSpotToWallet BizKind = "spot_to_wallet"
	BizWalletToSpot BizKind = "wallet_to_spot"
	BizSpotSend     BizKind = "spot_send"
	BizFiatOnramp BizKind = "fiat_onramp"
	BizFiatOfframp BizKind = "fiat_offramp"
	BizConvertDebit  BizKind = "convert_debit"
	BizConvertCredit BizKind = "convert_credit"
	BizP2PDebit BizKind = "p2p_debit"
	BizP2PCredit BizKind = "p2p_credit"
	BizSubDebit  BizKind = "sub_debit"
	BizSubCredit BizKind = "sub_credit"
	BizEarnSubscribe BizKind = "earn_subscribe"
	BizEarnRedeem    BizKind = "earn_redeem"
	BizEarnInterest  BizKind = "earn_interest"
	BizFeeDebit      BizKind = "fee_debit"
	BizHotColdSweep  BizKind = "hot_cold_sweep"
	BizHotWarmSweep  BizKind = "hot_warm_sweep"
	BizWarmColdSweep BizKind = "warm_cold_sweep"
	BizFundingTrading BizKind = "funding_trading"
	BizFundingMargin BizKind = "funding_margin"
	BizFundingIsolated BizKind = "funding_isolated"
	BizFundingFutures  BizKind = "funding_futures"
)

// HopSpec 一条允许的科目 hop 路径。

type HopSpec struct {
	// From 源科目 ID（对应 subjtree.Node.ID）。
	From string `json:"from"`
	// To 目标科目 ID。
	To string `json:"to"`
	// ViaBridge 是否必须经 bridge 中转（额外约束，非树边）。
	ViaBridge bool `json:"via_bridge,omitempty"`
}

// EntryTemplate 分录模板：描述一笔业务的借贷科目列表。
// 用于校验调用方生成的分录是否符合政策，不负责生成分录。

type EntryTemplate struct {
	// DebitSubjects 借方科目 ID 列表（顺序不强制）。
	DebitSubjects []string `json:"debit_subjects"`
	// CreditSubjects 贷方科目 ID 列表。
	CreditSubjects []string `json:"credit_subjects"`
}

// PolicyRule 某个 BizKind 在该策略代次下的约束。
type PolicyRule struct {
	BizKind BizKind `json:"biz_kind"`
	// Enabled 为 false 时该业务在本代次中被完全禁止（gate 直接拒绝）。
	Enabled bool `json:"enabled"`
	// AllowedHops 本业务允许的科目 hop 列表（至少一条）。
	AllowedHops []HopSpec `json:"allowed_hops"`
	// Template 分录模板，用于 gate 验证分录结构。
	Template EntryTemplate `json:"template,omitempty"`
	// Conservation 为 true 时 gate 将校验借贷金额守恒。
	Conservation bool `json:"conservation"`
	// TreeVersionRequired 要求科目树代次必须 >= 此值（0 表示不限）。
	TreeVersionRequired int `json:"tree_version_required,omitempty"`
}

// PolicyVersion 一个不可变的策略代次快照。
type PolicyVersion struct {
	Version int `json:"version"`
	PublishedAt time.Time `json:"published_at"`
	Author string `json:"author"`
	Memo string `json:"memo,omitempty"`
	Rules []PolicyRule `json:"rules"`
	// EffectiveFrom 生效窗口起点（零值表示发布即生效）。
	EffectiveFrom time.Time `json:"effective_from,omitempty"`
	// EffectiveUntil 生效窗口截止（零值表示永不过期）。
	EffectiveUntil time.Time `json:"effective_until,omitempty"`
	// GrayRatio 灰度流量百分比 [0,100]。100 表示全量；0 表示仅灰度 Pin 用户。
	GrayRatio int `json:"gray_ratio"`
}

// IsEffective 检查 t 时刻本代次是否在生效窗口内。

   func (pv PolicyVersion) IsEffective(t time.Time) bool {
	  if !pv.EffectiveFrom.IsZero() && t.Before(pv.EffectiveFrom) {
		   return false
	   }
	   if !pv.EffectiveUntil.IsZero() && t.After(pv.EffectiveUntil) {
		   return false
	   }
	   return true
   }

// RuleFor 查找指定 BizKind 的规则；不存在返回 ErrBizNotAllowed。
  func (pv PolicyVersion) RuleFor(biz BizKind) (PolicyRule, error) {
	   biz = BizKind(strings.TrimSpace(string(biz)))
	   for _, r := range pv.Rules {
		  if r.BizKind == biz {
			  if !r.Enabled {
				  return PolicyRule{}, fmt.Errorf("%w: %s", ErrBizNotAllowed, biz)
			  }
			   return r, nil
		  }
	   }
	  return PolicyRule{}, fmt.Errorf("%w: %s", ErrBizNotAllowed, biz)
   }

// AllowsHop 检查本规则是否允许指定 from→to hop。
   func (r PolicyRule) AllowsHop(from, to string) bool {
	  for _, h := range r.AllowedHops {
		  if h.From == from && h.To == to {
			  return true
		   }
	   }
	  return false
  }

// Validate 校验代次内部一致性。

   func (pv PolicyVersion) Validate() error {
	  if pv.Version <= 0 {
		  return fmt.Errorf("policy version must be > 0")
	   }
	  seen := map[BizKind]bool{}
	   for _, r := range pv.Rules {
		   r.BizKind = BizKind(strings.TrimSpace(string(r.BizKind)))
		  if r.BizKind == "" {
			  return fmt.Errorf("rule has empty biz_kind")
		  }
		   if seen[r.BizKind] {
			   return fmt.Errorf("duplicate biz_kind in version: %s", r.BizKind)
		   }
		  seen[r.BizKind] = true
		   if r.Enabled && len(r.AllowedHops) == 0 {
			   return fmt.Errorf("enabled rule %s has no allowed hops", r.BizKind)
		  }
	  }
	   if pv.GrayRatio < 0 || pv.GrayRatio > 100 {
		  return fmt.Errorf("gray_ratio must be in [0,100]")
	  }
	  return nil
   }
