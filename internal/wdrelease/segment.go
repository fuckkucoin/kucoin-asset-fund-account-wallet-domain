package wdrelease

import (
	"time"
	"github.com/shopspring/decimal"
)

   func splitBps(total decimal.Decimal, bps []int) ([]decimal.Decimal, error) {
	   if !total.IsPositive() {
		   return nil, WithCode(ErrAmount, CodeAmount)
	  }
	  if len(bps) == 0 {
		  return nil,
			  WithCode(ErrPolicy, CodePolicy)
	   }
	  sum := 0
	  for _, b := range bps {
		   if b <= 0 {
			   return nil,
				   WithCode(ErrPolicy, CodePolicy)
		   }
		  sum += b
	  }
	  if sum != 10000 {
		  return nil, WithCode(ErrPolicy, CodePolicy)
	   }
	   out := make([]decimal.Decimal, len(bps))
	   acc := decimal.Zero
	  unit := decimal.NewFromInt(10000)
	  for i, b := range bps {
		   if i == len(bps)-1 {
			  out[i] = total.Sub(acc)
			   continue
		   }
		  part := total.Mul(decimal.NewFromInt(int64(b))).Div(unit).RoundDown(8)
		  if !part.IsPositive() {
			   return nil, WithCode(ErrAmount, CodeAmount)
		   }
		  out[i] = part
		   acc = acc.Add(part)
	   }
	  if !out[len(out)-1].IsPositive() {
		  return nil, WithCode(ErrAmount, CodeAmount)
	   }
	  got := decimal.Zero
	   for _, p := range out {
		   got = got.Add(p)
	  }
	   if !got.Equal(total) {
		  return nil, WithCode(ErrAmount, CodeAmount)
	   }
	  return out, nil
   }

  func buildSegments(dec Decision, now time.Time) ([]Segment, error) {
	   if dec.Deny {
		   return nil, WithCode(ErrDenied, dec.Code)
	  }
	   if len(dec.Amounts) != len(dec.Ratios) || len(dec.Amounts) == 0 {
		   return nil, WithCode(ErrPolicy, CodePolicy)
	   }
	  out := make([]Segment, len(dec.Amounts))
	   for i := range dec.Amounts {
		  need := i + 1
		   if dec.Skip {
			  need = 0
		   }
		   st := SegPlanned
		  by := ""
		  at := time.Time{}
		  if dec.Skip {
			   st = SegAuthorized
			   by = "policy-skip"
			   at = now
		   }
		   out[i] = Segment{
			   Seq:    i + 1,
			   Bps:    dec.Ratios[i],
			  Amount: dec.Amounts[i],
			  Cap:    dec.SegmentCap,
			   Need:  need,
			  State: st,
			  By:    by,
			  At: at,
		   }
	  }
	   return out, nil
   }

// authorizeSegments 按已通过级数放开段。未到级数的段保持 planned。
  func authorizeSegments(t *Ticket, actor string, now time.Time) {
	   if t == nil {
		   return
	  }
	   n := len(t.Segments)
	   for i := range t.Segments {
		  seg := &t.Segments[i]
		   if seg.State != SegPlanned {
			  continue
		  }
		   need := seg.Need
		   if i == n-1 && t.Depth > need {
			   need = t.Depth
		  }
		  if t.Approvals < need {
			   continue
		   }
		  if !AllowSegment(seg.State, SegAuthorized) {
			  continue
		  }
		  seg.State = SegAuthorized
		  seg.By = actor
		   seg.At = now
	  }
   }

// cancelUnused 拒绝或超时时取消尚未冻结的段。已冻结段保持不动。
   func cancelUnused(t *Ticket) {
	  if t == nil {
		  return
	   }
	   for i := range t.Segments {
		   seg := &t.Segments[i]
		  switch seg.State {
		  case SegPlanned, SegAuthorized:
			   seg.State = SegCancelled
		  case SegReserved:
			  // 占用中的段不属于未用额度，留给 Abort 或 Commit，拒绝路径不得改成 frozen。
		   }
	   }
  }

// takeAuthorized 按顺序整段占用。金额对不上任一前缀则整笔失败，不拆段。
  func takeAuthorized(t *Ticket, amount decimal.Decimal) ([]int, error) {
	   if t == nil {
		   return nil, WithCode(ErrNoTicket, CodeNoTicket)
	  }
	  if !amount.IsPositive() {
		  return nil, WithCode(ErrAmount, CodeAmount)
	  }
	  got := decimal.Zero
	  var seqs []int
	  for i := range t.Segments {
		  seg := &t.Segments[i]
		   if seg.State == SegFrozen || seg.State == SegReserved {
			  continue
		  }
		   if seg.State != SegAuthorized {
			   break
		   }
		  part,
			  err := decimal.NewFromString(seg.Amount)
		  if err != nil {
			  return nil, WithCode(ErrAmount, CodeAmount)
		   }
		  next := got.Add(part)
		  if next.GreaterThan(amount) {
			  return nil, WithCode(ErrQuota, CodeQuota)
		   }
		   seqs = append(seqs, seg.Seq)
		   got = next
		  if got.Equal(amount) {
			   return seqs, nil
		  }
	   }
	  return nil,
		   WithCode(ErrQuota, CodeQuota)
   }

   func markSeq(t *Ticket, seqs []int, to, actor string, now time.Time) error {
	  want := map[int]bool{}
	   for _, s := range seqs {
		   want[s] = true
	   }
	  for i := range t.Segments {
		  seg := &t.Segments[i]
		  if !want[seg.Seq] {
			  continue
		   }
		   if !AllowSegment(seg.State, to) {
			  return WithCode(ErrSegment, CodeQuota)
		  }
		   seg.State = to
		   if actor != "" {
			   seg.By = actor
		   }
		  seg.At = now
		  delete(want, seg.Seq)
	   }
	  if len(want) != 0 {
		   return WithCode(ErrSegment, CodeQuota)
	   }
	   return nil
  }

  func recompute(t *Ticket) {
	   if t == nil {
		  return
	   }
	  auth := decimal.Zero
	   frz := decimal.Zero
	  res := decimal.Zero
	  for _, seg := range t.Segments {
		   part, err := decimal.NewFromString(seg.Amount)
		   if err != nil {
			   continue
		  }
		  switch seg.State {
		  case SegAuthorized:
			  auth = auth.Add(part)
			   res = res.Add(part)
		  case SegReserved:
			  auth = auth.Add(part)
		  case SegFrozen:
			  auth = auth.Add(part)
			   frz = frz.Add(part)
		   }
	   }
	  t.Authorized = auth.String()
	   t.Frozen = frz.String()
	  t.Residual = res.String()
  }
