// 出金限额、白名单和提速控制。冻结前硬拒。
// 作者: Cason
// 日期: 2021.01.06
package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

// RiskEngine 在冻结之前执行出金与入金限额。
// 超限或地址簿不符时直接拒绝。拒绝发生在建单之前，不得留下在途冻结。
type RiskEngine struct {
	maxSingle decimal.Decimal
	daily         decimal.Decimal
	dailyWithdraw decimal.Decimal
	dailyDeposit  decimal.Decimal
	suspicious    decimal.Decimal
	minAmount     decimal.Decimal
	allowlist     bool
	maxVel        int
	maxWithdrawVel             int
	maxDepositVel              int
	velWindow                  time.Duration
	suspiciousRequiresAdvanced bool
	firstDestHalfCap bool
}

// NewRisk 装配出金限额/白名单。硬拒绝不得留下在途冻结。
  func NewRisk(cfg config.RiskConfig) (*RiskEngine, error) {
	   ms,
		  err := decimal.NewFromString(cfg.MaxSingle)
	   if err != nil {
		  return nil, fmt.Errorf("max_single: %w", err)
	  }
	   dl, err := decimal.NewFromString(cfg.DailyLimit)
	   if err != nil {
		  return nil, fmt.Errorf("daily_limit: %w", err)
	   }
	   su, err := decimal.NewFromString(cfg.SuspiciousAmount)
	   if err != nil {
		  return nil, fmt.Errorf("suspicious_amount: %w", err)
	  }
	  min := decimal.RequireFromString("0.00000001")
	  if cfg.MinAmount != "" {
		   min, err = decimal.NewFromString(cfg.MinAmount)
		  if err != nil {
			  return nil, fmt.Errorf("min_amount: %w", err)
		  }
	   }
	  vw := cfg.VelocityWindow
	  if vw <= 0 {
		   vw = time.Minute
	   }
	  dw := dl
	   if cfg.DailyWithdrawLimit != "" {
		  dw, err = decimal.NewFromString(cfg.DailyWithdrawLimit)
		   if err != nil {
			  return nil, fmt.Errorf("daily_withdraw_limit: %w", err)
		  }
	   }
	  dd := dl
	   if cfg.DailyDepositLimit != "" {
		  dd,
			  err = decimal.NewFromString(cfg.DailyDepositLimit)
		  if err != nil {
			   return nil, fmt.Errorf("daily_deposit_limit: %w", err)
		   }
	   }
	   maxWV := cfg.MaxWithdrawVelocity
	   if maxWV <= 0 {
		   maxWV = cfg.MaxVelocity
	   }
	   maxDV := cfg.MaxDepositVelocity
	   if maxDV <= 0 {
		   maxDV = cfg.MaxVelocity
	   }
	   return &RiskEngine{
		  maxSingle: ms, daily: dl, dailyWithdraw: dw, dailyDeposit: dd,
		  suspicious: su, minAmount: min, allowlist: cfg.AllowlistEnabled,
		  maxVel: cfg.MaxVelocity, maxWithdrawVel: maxWV, maxDepositVel: maxDV,
		   velWindow: vw, suspiciousRequiresAdvanced: cfg.SuspiciousRequiresAdvanced,
		  firstDestHalfCap: cfg.FirstDestinationHalfCap,
	   }, nil
   }

type RiskDecision struct {
	Reject bool
	Reason string
	Flags  []string
}

// RequiresAdvancedKYC 大额或首提是否要 advanced KYC。

   func (d RiskDecision) RequiresAdvancedKYC() bool {
	  for _, f := range d.Flags {
		   if f == "suspicious_amount" || f == "first_destination" {
			   return true
		   }
	   }
	  return false
   }

   func rejectGate(gate, reason string, flags []string) RiskDecision {
	  metrics.FundGates.WithLabelValues(gate, "reject").Inc()
	   return RiskDecision{Reject: true, Reason: reason, Flags: flags}
  }

   func storeGate(gate string) {
	   metrics.FundGates.WithLabelValues(gate, "store").Inc()
   }

