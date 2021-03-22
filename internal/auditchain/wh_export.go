package auditchain

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/custody"
	"github.com/google/uuid"
)

// Desk 导出台。审批单种类用 ceremony，不另起权限中心。

type Desk struct {
	WH   *Warehouse
	Appr *approvestore.Store
}

   func NewDesk(wh *Warehouse, appr *approvestore.Store) *Desk {
	   if appr == nil {
		  appr = approvestore.NewMem()
	   }
	   return &Desk{WH: wh, Appr: appr}
  }

// ExportView 给第二个人看围栏。没有围栏不能批。
type ExportView struct {
	ExportRec
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	Approval string `json:"approval_state"`
}

// Bundle 信任导出。每行带默克尔路径，根对不上就整包拒绝。
type Bundle struct {
	Export ExportRec `json:"export"`
	Root   string    `json:"merkle_root"`
	Rows   []Row     `json:"rows"`
	Proofs []Proof `json:"proofs"`
}

// Request 申请人开单。段必须已封存且没被拦住。

  func (d *Desk) Request(ctx context.Context, actor string, segID, user, asset string, from, to time.Time) (ExportView, error) {
	  if d == nil || d.WH == nil || d.Appr == nil {
		  return ExportView{}, ErrUnavailable
	   }
	   actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		   return ExportView{}, ErrActor
	   }
	  if err := d.WH.ExportAllowed(ctx, segID); err != nil {
		   return ExportView{}, err
	   }
	  now := d.WH.now()
	   rec := ExportRec{
		  ID: uuid.NewString(), SegmentID: segID, UserID: strings.TrimSpace(user), Asset: strings.ToUpper(strings.TrimSpace(asset)),
		   From: from.UTC(), To: to.UTC(), RequestedBy: actor, State: ExportPending, CreatedAt: now,
	   }
	  a, err := d.Appr.Open(ctx, custody.KindCeremony, actor, rec.ID, "", "", "", "audit-export")
	   if err != nil {
		  return ExportView{}, err
	   }
	   rec.ApprovalID = a.ID
	  if err := d.WH.st.PutExport(ctx, rec); err != nil {
		  return ExportView{}, err
	  }
	  return d.view(ctx, rec)
  }

  func (d *Desk) view(ctx context.Context, rec ExportRec) (ExportView, error) {
	   out := ExportView{ExportRec: rec}
	  if rec.ApprovalID == "" {
		   return out, nil
	   }
	   a, fence, err := d.Appr.Get(ctx, rec.ApprovalID)
	  if err != nil {
		  return ExportView{}, err
	  }
	  out.FenceEpoch = fence.Epoch
	  out.FenceToken = fence.Token
	  out.Approval = string(a.State)
	   return out, nil
   }

// GetExport 读导出单和当前围栏。
  func (d *Desk) GetExport(ctx context.Context, id string) (ExportView, error) {
	  if d == nil || d.WH == nil {
		   return ExportView{}, ErrUnavailable
	  }
	  rec, ok, err := d.WH.st.Export(ctx, id)
	  if err != nil {
		  return ExportView{}, err
	  }
	  if !ok {
		  return ExportView{}, ErrExport
	   }
	  return d.view(ctx, rec)
   }

// Approve 第二个人批。申请人再批会在审批仓储里被拒。

   func (d *Desk) Approve(ctx context.Context, id, actor string, allow bool, epoch int64, token string) (ExportView, error) {
	  rec, ok, err := d.WH.st.Export(ctx, id)
	  if err != nil || !ok {
		   if err == nil {
			   err = ErrExport
		  }
		  return ExportView{}, err
	   }
	   if _, err := d.Appr.ApproveFenced(ctx, rec.ApprovalID, actor, allow, epoch, token); err != nil {
		  return ExportView{}, err
	  }
	  if !allow {
		   rec.State = ExportDenied
		  if err := d.WH.st.PutExport(ctx, rec); err != nil {
			  return ExportView{}, err
		  }
	  }
	   return d.view(ctx, rec)
  }

// Release 放出轨迹。批准人必须是另一个人，且每行证明验得过。
   func (d *Desk) Release(ctx context.Context, id, actor string) (Bundle, error) {
	  if d == nil || d.WH == nil {
		  return Bundle{}, ErrUnavailable
	  }
	   actor = strings.TrimSpace(actor)
	   rec, ok, err := d.WH.st.Export(ctx, id)
	   if err != nil || !ok {
		  if err == nil {
			   err = ErrExport
		  }
		  return Bundle{}, err
	  }
	   if err := d.WH.ExportAllowed(ctx, rec.SegmentID); err != nil {
		   rec.State = ExportDenied
		   _ = d.WH.st.PutExport(ctx, rec)
		   return Bundle{}, err
	   }
	  a, _, err := d.Appr.Get(ctx, rec.ApprovalID)
	   if err != nil {
		   return Bundle{}, err
	  }
	  if a.State != custody.ApprovalApproved && a.State != custody.ApprovalExecuted {
		   return Bundle{}, ErrExport
	   }
	   if a.ApprovedBy == "" || strings.EqualFold(a.ApprovedBy, a.RequestedBy) {
		   return Bundle{}, ErrExport
	   }
	   if strings.EqualFold(actor, a.RequestedBy) {
		  return Bundle{}, ErrExport
	  }
	   seg, ok, err := d.WH.st.Segment(ctx, rec.SegmentID)
	   if err != nil || !ok {
		  if err == nil {
			  err = ErrSegment
		  }
		   return Bundle{}, err
	  }
	  rows, err := d.WH.st.Rows(ctx, rec.SegmentID)
	  if err != nil {
		   return Bundle{}, err
	  }
	   leaves := LeavesOf(rows)
	  proofs := make([]Proof, 0, len(rows))
	  for i := range rows {
		  if rec.UserID != "" && rows[i].UserID != rec.UserID {
			  continue
		  }
		  if rec.Asset != "" && rows[i].Asset != rec.Asset {
			  continue
		   }
		   if !rec.From.IsZero() && rows[i].OccurredAt.Before(rec.From) {
			   continue
		  }
		   if !rec.To.IsZero() && rows[i].OccurredAt.After(rec.To) {
			  continue
		  }
		  p, err := Prove(leaves, i)
		   if err != nil {
			   return Bundle{}, err
		   }
		   if !VerifyProof(seg.MerkleRoot, p) {
			   _ = d.WH.MarkFrozen(ctx,
				  seg.ID, AlertDrift)
			  return Bundle{}, ErrDrift
		   }
		  proofs = append(proofs, p)
	   }
	   filtered := make([]Row, 0, len(proofs))
	   for i := range rows {
		  if rec.UserID != "" && rows[i].UserID != rec.UserID {
			  continue
		  }
		  if rec.Asset != "" && rows[i].Asset != rec.Asset {
			  continue
		   }
		  if !rec.From.IsZero() && rows[i].OccurredAt.Before(rec.From) {
			   continue
		  }
		   if !rec.To.IsZero() && rows[i].OccurredAt.After(rec.To) {
			   continue
		  }
		   filtered = append(filtered,
			   rows[i])
	  }
	  rec.State = ExportReleased
	  rec.ReleasedAt = d.WH.now()
	  if err := d.WH.st.PutExport(ctx, rec); err != nil {
		   return Bundle{}, err
	   }
	  return Bundle{Export: rec, Root: seg.MerkleRoot, Rows: filtered, Proofs: proofs}, nil
  }
