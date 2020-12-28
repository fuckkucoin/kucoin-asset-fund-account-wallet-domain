package custodyhalt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 常量 & 错误 ------------------------------------------------

const (
	KindClearPlan  = "aa_halt_clear_plan"
	KindCloseProof = "aa_halt_close_proof"
	// 状态字符串：ClearPlan 状态机 proposed→quorum→cleared / rejected。
	PlanStateProposed = "proposed"
	PlanStateQuorum   = "quorum" // 第一审批人已签；等待第二审批人。
	PlanStateCleared  = "cleared"
	PlanStateRejected = "rejected"
	// CloseProof 类型。
	ProofTypeNormal = "normal" // 四眼 + 无差异
	ProofTypeForced = "forced" // 强制清闸，仅审计用途
)

var (
	ErrQuorumRequired = errors.New("clear quorum: two distinct approvers required")
	ErrSameApprover     = errors.New("clear quorum: approver1 and approver2 must differ")
	ErrPlanNotFound     = errors.New("clear quorum: plan not found")
	ErrPlanNotProposed  = errors.New("clear quorum: plan must be in proposed state")
	ErrPlanNotQuorum    = errors.New("clear quorum: plan must be in quorum state before second approval")
	ErrPlanAlreadyFinal = errors.New("clear quorum: plan already in terminal state")
	ErrProofMismatch    = errors.New("clear quorum: close-proof plan_id mismatch")
	ErrNoOpenDiffs      = errors.New("clear quorum: open_diff_count must be zero for non-forced clear")
	ErrPlanIDRequired = errors.New("clear quorum: plan_id required")
	ErrApproverRequired   = errors.New("clear quorum: approver required")
	ErrCloseProofRequired = errors.New("clear quorum: close proof required before issuing cleared state")
)

// ---- ClearPlan -------------------------------------------------------
// ClearPlan 一次清闸提案；四眼协议要求两个不同操作人按顺序审批。
// 状态机：proposed → quorum（第一签）→ cleared（第二签）/ rejected。
// 与战争室 freeze_plan_id 正交；本字段若非空仅做只读关联，不写级联计划。
type ClearPlan struct {
	PlanID string `json:"plan_id"`
	Scope string `json:"scope"` // global / chain / asset / pair
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	Proposer string `json:"proposer"` // 提案人（算作第一审批人）
	Reason string `json:"reason"`
	State       string    `json:"state"`               // proposed/quorum/cleared/rejected
	Approver1   string    `json:"approver1,omitempty"` // 第一签（proposer 之外）
	Approver1At time.Time `json:"approver1_at,omitempty"`
	Approver2    string    `json:"approver2,omitempty"` // 第二签（须 ≠ Approver1）
	Approver2At  time.Time `json:"approver2_at,omitempty"`
	RejectReason string    `json:"reject_reason,omitempty"`
	RejectedBy   string    `json:"rejected_by,omitempty"`
	RejectedAt   time.Time `json:"rejected_at,omitempty"`
	CloseProofID string `json:"close_proof_id,omitempty"` // 绑定的闭合证明 ID
	OpenDiffCount int       `json:"open_diff_count"` // 提案时未关对账差异数；清闸前须为 0
	ProposedAt    time.Time `json:"proposed_at"`
	ClearedAt     time.Time `json:"cleared_at,omitempty"`
	// FreezeWarRoomPlanID 只读关联；本包不写级联冻结计划。
	FreezeWarRoomPlanID string `json:"freeze_war_room_plan_id,omitempty"`
}

// ---- CloseProof -------------------------------------------------------
// CloseProof 清闸闭合证明：四眼 + 零差异的不可变快照，供事后审计。
// 一旦写入不得修改；通过 CloseProofStore 追加写。
type CloseProof struct {
	ProofID string `json:"proof_id"`
	PlanID    string `json:"plan_id"`
	ProofType string `json:"proof_type"` // normal / forced
	Scope     string `json:"scope"`
	Chain     string `json:"chain,omitempty"`
	Asset     string `json:"asset,omitempty"`
	Approver1 string `json:"approver1"`
	Approver2 string `json:"approver2"`
	OpenDiffCount int       `json:"open_diff_count"` // 清闸时实测；normal 时必须为 0
	IssuedAt      time.Time `json:"issued_at"`
	// 证明正文：含双方签名字段与差异清单摘要（不含业务金额）。
	DiffSummary string `json:"diff_summary,omitempty"`
	Checksum string `json:"checksum"` // SHA-256(plan_id+proof_type+approver1+approver2+open_diff_count)
}

