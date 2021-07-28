package intxfergate

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ─── MisrouteCase ──────────────────────────────────────────────────────────
// MisrouteCaseState 误路由案例状态机。
// 状态机：
// open ──(scanner 发现偏差)──▶ open（初始）
// open ──(proof seal 工作)──▶ proven
// proven ──(管理员 Confirm)──▶ closed
// open ──(管理员 Dismiss)──▶ closed
type MisrouteCaseState string

const (
	MisrouteCaseOpen MisrouteCaseState = "open"
	MisrouteCaseProven MisrouteCaseState = "proven"
	MisrouteCaseClosed MisrouteCaseState = "closed"
)

// MisrouteCase 贷记偏差案例。
// 触发场景：
// 1. 解析快照 snap_to_user != 实际贷记 actual_to_user。
// 2. 解析后目录改绑（userdir 重新绑定邮箱/手机到新用户），原快照 to 依旧有效但日志偏差。
// 字段不变式：
// - SnapID + LedgerID 唯一确定一个事件；同对不得重复开案。
// - ProofID 在状态 proven 后填写；closed 无需 ProofID 当 Dismiss 时。
type MisrouteCase struct {
	ID       string // UUID v4
	SnapID   string // ResolveSnap.ID
	LedgerID string // ledger.Transfer.ID（实际扣款单）
	FromUserID string // 发起方
	SnapToUserID string // 快照锁定收款方
	ActualToUID string // 实际贷记用户（若目录改绑则不同）
	Asset   string // 币种
	Amount  string // decimal string
	IdemKey string // Send 幂等键
	State MisrouteCaseState // 生命周期
	ProofID     string // 关联 XferEffectProof.ID（proven 后填）
	ScanNote    string // 扫描器注释（说明偏差原因）
	ConfirmedBy string // 管理员 ID（closed 后填）
	ConfirmedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt time.Time
}

// Transition 状态机转换校验。
  func (m *MisrouteCase) Transition(next MisrouteCaseState) error {
	   valid := map[MisrouteCaseState][]MisrouteCaseState{
		  MisrouteCaseOpen:   {MisrouteCaseProven, MisrouteCaseClosed},
		   MisrouteCaseProven: {MisrouteCaseClosed},
		   MisrouteCaseClosed: {},
	  }
	   for _, s := range valid[m.State] {
		   if s == next {
			  return nil
		  }
	   }
	  return fmt.Errorf("misroute case: illegal transition %s→%s", m.State, next)
  }

// ─── XferEffectProof ───────────────────────────────────────────────────────
// XferEffectProof 站内转账效果证明。
// 证明语义：
// - proof_hash = SHA-256(snap_id ‖ from_user_id ‖ to_user_id ‖ amount ‖ ledger_id)
// - 证明缺失（或 proof_hash 不匹配）时禁止将对应 Send 标为 completed。
// - 不上链；不含链上 tx_hash。
// 字段不变式：
// - SnapID + LedgerID 唯一对应一次 SpotSend；同对不得重复开证。
// - ProofHash 不得为空；签名时刻 SealedAt 不得晚于 CreatedAt+10s（系统时钟偏差容忍）。
type XferEffectProof struct {
	ID string // UUID v4
	SnapID  string // ResolveSnap.ID
	LeaseID string // DebitLease.ID
	LedgerID   string // ledger.Transfer.ID
	FromUserID string // 发起方
	ToUserID string // 收款方（快照锁定）
	Asset    string // 币种
	Amount   string // decimal string
	IdemKey  string // 幂等键
	ProofHash string // SHA-256 hex（64 位）
	SealedAt   time.Time // 证明固化时刻
	FenceEpoch int64     // 隔离纪元（与 xfer 半截 run_id 键空间隔离用）
	CreatedAt  time.Time
}

// ComputeHash 计算证明摘要。
// 输入：snapID ‖ ":" ‖ fromUserID ‖ ":" ‖ toUserID ‖ ":" ‖ amount ‖ ":" ‖ ledgerID

   func ComputeHash(snapID, fromUserID, toUserID, amount, ledgerID string) string {
	   src := fmt.Sprintf("%s:%s:%s:%s:%s", snapID, fromUserID, toUserID, amount, ledgerID)
	  h := sha256.Sum256([]byte(src))
	  return fmt.Sprintf("%x", h)
   }

// Verify 校验证明摘要是否与字段一致。
   func (p *XferEffectProof) Verify() error {
	   if p == nil {
		  return fmt.Errorf("effect proof nil")
	  }
	  expected := ComputeHash(p.SnapID,
		   p.FromUserID, p.ToUserID, p.Amount, p.LedgerID)
	  if p.ProofHash != expected {
		   return fmt.Errorf("effect proof hash mismatch: got=%s expected=%s", p.ProofHash, expected)
	   }
	  return nil
   }

// ─── SendState FSM ─────────────────────────────────────────────────────────
// SendState 站内转账业务级状态机（覆盖于 ledger.Transfer 之上）。
// 状态机：
// resolving ──(snap ok + lease ok)──▶ debiting
// debiting ──(SpotSend ok)──▶ sealing
// sealing ──(EffectProof 生成)──▶ completed
// resolving ──(ambiguous/gate fail)──▶ blocked
// debiting ──(SpotSend fail)──▶ blocked
// sealing ──(proof 缺失超时)──▶ blocked
type SendState string

const (
	SendStateResolving SendState = "resolving"
	SendStateDebiting  SendState = "debiting"
	SendStateSealing   SendState = "sealing"
	SendStateCompleted SendState = "completed"
	SendStateBlocked   SendState = "blocked"
)

// SendRecord 业务级转账追踪记录（叠加于 intxfergate.Store 中）。
type SendRecord struct {
	ID         string // UUID v4
	IdemKey    string // 幂等键
	FromUserID string // 发起方
	Asset      string // 币种
	Amount string // decimal string
	State  SendState // 业务状态
	SnapID string    // ResolveSnap.ID（resolving 完成后填）
	LeaseID string // DebitLease.ID（debiting 开始后填）
	LedgerID string // ledger.Transfer.ID（SpotSend 完成后填）
	ProofID  string // XferEffectProof.ID（sealing 完成后填）
	BlockedAt   *time.Time
	BlockReason string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Transition 校验业务状态机转换。
  func (s *SendRecord) Transition(next SendState) error {
	   valid := map[SendState][]SendState{
		  SendStateResolving: {SendStateDebiting,
			   SendStateBlocked},
		   SendStateDebiting: {SendStateSealing, SendStateBlocked},
		   SendStateSealing: {SendStateCompleted,
			  SendStateBlocked},
		  SendStateCompleted: {},
		  SendStateBlocked:   {},
	  }
	  for _, st := range valid[s.State] {
		   if st == next {
			   return nil
		  }
	  }
	   return fmt.Errorf("send record: illegal transition %s→%s", s.State, next)
   }