// Evaluate 核对单笔上限、分方向日占用、频率、地址簿与首笔目标。
// 内部归集不走本闸。金额为零、低于最小额、小数位超过 18 或超出账本量级时拒绝。
// 日占用与频率的查询失败按拒绝处理，不得在数字未知时放行。
// 失败与已回滚的单据不占日限额，也不计入频率。
// 出金地址簿：总开关打开，或该用户在该链上已有簿内地址时，目标必须命中地址簿。
// 簿是空的且总开关关闭时，不按地址簿拦截。计数出错不得放行。
// 内部划转不出站，不查链上地址簿，但对手方不得为空，也不得是本人。

   func (r *RiskEngine) Evaluate(ctx context.Context, st Store, tr *Transfer) (RiskDecision, error) {
	  d := RiskDecision{Flags: []string{}}
	   if tr.Amount.LessThanOrEqual(decimal.Zero) {
		   return rejectGate("amount", "amount must be positive", nil), nil
	   }
	  if tr.Direction == HotColdSweep || tr.Direction == HotWarmSweep || tr.Direction == WarmColdSweep {
		   return d, nil
	  }
	  if !r.minAmount.IsZero() && tr.Amount.LessThan(r.minAmount) {
		   return rejectGate("amount", fmt.Sprintf("below min amount %s", r.minAmount), nil), nil
	  }
	  if tr.Amount.Exponent() < -18 {
		  return rejectGate("amount", ErrPrecision.Error(), nil), nil
	  }
	  if ExceedsLedgerMagnitude(tr.Amount) {
		  return rejectGate("amount", ErrAmountOverflow.Error(), nil), nil
	  }
	  if tr.Amount.GreaterThan(r.maxSingle) {
		  return rejectGate("amount", fmt.Sprintf("exceeds max single transfer %s", r.maxSingle), nil), nil
	  }
	  if st == nil {
		  storeGate("store")
		  return d, fmt.Errorf("risk store unavailable")
	   }
	   dailyCap := r.dailyFor(tr.Direction)
	   today, err := sumTodayFor(ctx, st, tr.UserID, tr.Asset, tr.Direction)
	  if err != nil {
		  storeGate("daily")
		  return d, err
	   }
	  // failed/rolled_back 不占日限额
	   if today.Add(tr.Amount).GreaterThan(dailyCap) {
		   return rejectGate("daily", fmt.Sprintf("exceeds daily %s limit %s", directionPool(tr.Direction), dailyCap), nil), nil
	   }
	   // 提现白名单只拦链上出金。内部划转不出站；充值打的是派生入站地址，不是出金目标。
	  if tr.Direction == WalletWithdraw {
		   if tr.ToAddress == "" {
			  return rejectGate("allowlist", "to_address required for withdraw", nil), nil
		   }
		  booked, err := allowlistInUse(ctx, st, tr.UserID, tr.Chain)
		  if err != nil {
			  storeGate("allowlist")
			  return d, err
		  }
		  if r.allowlist || booked {
			  ok, err := st.Allowlisted(ctx, tr.UserID, tr.Chain, tr.ToAddress)
			   if err != nil {
				  storeGate("allowlist")
				   return d, err
			   }
			  if !ok {
				   return rejectGate("allowlist", "address not on allowlist", nil), nil
			  }
		   }
		  first, err := isFirstDestination(ctx, st, tr)
		   if err != nil {
			  storeGate("allowlist")
			  return d, err
		   }
		  if first {
			   d.Flags = append(d.Flags, "first_destination")
			   if r.firstDestHalfCap {
				   half := r.maxSingle.Div(decimal.NewFromInt(2))
				   if tr.Amount.GreaterThan(half) {
					  return rejectGate("amount", fmt.Sprintf("first destination exceeds half max_single %s", half), d.Flags), nil
				  }
			  }
		   }
	  }
	  if tr.Direction == SpotSend {
		   dest := strings.TrimSpace(tr.ToAddress)
		   if dest == "" {
			  dest = strings.TrimSpace(tr.ToAddressHint)
		  }
		  if dest == "" {
			  return rejectGate("allowlist", "counterparty required", nil), nil
		  }
		  if dest == tr.UserID {
			  return rejectGate("allowlist", "cannot send to self", nil), nil
		  }
	   }
	   if tr.Amount.GreaterThanOrEqual(r.suspicious) {
		  d.Flags = append(d.Flags, "suspicious_amount")
	  }
	  maxVel := r.velocityFor(tr.Direction)
	  if maxVel > 0 {
		   cutoff := time.Now().UTC().Add(-r.velWindow)
		  n, err := countVelocity(ctx, st, tr.UserID, tr.Direction, cutoff)
		  if err != nil {
			   storeGate("velocity")
			   return d, err
		   }
		  if n >= maxVel {
			  return rejectGate("velocity", fmt.Sprintf("exceeds %s velocity %d/%s", directionPool(tr.Direction), maxVel, r.velWindow), d.Flags), nil
		  }
		  if n >= maxVel/2 && maxVel >= 4 {
			   d.Flags = append(d.Flags, "velocity_elevated")
		  }
	   }
	  return d, nil
   }

   func (r *RiskEngine) dailyFor(dir Direction) decimal.Decimal {
	  switch {
	   case dir == WalletWithdraw:
		   return r.dailyWithdraw
	   case dir == WalletDeposit:
		  return r.dailyDeposit
	  default:
		   return r.daily
	   }
   }

   func (r *RiskEngine) velocityFor(dir Direction) int {
	  switch {
	  case dir == WalletWithdraw:
		  return r.maxWithdrawVel
	  case dir == WalletDeposit:
		   return r.maxDepositVel
	  default:
		  return r.maxVel
	   }
  }

   func (r *RiskEngine) SuspiciousRequiresAdvanced() bool {
	  return r != nil && r.suspiciousRequiresAdvanced
  }

   func directionPool(dir Direction) string {
	   switch dir {
	   case WalletWithdraw:
		  return "withdraw"
	  case WalletDeposit:
		   return "deposit"
	  default:
		  return "internal"
	   }
   }

   func sameVelocityPool(a, b Direction) bool {
	   return directionPool(a) == directionPool(b)
   }

