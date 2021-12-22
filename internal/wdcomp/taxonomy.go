package wdcomp

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Decision 分类结果。MayAuto 已经扣过热态，策略表不能单独把它打回 true。

type Decision struct {
	Code FailureCode
	ReasonCode string
	UserText   string
	MayAuto bool
	ForceHuman  bool
	Live        bool
	Blocked     bool
	Unknown     bool
	Compensable string
	Filled    string
	InFlight  string
	Protected string
	Fee string
	OccRelease string
	PolicyVersion int
	Timeout       int
	Dual bool
}

  func normalize(in FailureFact) (FailureFact, amounts, error) {
	  f := in
	  f.Source = strings.TrimSpace(f.Source)
	   f.TransferID = strings.TrimSpace(f.TransferID)
	  f.UserID = strings.TrimSpace(f.UserID)
	  f.Asset = strings.TrimSpace(f.Asset)
	   f.Chain = strings.TrimSpace(f.Chain)
	   f.TxHash = strings.TrimSpace(f.TxHash)
	  f.ShardID = strings.TrimSpace(f.ShardID)
	  f.ShardPlanID = strings.TrimSpace(f.ShardPlanID)
	  f.BroadcastStatus = strings.ToLower(strings.TrimSpace(f.BroadcastStatus))
	   f.ChainState = strings.ToLower(strings.TrimSpace(f.ChainState))
	  f.Code = strings.TrimSpace(f.Code)
	  f.IdempotencyKey = strings.TrimSpace(f.IdempotencyKey)
	  if f.TransferID == "" || f.UserID == "" || f.Asset == "" {
		   return FailureFact{}, amounts{}, ErrFact
	  }
	   if f.Source == "" {
		  f.Source = SourceManual
	  }
	  fee, err := parseOpt(f.FeeReserved)
	  if err != nil {
		   return FailureFact{}, amounts{}, err
	   }
	  if strings.TrimSpace(f.Amount) == "" {
		   if fee.IsPositive() {
			  f.Amount = "0"
		   } else {
			   return FailureFact{}, amounts{}, ErrAmount
		   }
	   }
	   total, err := parseOpt(f.Amount)
	  if err != nil {
		  return FailureFact{}, amounts{}, err
	   }
	   filled, err := parseOpt(f.FilledAmount)
	  if err != nil {
		   return FailureFact{}, amounts{}, err
	  }
	   inflight,
		  err := parseOpt(f.InFlightAmount)
	   if err != nil {
		  return FailureFact{}, amounts{}, err
	  }
	   if filled.Cmp(total) > 0 || inflight.Cmp(total) > 0 {
		  return FailureFact{},
			   amounts{}, ErrAmount
	   }
	  rest, err := total.Sub(filled)
	   if err != nil {
		  return FailureFact{}, amounts{}, ErrAmount
	  }
	   rest, err = rest.Sub(inflight)
	  if err != nil {
		   return FailureFact{}, amounts{}, ErrAmount
	  }
	  var failed money.Amount
	  if strings.TrimSpace(f.FailedAmount) == "" {
		  failed = rest
	   } else {
		   failed, err = parseOpt(f.FailedAmount)
		  if err != nil {
			   return FailureFact{}, amounts{}, err
		  }
		  if failed.Cmp(rest) > 0 {
			  return FailureFact{}, amounts{}, ErrAmount
		   }
	   }
	   occSrc := f.Occupancy
	   if strings.TrimSpace(occSrc) == "" {
		  occSrc = f.OccupancySnapshot
	  }
	   if strings.TrimSpace(occSrc) == "" {
		  occSrc = f.FrozenSnapshot
	  }
	  if strings.TrimSpace(occSrc) == "" {
		  occSrc = total.String()
	   }
	  occ, err := parseOpt(occSrc)
	   if err != nil {
		  return FailureFact{}, amounts{}, err
	  }
	   protected,
		   err := filled.Add(inflight)
	  if err != nil {
		   return FailureFact{}, amounts{}, ErrAmount
	  }
	   occRelease := money.Zero()
	   if failed.IsPositive() && occ.IsPositive() {
		   occRelease = minAmt(occ, failed)
	   }
	   f.Amount = total.String()
	  f.FilledAmount = filled.String()
	   f.FailedAmount = failed.String()
	  f.InFlightAmount = inflight.String()
	  f.FeeReserved = fee.String()
	  f.Occupancy = occ.String()
	  if f.IdempotencyKey == "" {
		  f.IdempotencyKey = strings.Join([]string{f.Source, f.TransferID, f.ShardID, f.TxHash}, "|")
	   }
	  return f, amounts{
		  total: total, filled: filled, failed: failed, inflight: inflight,
		  fee: fee,
		  occ: occ, protected: protected, compensable: failed, occRelease: occRelease,
	  }, nil
   }

  func fingerprintOf(f FailureFact) string {
	   return strings.Join([]string{
		  f.Source, f.TransferID, f.UserID, f.Asset,
		  f.Amount,
		  f.FilledAmount, f.FailedAmount, f.InFlightAmount,
		   f.FeeReserved, f.TxHash, f.ShardID, f.BroadcastStatus,
	  }, "|")
   }

