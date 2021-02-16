package auditchain

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// InspectReport 一次巡检的结论。Failed 为真则巡检不通过。
type InspectReport struct {
	Checked int `json:"checked"`
	Sealed  int `json:"sealed"`
	Frozen int `json:"frozen"`
	Alerts []string `json:"alerts,omitempty"`
	Failed  bool     `json:"failed"`
	Lagging []string `json:"lagging,omitempty"`
}

// Inspect 扫全部段。发现篡改就冻结该段并写挡住导出的原因。

   func (w *Warehouse) Inspect(ctx context.Context) (InspectReport, error) {
	   var rep InspectReport
	  if w == nil || w.st == nil {
		  return rep, ErrUnavailable
	  }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	  segs, err := w.st.AllSegments(ctx)
	   if err != nil {
		  return rep, err
	  }
	   rep.Checked = len(segs)
	   byPart := map[string][]Segment{}
	  for _, seg := range segs {
		   byPart[seg.PartKey] = append(byPart[seg.PartKey], seg)
		   if seg.State == SegSealed {
			   rep.Sealed++
		   }
		   if seg.State == SegFrozen {
			   rep.Frozen++
		  }
		   if err := w.checkSegment(ctx, seg, &rep); err != nil {
			   return rep, err
		   }
	   }
	  for part, list := range byPart {
		  if err := w.checkOrdinals(ctx, part, list, &rep); err != nil {
			   return rep, err
		   }
	   }
	  if err := w.checkLag(ctx, &rep); err != nil {
		   return rep, err
	   }
	  if err := w.checkCheckpointChain(ctx, &rep); err != nil {
		  return rep, err
	   }
	  if rep.Failed {
		  whInspect.WithLabelValues("fail").Inc()
	   } else {
		  whInspect.WithLabelValues("ok").Inc()
	  }
	   return rep, nil
   }

   func (w *Warehouse) checkSegment(ctx context.Context, seg Segment, rep *InspectReport) error {
	   rows, err := w.st.Rows(ctx, seg.ID)
	  if err != nil {
		  return err
	  }
	  prev := ""
	   broken := ""
	   for i, r := range rows {
		   if r.Seq != i+1 {
			   broken = fmt.Sprintf("seq %d", r.Seq)
			  break
		   }
		  if r.PrevHash != prev {
			  broken = fmt.Sprintf("prev %d", r.Seq)
			  break
		   }
		  want := HashRow(r)
		   if r.SelfHash != want {
			   broken = fmt.Sprintf("hash %d", r.Seq)
			   break
		  }
		   prev = r.SelfHash
	   }
	  if broken != "" {
		   rep.Failed = true
		  rep.Alerts = append(rep.Alerts, AlertBroken)
		   if err := w.raise(ctx, AlertBroken, seg.ID, broken); err != nil {
			   return err
		   }
		  return w.freezeLocked(ctx, seg.ID, AlertBroken)
	   }
	  if seg.Count != len(rows) {
		  rep.Failed = true
		  rep.Alerts = append(rep.Alerts, AlertBroken)
		  if err := w.raise(ctx, AlertBroken, seg.ID, "count"); err != nil {
			  return err
		  }
		  return w.freezeLocked(ctx,
			  seg.ID, AlertBroken)
	   }
	   if seg.State == SegOpen {
		  return nil
	  }
	   root := MerkleRoot(LeavesOf(rows))
	  if seg.MerkleRoot != root {
		  rep.Failed = true
		  rep.Alerts = append(rep.Alerts, AlertDrift)
		   if err := w.raise(ctx, AlertDrift, seg.ID, "segment root"); err != nil {
			   return err
		  }
		  return w.freezeLocked(ctx, seg.ID, AlertDrift)
	  }
	  cks, err := w.st.Checkpoints(ctx, seg.ID)
	  if err != nil {
		  return err
	  }
	   var final *Checkpoint
	  for i := range cks {
		   c := cks[i]
		  if !c.Tip && c.Seq == seg.Count {
			   final = &c
		  }
	   }
	  if final == nil || final.MerkleRoot != root {
		   rep.Failed = true
		  rep.Alerts = append(rep.Alerts, AlertDrift)
		   if err := w.raise(ctx, AlertDrift, seg.ID, "checkpoint"); err != nil {
			  return err
		   }
		   return w.freezeLocked(ctx,
			  seg.ID, AlertDrift)
	   }
	   return nil
   }

  func (w *Warehouse) checkOrdinals(ctx context.Context, part string, list []Segment, rep *InspectReport) error {
	  for i, seg := range list {
		  if seg.Ordinal != i+1 {
			   rep.Failed = true
			   rep.Alerts = append(rep.Alerts, AlertMissing)
			  return w.raise(ctx, AlertMissing, seg.ID, ErrOrdinal.Error()+": "+part)
		  }
	   }
	   return nil
  }

  func (w *Warehouse) checkLag(ctx context.Context, rep *InspectReport) error {
	   curs, err := w.st.Cursors(ctx)
	  if err != nil {
		  return err
	   }
	  for _, c := range curs {
		   if c.Lag > w.cfg.LagLimit {
			  rep.Lagging = append(rep.Lagging, c.Source)
			   rep.Alerts = append(rep.Alerts, AlertLag)
			  if err := w.raise(ctx, AlertLag, "", c.Source); err != nil {
				   return err
			   }
		   }
	   }
	   return nil
   }

   func (w *Warehouse) checkCheckpointChain(ctx context.Context, rep *InspectReport) error {
	   cks, err := w.st.AllCheckpoints(ctx)
	   if err != nil {
		   return err
	  }
	  segs, err := w.st.AllSegments(ctx)
	  if err != nil {
		   return err
	   }
	  partOf := map[string]string{}
	  for _, seg := range segs {
		  partOf[seg.ID] = seg.PartKey
	   }
	   grouped := map[string][]Checkpoint{}
	   var order []string
	   for _, c := range cks {
		  part := partOf[c.SegmentID]
		   if _, ok := grouped[part]; !ok {
			   order = append(order, part)
		   }
		  grouped[part] = append(grouped[part], c)
	  }
	  for _, part := range order {
		   if err := w.walkCheckpointChain(ctx, grouped[part], rep); err != nil {
			  return err
		  }
	   }
	  return nil
  }