// allowlistInUse 判断该用户在该链上是否已经有出金地址簿。
// 条数大于 0 时，本次出金地址必须命中地址簿。
// 计数出错时返回错误，调用方不得放行。存储未提供计数时视为未启用，沿用总开关。
func allowlistInUse(ctx context.Context, st Store, userID, chain string) (bool, error) {
	c, ok := st.(interface {
		AllowlistCount(context.Context, string, string) (int, error)
	})
	if !ok {
		return false, nil
	}
	n, err := c.AllowlistCount(ctx, userID, chain)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// countVelocity 统计时间窗内、同一方向池、尚未失败的单据笔数。
// 充值、出金、内部划转各自计数，不得互相占用对方的频率。
// 存储能按条件计数时直接计数。否则拉列表。列表被截断时返回错误。
// 频率闸不得按残缺名单放行。
func countVelocity(ctx context.Context, st Store, userID string, dir Direction, since time.Time) (int, error) {
	if st == nil {
		return 0, fmt.Errorf("risk store unavailable")
	}
	if c, ok := st.(interface {
		CountSince(context.Context, string, Direction, time.Time) (int, error)
	}); ok {
		return c.CountSince(ctx, userID, dir, since)
	}
	list, err := transfersForVelocity(ctx, st, userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range list {
		if t.State == StateFailed || t.State == StateRolledBack {
			continue
		}
		if !sameVelocityPool(t.Direction, dir) {
			continue
		}
		if t.CreatedAt.After(since) {
			n++
		}
	}
	return n, nil
}

// transfersForVelocity 按用户取出单据，直到本次取回的条数小于上限。
// 上限从 128 加倍到 8192。仍然取满则认定窗口不完整并返回错误，调用方必须拒绝本笔。
  func transfersForVelocity(ctx context.Context, st Store, userID string) ([]*Transfer, error) {
	   if st == nil {
		   return nil, fmt.Errorf("risk store unavailable")
	   }
	   limit := 128
	   for {
		   list, err := st.ListUserTransfers(ctx, userID, limit)
		   if err != nil {
			   return nil, err
		   }
		   if len(list) < limit {
			   return list, nil
		   }
		   if limit >= 8192 {
			   return nil, fmt.Errorf("velocity window incomplete")
		   }
		   limit *= 2
	   }
   }

  func sumTodayFor(ctx context.Context, st Store, userID, asset string, dir Direction) (decimal.Decimal, error) {
	   if st == nil {
		   return decimal.Zero, fmt.Errorf("risk store unavailable")
	   }
	  if s, ok := st.(interface {
		  SumTodayByDirection(context.Context, string, string, Direction) (decimal.Decimal, error)
	   }); ok {
		  return s.SumTodayByDirection(ctx, userID, asset, dir)
	  }
	   // 存储未提供分方向日汇总时，用单据列表按同一口径加总。列表不完整时直接失败。
	   list, err := transfersForVelocity(ctx, st, userID)
	  if err != nil {
		  return decimal.Zero, err
	  }
	   day := time.Now().UTC().Truncate(24 * time.Hour)
	   sum := decimal.Zero
	  pool := directionPool(dir)
	   for _, t := range list {
		  if t.Asset != asset || t.CreatedAt.Before(day) {
			   continue
		  }
		   if t.State == StateFailed || t.State == StateRolledBack {
			  continue
		  }
		  if directionPool(t.Direction) != pool {
			   continue
		  }
		   sum = sum.Add(t.Amount)
	   }
	   return sum, nil
  }

// LookalikeInbound 判断出金地址是否与本人入站地址的前六位和后六位同时相同。
// 完全相同的地址不在这里处理，由上层按本人入站地址拒绝。
// 命中则拒绝该笔出金，不得进入冻结。
  func LookalikeInbound(chain, dest, own string) bool {
	  dest, own = strings.TrimSpace(dest), strings.TrimSpace(own)
	   if dest == "" || own == "" || sameChainAddress(chain, dest, own) {
		   return false
	   }
	   if wallet.IsEVM(chain) || strings.EqualFold(chain, "EVM") {
		  a := evmHex(own)
		  b := evmHex(dest)
		   if len(a) < 12 || len(b) < 12 {
			  return false
		   }
		  return a[:6] == b[:6] && a[len(a)-6:] == b[len(b)-6:]
	   }
	  if len(own) < 12 || len(dest) < 12 {
		   return false
	   }
	  return strings.EqualFold(own[:6], dest[:6]) && strings.EqualFold(own[len(own)-6:], dest[len(dest)-6:])
   }

  func evmHex(addr string) string {
	  addr = strings.ToLower(strings.TrimSpace(addr))
	  return strings.TrimPrefix(addr, "0x")
  }

// PassThroughHit 刚入账的充值又按高比例提出，按过桥/洗钱闸硬拒。

  func PassThroughHit(ctx context.Context, st Store, tr *Transfer, window time.Duration, bps int) bool {
	  if tr == nil || tr.Direction != WalletWithdraw || st == nil || window <= 0 || bps <= 0 {
		   return false
	  }
	  list,
		   err := st.ListUserTransfers(ctx, tr.UserID, 80)
	  if err != nil {
		  return true
	   }
	  cutoff := time.Now().UTC().Add(-window)
	  ratio := decimal.NewFromInt(int64(bps)).Div(decimal.NewFromInt(10000))
	   for _, t := range list {
		   if t == nil || t.ID == tr.ID || t.Direction != WalletDeposit || t.Asset != tr.Asset {
			   continue
		  }
		  if t.State != StateCredited {
			   continue
		   }
		  if t.CreatedAt.Before(cutoff) {
			   continue
		  }
		  if tr.Amount.GreaterThanOrEqual(t.Amount.Mul(ratio)) {
			  return true
		   }
	   }
	   return false
   }

   func isFirstDestination(ctx context.Context, st Store, tr *Transfer) (bool, error) {
	   list, err := st.ListUserTransfers(ctx, tr.UserID, 200)
	  if err != nil {
		  return false, err
	   }
	  addr := strings.TrimSpace(tr.ToAddress)
	   for _, t := range list {
		   if t.ID == tr.ID || t.Direction != WalletWithdraw {
			   continue
		   }
		  if t.State == StateFailed || t.State == StateRolledBack || t.State == StateInitiated {
			  continue
		   }
		  got := strings.TrimSpace(t.ToAddress)
		  if got == "" {
			   got = strings.TrimSpace(t.ToAddressHint)
		  }
		   if strings.EqualFold(got, addr) && strings.EqualFold(t.Chain, tr.Chain) {
			  return false, nil
		   }
	  }
	  return true, nil
   }

// allowlistChains 展开 EVM 家族：ETH / 共用 EVM 键上的地址，同样授权 L2 提到同一派生地址。

  func allowlistChains(chain string) []string {
	  chain = strings.TrimSpace(chain)
	   if !wallet.IsEVM(chain) {
		   return []string{chain}
	   }
	   out := []string{chain}
	  for _, c := range []string{"ETH", "EVM"} {
		   if c != chain {
			  out = append(out, c)
		   }
	  }
	   return out
  }