// IsLiveHash 链上可见哈希。空值与 internal: 前缀都不是 live。
  func IsLiveHash(hash string) bool {
	   h := strings.TrimSpace(hash)
	   if h == "" || strings.HasPrefix(h, "internal:") {
		   return false
	   }
	   return true
   }

  func chainHot(status string) bool {
	   switch strings.ToLower(strings.TrimSpace(status)) {
	  case "broadcast", "confirming", "reorged", "confirmed", "chain_submitted":
		   return true
	   default:
		  return false
	  }
   }

  func chainUnknown(state string) bool {
	   switch strings.ToLower(strings.TrimSpace(state)) {
	  case "unknown", "unclear":
		  return true
	  default:
		  return false
	  }
   }

   func shardSuccess(f FailureFact, a amounts) bool {
	   if f.Source != SourceShard {
		   return false
	  }
	  return a.failed.IsZero() && a.inflight.IsZero() && a.total.IsPositive() && a.filled.Equal(a.total) && a.fee.IsZero()
   }

   func signFailReason(reason string) bool {
	  r := strings.ToLower(reason)
	  return strings.Contains(r, "sign fail") || strings.Contains(r, "sign_fail") ||
		  strings.Contains(r, "signing failed") || strings.Contains(reason, "签名失败")
  }

   func rejectReason(reason string) bool {
	  r := strings.ToLower(reason)
	   return strings.Contains(r, "reject") || strings.Contains(r, "广播拒绝")
   }

  func partial(a amounts) bool {
	   return a.filled.IsPositive() && a.failed.IsPositive()
   }

