package ledger

import (
	"context"
	"strings"
)

const FenceOpReorg FenceOp = "reorg"

const (
	fenceAdmit = "admit"
	fenceStale = "stale"
	fenceIllegal = "illegal"
)

// FenceInput 一次围栏判定的输入。LeaseToken 为空表示调用方还没拿到租约，不比较租约。

type FenceInput struct {
	Op    FenceOp
	State State
	Epoch    int64
	Token    string
	RowEpoch int64
	RowToken string
	LeaseToken string
	Direction  Direction
	TxHash     string
}

// FenceDecision 矩阵结论。ChainGuard 为真时，解冻仍必须经过链上存活检查。

type FenceDecision struct {
	Verdict string `json:"verdict"`
	Reason string `json:"reason"`
	Rule string `json:"rule"`
	ChainGuard bool `json:"chain_guard,omitempty"`
}

// FenceRule 运维可读的一条规则。判定函数与这张表使用同一套规则号。
type FenceRule struct {
	ID      string `json:"id"`
	Op      string `json:"op"`
	When    string `json:"when"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

// FenceRules 行级围栏规则表，供运维读面，不参与写。
   func FenceRules() []FenceRule {
	   return []FenceRule{
		  {ID: "claim-required", Op: "*", When: "epoch 或 token 为空", Verdict: fenceStale, Reason: "claim required"},
		  {ID: "lease-not-row", Op: "*", When: "声明的令牌等于当前租约，但不等于行上令牌", Verdict: fenceStale, Reason: "lease token is not the row fence"},
		   {ID: "token-mismatch", Op: "*", When: "epoch 或 token 与行上不一致", Verdict: fenceStale, Reason: "token mismatch"},
		  {ID: "advance-submitted", Op: string(FenceOpAdvance), When: "chain_submitted 或 confirmed，令牌一致", Verdict: fenceAdmit, Reason: "advance submitted"},
		   {ID: "advance-other", Op: string(FenceOpAdvance), When: "其他状态", Verdict: fenceIllegal, Reason: "advance only submitted"},
		   {ID: "unfreeze-terminal", Op: string(FenceOpUnfreeze), When: "credited 或 rolled_back", Verdict: fenceIllegal, Reason: "terminal cannot unfreeze"},
		   {ID: "unfreeze-open", Op: string(FenceOpUnfreeze), When: "其余未终态且令牌一致", Verdict: fenceAdmit, Reason: "unfreeze open"},
		   {ID: "unfreeze-live", Op: string(FenceOpUnfreeze), When: "已提交且链上 hash 非内部占位", Verdict: fenceAdmit, Reason: "chain guard still applies"},
		  {ID: "reorg-credited", Op: string(FenceOpReorg), When: "credited 且令牌一致", Verdict: fenceAdmit, Reason: "reorg unwind credited"},
		  {ID: "reorg-other", Op: string(FenceOpReorg), When: "非 credited", Verdict: fenceIllegal, Reason: "reorg only credited"},
		   {ID: "lease-rotated", Op: "*", When: "行令牌一致但与租约令牌不同", Verdict: fenceAdmit, Reason: "lease rotated, row fence held"},
	  }
   }

// decideFence 失败即关判定。不改单据，不盖章。
   func decideFence(in FenceInput) FenceDecision {
	  token := strings.TrimSpace(in.Token)
	   rowToken := strings.TrimSpace(in.RowToken)
	   lease := strings.TrimSpace(in.LeaseToken)
	  if in.Epoch <= 0 || token == "" {
		  return FenceDecision{Verdict: fenceStale, Reason: "claim required", Rule: "claim-required"}
	  }
	  if lease != "" && token == lease && token != rowToken {
		  return FenceDecision{Verdict: fenceStale, Reason: "lease token is not the row fence", Rule: "lease-not-row"}
	  }
	  if in.Epoch != in.RowEpoch || token != rowToken {
		  return FenceDecision{Verdict: fenceStale, Reason: "token mismatch", Rule: "token-mismatch"}
	  }
	  switch in.Op {
	  case FenceOpAdvance:
		   if submittedTransfer(in.State) {
			   return FenceDecision{Verdict: fenceAdmit, Reason: "advance submitted", Rule: "advance-submitted"}
		   }
		  return FenceDecision{Verdict: fenceIllegal, Reason: "advance only submitted", Rule: "advance-other"}
	   case FenceOpUnfreeze:
		   if in.State == StateCredited || in.State == StateRolledBack {
			  return FenceDecision{Verdict: fenceIllegal, Reason: "terminal cannot unfreeze", Rule: "unfreeze-terminal"}
		  }
		  d := FenceDecision{Verdict: fenceAdmit, Reason: "unfreeze open", Rule: "unfreeze-open"}
		   if liveChain(in) {
			   d.ChainGuard = true
			   d.Reason = "chain guard still applies"
			  d.Rule = "unfreeze-live"
		   }
		  return d
	   case FenceOpReorg:
		  if in.State == StateCredited {
			   note := "reorg unwind credited"
			   rule := "reorg-credited"
			   if lease != "" && lease != token {
				  note = "lease rotated, row fence held"
				   rule = "lease-rotated"
			  }
			   return FenceDecision{Verdict: fenceAdmit, Reason: note, Rule: rule}
		   }
		   return FenceDecision{Verdict: fenceIllegal, Reason: "reorg only credited", Rule: "reorg-other"}
	   default:
		   return FenceDecision{Verdict: fenceIllegal, Reason: "unknown fence op", Rule: "op-unknown"}
	   }
  }

  func liveChain(in FenceInput) bool {
	  if !in.Direction.OnChain() {
		   return false
	   }
	   if in.State != StateChainSubmitted && in.State != StateConfirmed {
		   return false
	  }
	   hash := strings.TrimSpace(in.TxHash)
	  if hash == "" || IsInternalTxHash(hash) {
		  return false
	  }
	  return true
  }

  func (s *Service) gateClaim(ctx context.Context, id string, op FenceOp, epoch int64, token string, cur *Transfer) error {
	  if cur == nil {
		   return ErrNotFound
	   }
	   d := decideFence(FenceInput{
		  Op: op, State: cur.State, Epoch: epoch, Token: token,
		   RowEpoch: cur.FenceEpoch, RowToken: cur.FenceToken,
		  Direction: cur.Direction, TxHash: cur.TxHash,
	   })
	  switch d.Verdict {
	  case fenceStale:
		  s.rejectFence(ctx, id, op, epoch, token, d.Reason)
		   return ErrStaleFence
	   case fenceIllegal:
		  return ErrIllegalTransition
	   default:
		  return nil
	   }
   }
