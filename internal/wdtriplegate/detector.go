package wdtriplegate

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Detector 漂移检测器。
type Detector struct {
	store Store
	clock func() time.Time
}

// NewDetector 构造漂移检测器。

  func NewDetector(store Store) *Detector {
	  if store == nil {
		   store = NewMemStore()
	   }
	  return &Detector{store: store, clock: func() time.Time { return time.Now().UTC() }}
   }

// WithClock 注入时钟（用）。

  func (d *Detector) WithClock(fn func() time.Time) *Detector {
	   if fn != nil {
		   d.clock = fn
	   }
	  return d
   }

   // DetectReq 漂移发现请求（由 Binder 在复验失败时填充）。
   type DetectReq struct {
	   MovementID    string
	  TripleLeaseID string
	   DriftKind DriftKind
	  LeaseListGen int64
	   CurrentListGen   int64
	  LeasePolicyVer   int
	   CurrentPolicyVer int
	   LeaseKYTPin string
	  CurrentKYTPin string
  }

   // Prove 产出 TripleDriftCase 并写入存储（append-only）。
  // 若同 movement_id + drift_kind 已存在案件则直接返回现有案件（幂等）。

   func (d *Detector) Prove(req DetectReq) (*TripleDriftCase, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	  movementID := strings.TrimSpace(req.MovementID)
	  if movementID == "" {
		   return nil, ErrMovementRequired
	   }
	  // 幂等：检查已存在的漂移案件
	  existing, _ := d.store.ListDriftCasesByMovement(movementID)
	  for _, dc := range existing {
		   if dc.DriftKind == req.DriftKind && dc.TripleLeaseID == req.TripleLeaseID {
			  cp := CopyDriftCase(*dc)
			  return &cp, nil
		   }
	   }
	  now := d.clock()
	  driftCaseID := uuid.NewString()
	  // proof_checksum = SHA-256(movement_id|drift_kind|lease_gen|current_gen|detected_at)
	  raw := fmt.Sprintf("%s|%s|%d|%d|%s",
		   movementID, string(req.DriftKind),
		  req.LeaseListGen,
		   req.CurrentListGen,
		  now.Format(time.RFC3339Nano))
	   checksum := hexSHA256(raw)
	   dc := TripleDriftCase{
		  DriftCaseID: driftCaseID,
		  MovementID: movementID,
		   TripleLeaseID: strings.TrimSpace(req.TripleLeaseID),
		  DriftKind:     req.DriftKind,
		  LeaseListGen:   req.LeaseListGen,
		  CurrentListGen: req.CurrentListGen,
		  LeasePolicyVer:   req.LeasePolicyVer,
		   CurrentPolicyVer: req.CurrentPolicyVer,
		   LeaseKYTPin:      req.LeaseKYTPin,
		   CurrentKYTPin:    req.CurrentKYTPin,
		  State:         DriftOpen,
		   ProofChecksum: checksum,
		   DetectedAt:    now,
		  CreatedAt:     now,
		   UpdatedAt:     now,
	  }
	  if err := d.store.AppendDriftCase(dc); err != nil {
		  return nil, err
	  }
	  MetricDriftCaseOpen.Inc()
	   cp := CopyDriftCase(dc)
	   return &cp, nil
  }

// AttachProof 将证明摘要附上（Open→Proven）。
// proofChecksum 由外部审计工具生成后通过 ops API 提交。
  func (d *Detector) AttachProof(driftCaseID, proofChecksum string) (*TripleDriftCase, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	   driftCaseID = strings.TrimSpace(driftCaseID)
	  if driftCaseID == "" {
		   return nil, ErrDriftCaseNotFound
	  }
	  dc, err := d.store.GetDriftCase(driftCaseID)
	  if err != nil {
		  return nil, err
	  }
	  if DriftCaseIsTerminal(dc.State) {
		   return nil, ErrDriftCaseTerminal
	  }
	   if dc.State == DriftProven {
		  cp := CopyDriftCase(*dc)
		   return &cp, nil // 幂等
	   }
	   now := d.clock()
	  extra := &DriftCaseExtra{ProofChecksum: strings.TrimSpace(proofChecksum)}
	   return d.store.UpdateDriftCaseState(driftCaseID, DriftOpen, DriftProven, now, extra)
   }

// ConfirmClose 双人确认关闭漂移案件（Proven→Closed 或 Open→Closed）。
// confirmed_by != co_confirmed_by 是双人鉴权不变量。

  func (d *Detector) ConfirmClose(req ConfirmDriftCaseReq) (*TripleDriftCase, error) {
	   if d == nil || d.store == nil {
		  return nil, ErrUnavailable
	  }
	  driftCaseID := strings.TrimSpace(req.DriftCaseID)
	   confirmedBy := strings.TrimSpace(req.ConfirmedBy)
	   coConfirmedBy := strings.TrimSpace(req.CoConfirmedBy)
	   if driftCaseID == "" {
		  return nil, ErrDriftCaseNotFound
	   }
	   if confirmedBy == "" || coConfirmedBy == "" || confirmedBy == coConfirmedBy {
		  return nil, ErrDualConfirm
	   }
	   dc,
		  err := d.store.GetDriftCase(driftCaseID)
	  if err != nil {
		   return nil, err
	  }
	  if DriftCaseIsTerminal(dc.State) {
		   return nil, ErrDriftCaseTerminal
	  }
	  now := d.clock()
	   extra := &DriftCaseExtra{
		   ConfirmedBy:   confirmedBy,
		  CoConfirmedBy: coConfirmedBy,
		   CloseNote:     strings.TrimSpace(req.Note),
	  }
	  result,
		   err := d.store.UpdateDriftCaseState(driftCaseID, dc.State, DriftClosed, now, extra)
	   if err != nil {
		   return nil, err
	  }
	   MetricDriftCaseClosed.Inc()
	  return result, nil
  }

// ListOpenCases 列出所有 Open 状态的漂移案件。

   func (d *Detector) ListOpenCases(limit int) ([]*TripleDriftCase, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	  return d.store.ListOpenDriftCases(limit)
  }

// ListByMovement 查询 movement_id 的全部漂移案件。
   func (d *Detector) ListByMovement(movementID string) ([]*TripleDriftCase, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	   return d.store.ListDriftCasesByMovement(strings.TrimSpace(movementID))
  }

// ScanAndReport 巡检当前所有 Open 漂移案件（Worker 调用；只读汇报）。
   func (d *Detector) ScanAndReport() ([]*TripleDriftCase, error) {
	   return d.ListOpenCases(500)
   }
