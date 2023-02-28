package keyrotgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// SlotInfo 调用方传入的槽当前状态信息（只读快照，不依赖 keycer.Slot 类型）。
type SlotInfo struct {
	SlotID      string
	Fingerprint string // 当前指纹（若已 Retired 则为空或旧指纹）
	IsRetired   bool   // 槽是否已退役
}

// Detector 指纹漂移检测器。
type Detector struct {
	store Store
	clock func() time.Time
}

// NewDetector 构造 Detector。
   func NewDetector(store Store) *Detector {
	   if store == nil {
		   store = NewMemStore()
	  }
	   return &Detector{
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。
   func (d *Detector) WithClock(fn func() time.Time) *Detector {
	   if fn != nil {
		   d.clock = fn
	   }
	   return d
   }

   // DetectDrift 检测指定 SignBind 是否存在漂移（槽已 Retired 而绑定持有旧指纹）。
   // srcSlot：当前源槽快照（由 Gate 读取后传入）。
   // newFP：目标槽当前指纹（若已加载；否则空字符串）。
   // bind：要检测的 SignBind。
  // leaseID：关联的轮换租约 ID（可为空）。
  // 返回：产出的 KeyRotDriftCase（已持久化）；若无漂移则返回 nil, nil。
  func (d *Detector) DetectDrift(ctx context.Context, srcSlot SlotInfo, newFP string, bind *SignBind, leaseID string) (*KeyRotDriftCase, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	   if bind == nil {
		   return nil, nil
	   }
	  // 漂移条件：源槽已 Retired，且绑定持有的指纹与源槽旧指纹匹配
	   if !srcSlot.IsRetired {
		   return nil, nil
	   }
	   if !FingerprintsMatch(bind.Fingerprint, srcSlot.Fingerprint) && srcSlot.Fingerprint != "" {
		   // 指纹不匹配（不是针对该槽的旧绑定）；不产出漂移案件
		   return nil, nil
	  }
	   // 幂等：同 sign_bind_id 只产出一个漂移案件
	   existing, err := d.findExistingDriftByBind(ctx, bind.SignBindID)
	   if err != nil {
		  return nil, err
	   }
	  if existing != nil {
		   return existing, nil
	  }
	   now := d.clock()
	  dc := &KeyRotDriftCase{
		   DriftCaseID: uuid.NewString(),
		   SrcSlotID:      srcSlot.SlotID,
		  OldFingerprint: bind.Fingerprint,
		  NewFingerprint: strings.TrimSpace(newFP),
		   SignBindID:      bind.SignBindID,
		   RotateLeaseID:   strings.TrimSpace(leaseID),
		   State:           DriftOpen,
		   DriftDetectedAt: now,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		   return nil, err
	   }
	  MetricDriftDetected.Inc()
	  return dc, nil
   }

// ProveCase 提交漂移案件证明（双人确认）。
   func (d *Detector) ProveCase(ctx context.Context, req ProveRotateReq) (*KeyRotDriftCase, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.DriftCaseID) == "" {
		   return nil, ErrDriftCaseNotFound
	   }
	   if strings.TrimSpace(req.ProofChecksum) == "" {
		  return nil, ErrProofChecksumEmpty
	   }
	  if strings.TrimSpace(req.ConfirmedBy) == "" || strings.TrimSpace(req.CoConfirmedBy) == "" {
		  return nil, ErrActorRequired
	  }
	  if strings.EqualFold(strings.TrimSpace(req.ConfirmedBy), strings.TrimSpace(req.CoConfirmedBy)) {
		   return nil, ErrDualConfirm
	   }
	   dc, err := d.store.GetDriftCase(ctx, req.DriftCaseID)
	   if err != nil {
		  return nil, err
	   }
	   if dc.IsTerminal() {
		  return nil, ErrDriftCaseTerminal
	   }
	  if !DriftCaseCanProve(dc.State) {
		   return nil, ErrInvalidTransition
	  }
	  now := d.clock()
	  dc.State = DriftProven
	   dc.ProvenAt = &now
	  dc.ProofChecksum = strings.TrimSpace(req.ProofChecksum)
	   dc.ConfirmedBy = strings.TrimSpace(req.ConfirmedBy)
	  dc.CoConfirmedBy = strings.TrimSpace(req.CoConfirmedBy)
	   dc.Note = strings.TrimSpace(req.Note)
	  dc.UpdatedAt = now
	   if err := d.store.UpdateDriftCaseState(ctx, dc.DriftCaseID, DriftOpen, DriftProven, dc); err != nil {
		  return nil, err
	   }
	   MetricDriftProven.Inc()
	  return dc, nil
   }

// CloseCase 关闭漂移案件（证明通过后）。
  func (d *Detector) CloseCase(ctx context.Context, req CloseDriftCaseReq) (*KeyRotDriftCase, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.DriftCaseID) == "" {
		  return nil, ErrDriftCaseNotFound
	   }
	   dc, err := d.store.GetDriftCase(ctx, req.DriftCaseID)
	  if err != nil {
		   return nil, err
	  }
	  if dc.IsTerminal() {
		  return dc, nil // 幂等
	   }
	   if !DriftCaseCanClose(dc.State) {
		  return nil, ErrInvalidTransition
	   }
	   // 关闭前必须已有证明
	   if dc.State != DriftProven {
		  return nil, ErrProofChecksumEmpty
	  }
	  now := d.clock()
	   dc.State = DriftClosed
	   dc.ClosedAt = &now
	  dc.UpdatedAt = now
	   if err := d.store.UpdateDriftCaseState(ctx, dc.DriftCaseID, DriftProven, DriftClosed, dc); err != nil {
		  return nil, err
	  }
	  MetricDriftClosed.Inc()
	   return dc, nil
   }

