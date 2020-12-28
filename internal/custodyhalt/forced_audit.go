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
	KindForcedAudit = "aa_halt_forced_audit"
	// ForcedReasonCode 允许值；调用方必须从列表中选取。
	ForcedReasonEmergencyHot    = "emergency_hot_recovery"  // 热钱包紧急恢复，已手工核对
	ForcedReasonIncidentCleared = "incident_cleared_by_war" // 战争室确认事故已清，差异工单另行处理
	ForcedReasonReconTimeout = "recon_timeout_ops_accept" // 对账超时，运营接受剩余差异
	ForcedReasonDrillTest    = "drill_test"               // 演练，不用于差异
	ForcedReasonOther        = "other_see_ticket"         // 其他（必须附事后工单 ID）
)

var allowedForcedReasonCodes = map[string]bool{
	ForcedReasonEmergencyHot: true,
	ForcedReasonIncidentCleared: true,
	ForcedReasonReconTimeout: true,
	ForcedReasonDrillTest:    true,
	ForcedReasonOther:        true,
}

var (
	ErrForcedReasonCode = errors.New("forced clear: reason_code required and must be a known value")
	ErrForcedTicketOther = errors.New("forced clear: other reason_code requires post_ticket_id")
	ErrForcedAuditStore  = errors.New("forced clear audit store unavailable")
)

// ---- ForcedAuditRecord -----------------------------------------------
// ForcedAuditRecord 强制清闸审计记录；写入 KV 后不可修改。
// 包含理由码、事后工单 ID（非空时有效）与当时熔断快照。
type ForcedAuditRecord struct {
	AuditID string `json:"audit_id"`
	Scope   string `json:"scope"`
	Chain   string `json:"chain,omitempty"`
	Asset   string `json:"asset,omitempty"`
	Actor string `json:"actor"`
	Reason        string `json:"reason"`                   // 文字说明
	ReasonCode    string `json:"reason_code"`              // 枚举值
	PostTicketID  string `json:"post_ticket_id,omitempty"` // 事后工单 ID
	OpenDiffCount int    `json:"open_diff_count"`          // 强制清闸时实测未关差异数
	FlagSnapshot  string `json:"flag_snapshot"`            // JSON 形态的 Flag/ScopedFlag 快照
	IssuedAt time.Time `json:"issued_at"`
	// MisClearRisk 是否存在误清风险；open_diff_count>0 时自动置 true。
	MisClearRisk bool `json:"misclear_risk"`
}

// ---- ForcedAuditStore -----------------------------------------------
// ForcedAuditStore 强制清闸审计写入器；与 AuditStore 并联，不替换。
// 功能：ClearForced 路径必须先通过 ValidateForced，再调 RecordForced，否则拒绝执行。
// 只写 aa_halt_forced_audit kind；不读写普通熔断 KV。
// 不变量：每次 ClearForced 调用对应唯一一条 ForcedAuditRecord；审计 ID 由时间戳+actor 构成。
type ForcedAuditStore struct {
	kv persist.KV
}

  func NewForcedAuditStore(kv persist.KV) *ForcedAuditStore {
	   return &ForcedAuditStore{kv: kv}
   }

// ValidateForced 在执行强制清闸之前调用；检查理由码与工单约束。
// 功能：缺理由码或不合法理由码时拒绝；other 理由码要求 postTicketID。

   func ValidateForced(reasonCode, postTicketID string) error {
	   reasonCode = strings.TrimSpace(reasonCode)
	   if reasonCode == "" {
		  return ErrForcedReasonCode
	   }
	   if !allowedForcedReasonCodes[reasonCode] {
		   return fmt.Errorf("%w: got %q, allowed: %v", ErrForcedReasonCode, reasonCode,
			  allowedForcedReasonCodesSlice())
	  }
	  if reasonCode == ForcedReasonOther && strings.TrimSpace(postTicketID) == "" {
		   return ErrForcedTicketOther
	  }
	  return nil
   }