// classify 生成失败码与可自动性。
// 功能：热态覆盖业务码的自动位。
// 分片部分成交在本段无热态时仍可自动，但只补偿失败金额。
// 不变量：live hash 时 MayAuto 必为 false。
   func classify(f FailureFact, a amounts, book *PolicyBook) (Decision, error) {
	  if book == nil {
		   return Decision{}, ErrUnavailable
	   }
	   if shardSuccess(f, a) {
		  return Decision{}, ErrSuccessPath
	  }
	  if a.compensable.IsZero() && a.fee.IsZero() {
		   return Decision{}, ErrNothingToCompensate
	  }
	   live := IsLiveHash(f.TxHash)
	   blocked := chainHot(f.BroadcastStatus)
	   unknown := chainUnknown(f.ChainState)
	  if f.Code == string(CodeLiveHash) {
		   live = true
	  }
	   if f.Code == string(CodeUnknownChain) {
		  unknown = true
	   }
	  hot := live || blocked || unknown
	   code := businessCode(f, a)
	   if hot {
		   if partial(a) {
			   code = CodeShardPartial
		  } else if live {
			  code = CodeLiveHash
		   } else {
			   code = CodeUnknownChain
		  }
	  }
	  pol, known := book.Get(code)
	  if !known {
		  return Decision{}, ErrPolicy
	   }
	   force := pol.ForceHuman || hot
	  may := pol.AutoAllowed && !force && !hot
	  if live || blocked {
		   may = false
		   force = true
	  }
	   d := Decision{
		   Code: code, ReasonCode: reasonCode(code), UserText: userText(code),
		   MayAuto: may, ForceHuman: force, Live: live, Blocked: blocked || unknown, Unknown: unknown,
		  Compensable: a.compensable.String(), Filled: a.filled.String(), InFlight: a.inflight.String(),
		  Protected: a.protected.String(), Fee: a.fee.String(), OccRelease: a.occRelease.String(),
		   PolicyVersion: pol.Version,
		  Timeout: pol.TimeoutSec, Dual: pol.DualConfirm || hot,
	   }
	  if d.Timeout <= 0 {
		   d.Timeout = 900
	  }
	   return d, nil
  }

   func businessCode(f FailureFact, a amounts) FailureCode {
	   if c := FailureCode(f.Code); knownBusiness(c) {
		  return c
	   }
	   if f.Source == SourceShard {
		   if partial(a) {
			  return CodeShardPartial
		   }
		  if a.failed.IsPositive() {
			  return CodeShardFail
		   }
		   if a.fee.IsPositive() {
			  return CodeFeeOrphan
		   }
	  }
	  if a.compensable.IsZero() && a.fee.IsPositive() {
		  return CodeFeeOrphan
	   }
	  switch f.Source {
	  case SourcePolicy:
		   return CodePolicyReject
	  case SourceCancel:
		  return CodeHumanCancel
	   default:
		  if signFailReason(f.Reason) {
			   return CodeSignFail
		   }
		   if rejectReason(f.Reason) {
			   return CodeBroadcastReject
		  }
		  return CodeStuckAbandon
	   }
  }

   func knownBusiness(c FailureCode) bool {
	   switch c {
	  case CodeSignFail, CodeBroadcastReject, CodeStuckAbandon, CodePolicyReject,
		   CodeHumanCancel, CodeShardPartial, CodeShardFail, CodeFeeOrphan:
		   return true
	  default:
		  return false
	   }
  }

   func KnownCodes() []FailureCode {
	  return []FailureCode{
		  CodeSignFail, CodeBroadcastReject, CodeStuckAbandon, CodePolicyReject,
		  CodeHumanCancel, CodeShardPartial, CodeShardFail, CodeFeeOrphan,
		   CodeUnknownChain, CodeLiveHash,
	  }
  }

   func reasonCode(c FailureCode) string {
	   switch c {
	   case CodeSignFail:
		  return "WD_SIGN_FAIL_NO_HASH"
	   case CodeBroadcastReject:
		  return "WD_BROADCAST_REJECTED"
	  case CodeStuckAbandon:
		  return "WD_STUCK_ABANDON"
	  case CodePolicyReject:
		  return "WD_POLICY_REJECT"
	   case CodeHumanCancel:
		  return "WD_HUMAN_CANCEL"
	  case CodeShardPartial:
		   return "WD_SHARD_REMAINDER"
	   case CodeShardFail:
		  return "WD_SHARD_FAIL"
	   case CodeFeeOrphan:
		   return "WD_FEE_RESERVE_RELEASE"
	   case CodeUnknownChain:
		   return "WD_CHAIN_STATE_UNKNOWN"
	   case CodeLiveHash:
		  return "WD_LIVE_HASH_HOLD"
	   default:
		  return "WD_FAILURE_UNSPECIFIED"
	  }
  }

  func userText(c FailureCode) string {
	   switch c {
	  case CodeSignFail:
		   return "签名失败且未上链，冻结退回可用"
	  case CodeBroadcastReject:
		  return "广播被拒绝且未上链，冻结退回可用"
	   case CodeStuckAbandon:
		  return "出金已放弃且未上链，冻结退回可用"
	  case CodePolicyReject:
		   return "策略拒绝出金，冻结退回可用"
	   case CodeHumanCancel:
		  return "出金已取消且未上链，冻结退回可用"
	  case CodeShardPartial:
		   return "已成交分片保持不动，仅失败部分进入补偿"
	   case CodeShardFail:
		   return "分片出金失败且未上链，剩余冻结退回可用"
	  case CodeFeeOrphan:
		  return "本金不回滚，仅释放残留费预留"
	   case CodeUnknownChain:
		  return "链上状态不明，冻结保持，需人工处理"
	  case CodeLiveHash:
		  return "链上已有交易哈希，冻结保持，需人工处理"
	   default:
		  return "出金失败待处理"
	  }
  }
// temporary guard removed after the production check
