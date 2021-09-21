package p2pgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// DayWindows 按日聚合占用与流水。
  func (f *Funds) DayWindows(ctx context.Context, q OccupancyQuery) ([]DayBucket, error) {
	  q.prepare()
	  uid, err := validate.UserID(q.UserID)
	  if err != nil {
		  return nil, err
	   }
	  q.UserID = uid
	  views, err := f.ListHoldViews(ctx, q)
	   if err != nil {
		  return nil, err
	  }
	   flows,
		  err := f.ListFlowAll(ctx, q)
	  if err != nil {
		  return nil, err
	   }
	   days := WindowDays(q.StartAt,
		  q.EndAt)
	  if len(days) == 0 {
		   days = inferDays(views, flows)
	  }
	   by := map[string]*DayBucket{}
	   for _, d := range days {
		  by[d] = &DayBucket{Day: d, FlowCredit: "0", FlowDebit: "0"}
	   }
	   touch := func(day string) *DayBucket {
		  b := by[day]
		  if b == nil {
			   b = &DayBucket{Day: day, FlowCredit: "0", FlowDebit: "0"}
			  by[day] = b
		   }
		  return b
	   }
	   for _, v := range views {
		   day := DayKey(parseRFC3339(v.CreatedAt))
		   if day == "" {
			  continue
		  }
		   b := touch(day)
		   if v.Open {
			   b.HoldOpen++
		   } else {
			  b.HoldRelease++
		  }
	  }
	  for _, e := range flows {
		   if e.Adjacent {
			  continue
		   }
		   day := DayKey(time.UnixMilli(e.CreatedAtMs).UTC())
		  if e.CreatedAtMs == 0 {
			   day = DayKey(parseRFC3339(e.CreatedAt))
		   }
		   if day == "" {
			   continue
		   }
		  b := touch(day)
		  b.FlowCount++
		   if !flowCredited(e) {
			   continue
		  }
		  var acc decAcc
		  switch e.Sign {
		  case SignCredit:
			  acc.add(b.FlowCredit)
			  acc.add(e.Amount)
			   b.FlowCredit = acc.string()
		  case SignDebit:
			   acc.add(b.FlowDebit)
			  acc.add(e.Amount)
			  b.FlowDebit = acc.string()
		  }
	   }
	  out := make([]DayBucket, 0, len(by))
	   for _, b := range by {
		   out = append(out, *b)
	   }
	   sortDayBuckets(out)
	   return out, nil
   }

  func inferDays(views []HoldView, flows []FlowEntry) []string {
	   var minT, maxT time.Time
	  see := func(t time.Time) {
		   if t.IsZero() {
			  return
		   }
		   if minT.IsZero() || t.Before(minT) {
			  minT = t
		   }
		  if maxT.IsZero() || t.After(maxT) {
			  maxT = t
		   }
	   }
	  for _, v := range views {
		  see(parseRFC3339(v.CreatedAt))
	   }
	  for _, e := range flows {
		  if e.CreatedAtMs > 0 {
			  see(time.UnixMilli(e.CreatedAtMs).UTC())
		  } else {
			   see(parseRFC3339(e.CreatedAt))
		   }
	   }
	  if minT.IsZero() || maxT.IsZero() {
		   return nil
	   }
	   return WindowDays(minT, maxT)
   }
