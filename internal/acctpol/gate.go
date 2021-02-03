package acctpol

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// GateRequest hop 准入检查请求。
type GateRequest struct {
	BizKind BizKind `json:"biz_kind"`
	// From/To 同 subjtree.Node.ID，与 ledger.AccountType 字符串对应。
	From string `json:"from"`
	To string `json:"to"`
	UserID string `json:"user_id,omitempty"`
}

// GateResult hop 准入结果。

type GateResult struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	PolicyVersion int `json:"policy_version"`
}

// Gate hop 前准入检查器（并发安全）。

type Gate struct {
	engine *Engine
	dualLog *DualReadLog
}

// NewGate 创建 gate；engine 不得为 nil。
   func NewGate(engine *Engine, dualLog *DualReadLog) *Gate {
	  return &Gate{engine: engine, dualLog: dualLog}
  }

// Check 检查指定 hop 是否被当前策略允许。
// 返回 GateResult；调用方在 Allow=false 时必须拒绝 hop，不得接着执行。
   func (g *Gate) Check(ctx context.Context, req GateRequest) (GateResult, error) {
	  evalReq := EvalRequest{
		  BizKind: req.BizKind,
		   HopFrom: req.From,
		  HopTo:  req.To,
		  UserID: req.UserID,
	  }
	   res,
		  err := g.engine.Eval(ctx, evalReq)
	  if err != nil {
		   return GateResult{}, fmt.Errorf("acctpol gate: %w", err)
	   }
	  gr := GateResult{
		  Allow: res.Allow,
		  Reason:        res.Reason,
		   PolicyVersion: res.PolicyVersion,
	   }
	   if !gr.Allow {
		  reason := res.Reason
		   if reason == "" {
			   reason = "policy"
		  }
		  metrics.AcctPolGateReject.WithLabelValues(string(req.BizKind), reason).Inc()
	  }
	   // 双读观测（异步，不阻塞）
	   if g.dualLog != nil {
		  go DualReadHook(ctx, g.engine, evalReq, res, g.dualLog)
	   }
	  return gr, nil
  }
