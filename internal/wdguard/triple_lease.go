package wdguard

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

// TripleGateHook wdguard 与 wdtriplegate 的集成钩子。

type TripleGateHook struct {
	gate *wdtriplegate.Gate
}

// NewTripleGateHook 构造集成钩子。gate=nil 时所有方法为无操作（向后兼容）。

  func NewTripleGateHook(gate *wdtriplegate.Gate) *TripleGateHook {
	   return &TripleGateHook{gate: gate}
   }

// PreflightLeaseResult Preflight 成功后的三联租约签发结果。

type PreflightLeaseResult struct {
	TripleLeaseID string `json:"triple_lease_id,omitempty"`
	ListGen       int64  `json:"list_gen,omitempty"`
	PolicyVer int  `json:"policy_ver,omitempty"`
	Issued    bool `json:"issued"`
}

// IssueAfterPreflight Preflight/CheckTriple 成功后签发三联租约。
// 参数：
// - movementID、userID、chain、dest：来自出金请求
// - openTickets：未结异常单数量（必须为 0）
// - listGen：destallow 当前世代
// - policyVer：policypack 当前代次
// - issuedBy：签发人（actor）
// 返回 PreflightLeaseResult；gate=nil 时返回空结果（不影响主路径）。

  func (h *TripleGateHook) IssueAfterPreflight(
	   movementID, userID, chain, dest string,
	   openTickets int,
	  listGen int64,
	  policyVer int,
	  issuedBy string,
   ) (PreflightLeaseResult, error) {
	  if h == nil || h.gate == nil {
		  return PreflightLeaseResult{}, nil
	  }
	  if openTickets > 0 {
		   return PreflightLeaseResult{},
			   ErrOpenTicket
	  }
	   if strings.TrimSpace(movementID) == "" {
		  return PreflightLeaseResult{}, nil
	  }
	  lease, err := h.gate.IssueLease(wdtriplegate.IssueLeaseReq{
		  MovementID: movementID,
		  UserID: userID,
		   Chain:  chain,
		   Dest:   dest,
		  TicketClear: true,
		  KYTPin:      "allow",
		   ListGen:   listGen,
		  PolicyVer: policyVer,
		   IssuedBy:   issuedBy,
		   TTLSeconds: 300,
	   })
	  if err != nil {
		   return PreflightLeaseResult{}, err
	  }
	   return PreflightLeaseResult{
		  TripleLeaseID: lease.TripleLeaseID,
		  ListGen:       lease.ListGen,
		   PolicyVer:     lease.PolicyVer,
		   Issued:        true,
	  }, nil
  }

// RequireBroadcastClear 广播前调用：世代复验绑定 + 最终校验。
// 调用方需提供：
// - movementID、tripleLeaseID：出金请求上下文
// - currentListGen：广播时实时读取的 destallow 世代
// - currentPolicyVer：广播时实时读取的策略代次
// - currentKYTPin：广播时 KYT 决策（必须为 "allow"）
// 返回 nil 表示可广播；否则返回拒绝原因（不得广播）。
   func (h *TripleGateHook) RequireBroadcastClear(
	  ctx context.Context,
	  movementID, tripleLeaseID string,
	   currentListGen int64,
	   currentPolicyVer int,
	  currentKYTPin string,
  ) error {
	  if h == nil || h.gate == nil {
		  return nil
	   }
	  // 广播前世代复验绑定
	  result, err := h.gate.Bind(wdtriplegate.BindReq{
		  MovementID:       movementID,
		   TripleLeaseID:    tripleLeaseID,
		  CurrentListGen:   currentListGen,
		  CurrentPolicyVer: currentPolicyVer,
		  CurrentKYTPin: currentKYTPin,
	   })
	   if err != nil {
		   return err
	  }
	   if !result.BroadcastAllowed {
		   return ErrTripleDrift
	  }
	  // 最终防线校验
	   return h.gate.RequireBroadcastAllowed(movementID)
  }

// ConsumeAfterBroadcast 广播成功后消费租约（可异步调用）。
   func (h *TripleGateHook) ConsumeAfterBroadcast(_ context.Context, tripleLeaseID string) error {
	  if h == nil || h.gate == nil {
		   return nil
	   }
	  if strings.TrimSpace(tripleLeaseID) == "" {
		   return nil
	  }
	  _, err := h.gate.ConsumeLease(tripleLeaseID)
	  return err
   }