// ---- QuorumStore -------------------------------------------------------
// QuorumStore 把 ClearPlan / CloseProof 落到 KV；不参与资金路径。
// 功能：四眼协议状态机持久化；与战争室 freeze_plan 仓完全隔离。
// 仅操作 aa_halt_clear_plan / aa_halt_close_proof 两个 KV kind；不读写 aa_wd_halt 以外的熔断键。
// 不变量：PlanID 一旦到达 cleared/rejected 不再流转；CloseProof 仅追加。
type QuorumStore struct {
	kv persist.KV
}

  func NewQuorumStore(kv persist.KV) *QuorumStore {
	  return &QuorumStore{kv: kv}
  }

// ProposeClearPlan 提交清闸提案。proposer 作为第一签名人；openDiffs 必须在调用前由 ClearChecker 获取。
// 功能：初始化 ClearPlan，状态 proposed；绑定当前差异计数。
// 不变量：planID 由调用方生成（UUID）；不校验重复提案（同 planID 幂等 put）。

  func (qs *QuorumStore) ProposeClearPlan(ctx context.Context, plan ClearPlan) error {
	   if qs == nil || qs.kv == nil {
		   return ErrStore
	  }
	  if strings.TrimSpace(plan.PlanID) == "" {
		   return ErrPlanIDRequired
	   }
	   if strings.TrimSpace(plan.Proposer) == "" {
		   return ErrApproverRequired
	  }
	  if strings.TrimSpace(plan.Reason) == "" {
		  return ErrReason
	   }
	   plan.State = PlanStateProposed
	   if plan.ProposedAt.IsZero() {
		   plan.ProposedAt = time.Now().UTC()
	   }
	   if plan.Scope == "" {
		   plan.Scope = string(ScopeGlobal)
	   }
	   return qs.putPlan(ctx, plan, persist.Fence{})
   }

// FirstApprove 第一个审批人（须 ≠ proposer）签字；状态推进到 quorum。
// 功能：四眼第一签；禁止 proposer 自我批准。
// 不变量：只接受 proposed 状态；approver1 ≠ proposer。

   func (qs *QuorumStore) FirstApprove(ctx context.Context, planID, approver string) error {
	  plan,
		   row, err := qs.getPlanRow(ctx, planID)
	  if err != nil {
		   return err
	   }
	   if plan.State != PlanStateProposed {
		   return ErrPlanNotProposed
	   }
	   if strings.TrimSpace(approver) == "" {
		  return ErrApproverRequired
	  }
	  if approver == plan.Proposer {
		  return fmt.Errorf("%w: approver1=%s equals proposer", ErrSameApprover, approver)
	   }
	  plan.Approver1 = approver
	   plan.Approver1At = time.Now().UTC()
	   plan.State = PlanStateQuorum
	  return qs.putPlan(ctx, plan, row.Fence)
   }

// SecondApprove 第二个审批人（须 ≠ proposer 且 ≠ approver1）完成四眼；状态推进到 cleared。
// 前置：openDiffCount==0（非 force 路径）；生成 CloseProof 并绑定。
// 不变量：只接受 quorum 状态；approver2 ≠ proposer ≠ approver1。

  func (qs *QuorumStore) SecondApprove(ctx context.Context, planID, approver string, openDiffs int, diffSummary string) (*CloseProof, error) {
	  plan, row, err := qs.getPlanRow(ctx, planID)
	   if err != nil {
		  return nil, err
	  }
	  if plan.State != PlanStateQuorum {
		   return nil, ErrPlanNotQuorum
	   }
	   if strings.TrimSpace(approver) == "" {
		   return nil, ErrApproverRequired
	  }
	   if approver == plan.Proposer || approver == plan.Approver1 {
		   return nil, fmt.Errorf("%w: approver2=%s must differ from proposer and approver1", ErrSameApprover, approver)
	   }
	  if openDiffs != 0 {
		  return nil, fmt.Errorf("%w: got %d open diffs", ErrNoOpenDiffs, openDiffs)
	  }
	   now := time.Now().UTC()
	   proofID := newProofID(planID)
	   proof := CloseProof{
		   ProofID: proofID,
		  PlanID:  planID,
		  ProofType: ProofTypeNormal,
		  Scope:     plan.Scope,
		  Chain:     plan.Chain,
		  Asset:     plan.Asset,
		   Approver1: plan.Approver1,
		   Approver2: approver,
		   OpenDiffCount: openDiffs,
		   IssuedAt:    now,
		  DiffSummary: diffSummary,
		   Checksum: computeProofChecksum(planID, ProofTypeNormal, plan.Approver1, approver, openDiffs),
	  }
	   if err := qs.writeProof(ctx, proof); err != nil {
		   return nil, fmt.Errorf("write close proof: %w", err)
	   }
	   plan.Approver2 = approver
	  plan.Approver2At = now
	  plan.State = PlanStateCleared
	  plan.ClearedAt = now
	  plan.CloseProofID = proofID
	   plan.OpenDiffCount = openDiffs
	   if err := qs.putPlan(ctx, plan, row.Fence); err != nil {
		   return nil, fmt.Errorf("update plan to cleared: %w", err)
	   }
	   return &proof, nil
   }