// walkCheckpointChain 按 prev_root 串检查点。同一时刻的尖端和封存不靠 UUID 排序。
   func (w *Warehouse) walkCheckpointChain(ctx context.Context, cks []Checkpoint, rep *InspectReport) error {
	  pending := append([]Checkpoint(nil), cks...)
	  cur := ""
	  for len(pending) > 0 {
		  idx := -1
		  n := 0
		   for i, c := range pending {
			   if c.PrevRoot == cur {
				  n++
				  idx = i
			  }
		  }
		  if n != 1 {
			   id := ""
			  if len(pending) > 0 {
				   id = pending[0].SegmentID
			  }
			   rep.Failed = true
			   rep.Alerts = append(rep.Alerts,
				  AlertDrift)
			   if err := w.raise(ctx, AlertDrift, id, "prev root"); err != nil {
				   return err
			   }
			  if id == "" {
				   return ErrDrift
			  }
			  return w.freezeLocked(ctx,
				  id, AlertDrift)
		   }
		  c := pending[idx]
		   pending = append(pending[:idx], pending[idx+1:]...)
		   cur = c.MerkleRoot
	   }
	   return nil
  }

// MarkFrozen 巡检之外的冻结入口。调用方没持仓库锁。
  func (w *Warehouse) MarkFrozen(ctx context.Context, id, reason string) error {
	   if w == nil {
		   return ErrUnavailable
	   }
	   w.mu.Lock()
	  defer w.mu.Unlock()
	  return w.freezeLocked(ctx, id, reason)
   }

  func (w *Warehouse) raise(ctx context.Context, name, segmentID, detail string) error {
	  a := Alert{ID: uuid.NewString(), Name: name, SegmentID: segmentID, Detail: detail, Open: true, At: w.now()}
	  return w.st.PutAlert(ctx, a)
  }

  func (w *Warehouse) freezeLocked(ctx context.Context, id, reason string) error {
	  seg, ok, err := w.st.Segment(ctx, id)
	   if err != nil || !ok {
		   if err == nil {
			   err = ErrSegment
		   }
		  return err
	   }
	  if seg.State == SegOpen {
		  if _, err := w.sealLocked(ctx, id, w.now(), "integrity"); err != nil {
			   return err
		   }
		   seg, ok, err = w.st.Segment(ctx, id)
		  if err != nil || !ok {
			   if err == nil {
				  err = ErrSegment
			   }
			   return err
		   }
	  }
	  if seg.State == SegFrozen {
		   if seg.HoldReason == "" {
			   seg.HoldReason = reason
			  return w.st.SaveSegment(ctx, seg)
		   }
		   return nil
	  }
	  if err := Transit(seg.State, SegFrozen); err != nil {
		  return err
	   }
	  seg.State = SegFrozen
	  seg.FrozenAt = w.now()
	   seg.HoldReason = reason
	  if err := w.st.SaveSegment(ctx, seg); err != nil {
		   return err
	  }
	  return w.raise(ctx, AlertBlocked, id, reason)
  }

// ExportAllowed 未封存、已冻结、或被拦住的段不能做信任导出。
   func (w *Warehouse) ExportAllowed(ctx context.Context, segmentID string) error {
	  if w == nil {
		   return ErrUnavailable
	  }
	   if ExportHalted() {
		   return fmt.Errorf("%w: ingest halt", ErrExport)
	   }
	   seg, ok, err := w.st.Segment(ctx, segmentID)
	  if err != nil {
		   return err
	  }
	  if !ok {
		   return ErrSegment
	  }
	  if seg.State != SegSealed {
		  return fmt.Errorf("%w: %s", ErrExport, seg.State)
	  }
	   if seg.HoldReason != "" {
		  return fmt.Errorf("%w: %s", ErrExport, seg.HoldReason)
	  }
	   return nil
  }

// Backdate 演练用：把游标更新时间拨到过去，让滞后告警出现。

   func (w *Warehouse) BackdateCursor(ctx context.Context, source string, at time.Time) error {
	  if w == nil {
		  return ErrUnavailable
	   }
	  w.mu.Lock()
	   defer w.mu.Unlock()
	   cur, ok, err := w.st.Cursor(ctx, source)
	   if err != nil || !ok {
		  if err == nil {
			  err = ErrCursor
		   }
		   return err
	  }
	  cur.UpdatedAt = at.UTC()
	  cur.Lag = w.cfg.LagLimit + time.Minute
	   return w.st.PutCursor(ctx, cur)
   }