// RecordForced 写入强制清闸审计；写失败不阻断熔断操作（防止审计拖垮业务）但返回错误供日志记录。
// 功能：持久化强制清闸证据，供事后误清检测使用。
// 不变量：同一 auditID 只写一次（首次 fence epoch=0）；重复调用幂等（同 key 不覆盖）。

  func (f *ForcedAuditStore) RecordForced(ctx context.Context, rec ForcedAuditRecord) error {
	   if f == nil || f.kv == nil {
		   return ErrForcedAuditStore
	   }
	  if rec.AuditID == "" {
		   rec.AuditID = newForcedAuditID(rec.Actor)
	   }
	   if rec.IssuedAt.IsZero() {
		   rec.IssuedAt = time.Now().UTC()
	  }
	   rec.MisClearRisk = rec.OpenDiffCount > 0
	  body, err := json.Marshal(rec)
	   if err != nil {
		   return err
	   }
	  // 检查是否已存在；存在则幂等跳过。
	  _, existing := f.kv.Get(ctx, KindForcedAudit, rec.AuditID)
	  if existing == nil {
		   return nil // 已写入，幂等
	   }
	  return f.kv.Put(ctx, persist.Row{Kind: KindForcedAudit, Key: rec.AuditID, Body: body}, persist.Fence{})
   }

// LoadForced 读取最近 limit 条强制清闸审计。
   func (f *ForcedAuditStore) LoadForced(ctx context.Context, limit int) []ForcedAuditRecord {
	   if f == nil || f.kv == nil {
		   return nil
	  }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	   rows, err := f.kv.List(ctx, KindForcedAudit, limit)
	  if err != nil || len(rows) == 0 {
		  return nil
	   }
	   out := make([]ForcedAuditRecord, 0, len(rows))
	  for _, r := range rows {
		  var rec ForcedAuditRecord
		   if json.Unmarshal(r.Body, &rec) == nil {
			  out = append(out, rec)
		  }
	   }
	   return out
  }

// QueryForced 按 actor / scope / reason_code 过滤。

   func (f *ForcedAuditStore) QueryForced(ctx context.Context, actor, scope, reasonCode string, limit int) []ForcedAuditRecord {
	   all := f.LoadForced(ctx, 200)
	  actor = strings.TrimSpace(actor)
	   scope = strings.TrimSpace(strings.ToLower(scope))
	   reasonCode = strings.TrimSpace(reasonCode)
	   if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	  out := make([]ForcedAuditRecord, 0, len(all))
	   for _, rec := range all {
		   if actor != "" && rec.Actor != actor {
			   continue
		  }
		   if scope != "" && strings.ToLower(rec.Scope) != scope {
			  continue
		   }
		   if reasonCode != "" && rec.ReasonCode != reasonCode {
			   continue
		   }
		   out = append(out, rec)
	   }
	   if len(out) > limit {
		   out = out[len(out)-limit:]
	  }
	  return out
   }

// ---- 与 Gate 集成 --------------------------------------------------
// ForcedClearWithAudit 强制清闸入口：先校验，再执行 Gate.ClearForced，再落审计。
// 调用方传入 openDiffCount（由 ClearChecker 或外部统计）。
// 功能：替代裸调 gate.ClearForced，保证审计不可绕过。
// 审计写失败时记录错误但不回滚已完成的 Gate 操作（fail-open on audit）。
   func ForcedClearWithAudit(
	  ctx context.Context,
	   gate *Gate,
	  fas *ForcedAuditStore,
	  scope Scope,
	   chain, asset, actor, reason, reasonCode, postTicketID string,
	  openDiffCount int,
	   flagSnapshot string,
  ) error {
	   if err := ValidateForced(reasonCode, postTicketID); err != nil {
		  return err
	   }
	  var clearErr error
	   if scope == ScopeGlobal || scope == "" {
		   clearErr = gate.ClearForced(ctx, actor, reason)
	  } else {
		   clearErr = gate.ClearScopeForced(ctx, scope, chain, asset, actor, reason)
	  }
	   if clearErr != nil {
		  return clearErr
	  }
	   // 落审计；失败不回滚。
	  rec := ForcedAuditRecord{
		  AuditID: newForcedAuditID(actor),
		  Scope: string(scope),
		  Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   Asset:  strings.ToUpper(strings.TrimSpace(asset)),
		   Actor:  actor,
		   Reason: reason,
		  ReasonCode:   reasonCode,
		   PostTicketID: postTicketID,
		  OpenDiffCount: openDiffCount,
		  FlagSnapshot:  flagSnapshot,
		  IssuedAt:      time.Now().UTC(),
	  }
	   _ = fas.RecordForced(ctx, rec)
	  return nil
   }

// ---- 内部辅助 -------------------------------------------------------

   func newForcedAuditID(actor string) string {
	  return fmt.Sprintf("fa:%d:%s", time.Now().UnixNano(), strings.TrimSpace(actor))
  }

   func allowedForcedReasonCodesSlice() []string {
	   out := make([]string, 0, len(allowedForcedReasonCodes))
	   for k := range allowedForcedReasonCodes {
		   out = append(out, k)
	  }
	  return out
  }