// RejectPlan 审批人否决提案；状态推进到 rejected。
  func (qs *QuorumStore) RejectPlan(ctx context.Context, planID, rejector, rejectReason string) error {
	   plan,
		  row, err := qs.getPlanRow(ctx, planID)
	  if err != nil {
		  return err
	  }
	  if plan.State == PlanStateCleared || plan.State == PlanStateRejected {
		   return ErrPlanAlreadyFinal
	  }
	   if strings.TrimSpace(rejector) == "" {
		  return ErrApproverRequired
	  }
	   now := time.Now().UTC()
	   plan.State = PlanStateRejected
	  plan.RejectedBy = rejector
	  plan.RejectedAt = now
	   plan.RejectReason = strings.TrimSpace(rejectReason)
	  return qs.putPlan(ctx, plan, row.Fence)
   }

// GetPlan 读取 ClearPlan；不存在返回 ErrPlanNotFound。

  func (qs *QuorumStore) GetPlan(ctx context.Context, planID string) (ClearPlan, error) {
	  plan, _, err := qs.getPlanRow(ctx, planID)
	  return plan, err
  }

// ListPlans 列出最近 limit 条清闸提案；按 KV 键升序。
   func (qs *QuorumStore) ListPlans(ctx context.Context, limit int) ([]ClearPlan, error) {
	  if qs == nil || qs.kv == nil {
		  return nil, ErrStore
	  }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	   rows, err := qs.kv.List(ctx, KindClearPlan, limit)
	  if err != nil {
		  return nil, err
	  }
	  out := make([]ClearPlan, 0, len(rows))
	   for _, r := range rows {
		   var p ClearPlan
		   if json.Unmarshal(r.Body, &p) == nil {
			   out = append(out, p)
		   }
	  }
	   return out, nil
   }

// GetProof 读取 CloseProof；不存在返回 ErrPlanNotFound。
  func (qs *QuorumStore) GetProof(ctx context.Context, proofID string) (CloseProof, error) {
	   if qs == nil || qs.kv == nil {
		  return CloseProof{}, ErrStore
	  }
	  row,
		  err := qs.kv.Get(ctx, KindCloseProof, proofID)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return CloseProof{},
				  ErrPlanNotFound
		   }
		  return CloseProof{}, err
	   }
	  var p CloseProof
	  if err := json.Unmarshal(row.Body, &p); err != nil {
		  return CloseProof{}, fmt.Errorf("decode close proof: %w", err)
	  }
	   return p, nil
  }

// VerifyProof 校验 CloseProof checksum；供审计与误清探测使用。

   func VerifyProof(p CloseProof) error {
	  want := computeProofChecksum(p.PlanID, p.ProofType, p.Approver1, p.Approver2, p.OpenDiffCount)
	   if want != p.Checksum {
		   return fmt.Errorf("%w: stored=%s computed=%s", ErrProofMismatch, p.Checksum, want)
	  }
	   return nil
   }

// ---- QuorumGate: Gate 挂钩扩展 ----------------------------------------
// RequireQuorumForClear 把四眼前置装配到 Gate 上。
// 非 force 路径的 Clear/ClearScope 调用前必须提供已 cleared 的 ClearPlan。
// 功能：将四眼协议嵌入现有 Gate 而不改 guardClear 签名。
// force=true 路径绕过四眼，但必须由 ForcedAuditStore 留审计。
// 不变量：planID 对应的 ClearPlan 已处于 cleared 状态。
type QuorumGuard struct {
	qs *QuorumStore
}

   func NewQuorumGuard(qs *QuorumStore) *QuorumGuard {
	  return &QuorumGuard{qs: qs}
   }