// HasOpenDrift 检查指定源槽是否有未关闭的漂移案件（挡后续 Unlock/Rotate）。
  func (d *Detector) HasOpenDrift(ctx context.Context, srcSlotID string) (bool, error) {
	  if d == nil || d.store == nil {
		  return false, ErrUnavailable
	   }
	   cases, err := d.store.ListOpenDriftCases(ctx, srcSlotID)
	  if err != nil {
		  return false, err
	   }
	   return len(cases) > 0, nil
   }

// ListOpenCases 列出指定源槽的未关闭漂移案件。
  func (d *Detector) ListOpenCases(ctx context.Context, srcSlotID string) ([]*KeyRotDriftCase, error) {
	  if d == nil || d.store == nil {
		  return nil, ErrUnavailable
	   }
	   return d.store.ListOpenDriftCases(ctx, srcSlotID)
  }

// findExistingDriftByBind 按 sign_bind_id 查找已存在的漂移案件（幂等保护）。
   func (d *Detector) findExistingDriftByBind(ctx context.Context, signBindID string) (*KeyRotDriftCase, error) {
	   // MemStore/SQLStore 均需遍历；此处通过 ListOpenDriftCases 配合 slot 查找
	   // 实际 SQL 实现中可直接 WHERE sign_bind_id = ? 以提升效率
	   // 此处使用辅助函数，避免在 Store 接口新增 GetByCaseBind
	   allOpen, err := d.store.ListOpenDriftCases(ctx, "")
	  if err != nil {
		   return nil, err
	  }
	  for _, dc := range allOpen {
		   if dc.SignBindID == signBindID {
			  return dc, nil
		   }
	  }
	   return nil, nil
   }

// ScanActiveDrift 巡检所有活跃 SignBind，对源槽已 Retired 的产出漂移案件。
// 由 Worker 定期调用；返回新产出的漂移案件数。
   func (d *Detector) ScanActiveDrift(ctx context.Context, srcSlotID string, slotRetired bool, currentFP, newFP, leaseID string) (int, error) {
	   if d == nil || d.store == nil {
		  return 0, ErrUnavailable
	   }
	   if !slotRetired {
		   return 0, nil
	  }
	  binds, err := d.store.ListActiveSignBinds(ctx, srcSlotID)
	  if err != nil {
		   return 0, err
	   }
	   slotInfo := SlotInfo{
		   SlotID:      srcSlotID,
		   Fingerprint: currentFP,
		  IsRetired: slotRetired,
	  }
	   count := 0
	   for _, bind := range binds {
		  _, err := d.DetectDrift(ctx, slotInfo, newFP, bind, leaseID)
		  if err != nil {
			  continue
		   }
		  count++
	   }
	  return count, nil
  }