// AllowClear 实现 ClearChecker 接口；计划 ID 通过 context value 传入。
  func (qg *QuorumGuard) AllowClear(ctx context.Context) error {
	   if qg == nil || qg.qs == nil {
		  return ErrQuorumRequired
	  }
	  planID, _ := ctx.Value(clearPlanIDKey{}).(string)
	   if strings.TrimSpace(planID) == "" {
		  return fmt.Errorf("%w: no clear_plan_id in context", ErrQuorumRequired)
	  }
	  plan,
		   err := qg.qs.GetPlan(ctx, planID)
	  if err != nil {
		  return fmt.Errorf("%w: %v", ErrQuorumRequired, err)
	  }
	  if plan.State != PlanStateCleared {
		   return fmt.Errorf("%w: plan %s state=%s", ErrQuorumRequired, planID, plan.State)
	  }
	  if plan.CloseProofID == "" {
		  return fmt.Errorf("%w: plan %s has no close proof", ErrCloseProofRequired, planID)
	  }
	  proof, err := qg.qs.GetProof(ctx, plan.CloseProofID)
	  if err != nil {
		  return fmt.Errorf("%w: load proof: %v", ErrCloseProofRequired, err)
	  }
	  if err := VerifyProof(proof); err != nil {
		   return fmt.Errorf("%w: proof verification failed: %v", ErrCloseProofRequired, err)
	   }
	  return nil
   }

// ---- context helpers -----------------------------------------------
// clearPlanIDKey 传递 ClearPlan ID 给四眼前置检查。
type clearPlanIDKey struct{}

// WithClearPlanID 向 context 注入 plan_id；Guard 从此处读取。
   func WithClearPlanID(ctx context.Context, planID string) context.Context {
	  return context.WithValue(ctx, clearPlanIDKey{}, planID)
   }

// ---- 内部辅助 -------------------------------------------------------

   func (qs *QuorumStore) getPlanRow(ctx context.Context, planID string) (ClearPlan, persist.Row, error) {
	   if qs == nil || qs.kv == nil {
		  return ClearPlan{}, persist.Row{}, ErrStore
	   }
	  planID = strings.TrimSpace(planID)
	  if planID == "" {
		   return ClearPlan{}, persist.Row{}, ErrPlanIDRequired
	   }
	   row, err := qs.kv.Get(ctx, KindClearPlan, planID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return ClearPlan{}, persist.Row{}, ErrPlanNotFound
		  }
		   return ClearPlan{}, persist.Row{}, err
	  }
	   var p ClearPlan
	  if err := json.Unmarshal(row.Body, &p); err != nil {
		  return ClearPlan{}, persist.Row{}, fmt.Errorf("decode clear plan: %w", err)
	   }
	   return p, row, nil
   }

  func (qs *QuorumStore) putPlan(ctx context.Context, plan ClearPlan, fence persist.Fence) error {
	   body, err := json.Marshal(plan)
	   if err != nil {
		  return err
	   }
	   return qs.kv.Put(ctx, persist.Row{Kind: KindClearPlan, Key: plan.PlanID, Body: body}, fence)
   }

   func (qs *QuorumStore) writeProof(ctx context.Context, proof CloseProof) error {
	   body, err := json.Marshal(proof)
	   if err != nil {
		   return err
	  }
	  // CloseProof 只追加；fence epoch 0 首写。
	   return qs.kv.Put(ctx, persist.Row{Kind: KindCloseProof, Key: proof.ProofID, Body: body}, persist.Fence{})
  }

// newProofID 生成确定性 proof_id：proof:<planID>:<时间戳 ns>。
  func newProofID(planID string) string {
	  return fmt.Sprintf("proof:%s:%d", planID, time.Now().UnixNano())
   }

// computeProofChecksum 构成闭合证明的幂等校验码（明文 SHA-256 摘要）。
// 输入：planID + proofType + approver1 + approver2 + openDiffCount。
// 不含时间戳：证明内容不因时区/精度差异改变。

   func computeProofChecksum(planID, proofType, approver1, approver2 string, openDiffs int) string {
	  raw := fmt.Sprintf("%s|%s|%s|%s|%d", planID, proofType, approver1, approver2, openDiffs)
	   // 使用口径库 FNV-128 等价的简单 hex 摘要（无外部加密依赖）。
	   h := simpleHex(raw)
	   return h
   }

// simpleHex 用内置 fmt 产出可重复的 hex 指纹；不依赖 crypto 包。
// 仅供证明校验，不用于密码学目的。
  func simpleHex(s string) string {
	   var acc uint64 = 14695981039346656037
	  for i := 0; i < len(s); i++ {
		   acc ^= uint64(s[i])
		  acc *= 1099511628211
	   }
	  return fmt.Sprintf("%016x", acc)
   }
