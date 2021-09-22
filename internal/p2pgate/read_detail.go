package p2pgate

import (
	"context"
	"strings"
	"time"
)

  func buildTimeline(v HoldView, debit, rel *FlowEntry) []TimelineEvent {
	   out := make([]TimelineEvent, 0, 4)
	  out = append(out, TimelineEvent{
		  At: v.CreatedAt, AtMs: v.CreatedAtMs, Kind: "hold_open",
		  Amount: v.Amount, Note: "holdcat " + v.Reason,
	   })
	  if debit != nil {
		  out = append(out, TimelineEvent{
			   At: debit.CreatedAt, AtMs: debit.CreatedAtMs, Kind: "ledger_debit",
			  TransferID: debit.TransferID, Amount: debit.Amount, State: debit.State,
			  Note: debit.IdempotencyKey,
		  })
	  }
	   if !v.Open {
		  at := v.CreatedAt
		   atMs := v.CreatedAtMs
		  if rel != nil {
			   at, atMs = rel.CreatedAt, rel.CreatedAtMs
		  }
		  out = append(out, TimelineEvent{
			   At: at, AtMs: atMs, Kind: "hold_release", Amount: v.Amount,
		   })
	  }
	   if rel != nil {
		   out = append(out, TimelineEvent{
			   At: rel.CreatedAt, AtMs: rel.CreatedAtMs, Kind: "ledger_credit",
			   TransferID: rel.TransferID, Amount: rel.Amount, State: rel.State,
			   Note: rel.IdempotencyKey,
		   })
	   }
	   // 时间线按毫秒升序；同毫秒保持开立→借记→释放→贷记。
	  sortTimeline(out)
	   return out
  }

   func sortTimeline(ev []TimelineEvent) {
	   for i := 1; i < len(ev); i++ {
		   for j := i; j > 0; j-- {
			   if ev[j].AtMs >= ev[j-1].AtMs {
				  break
			   }
			  ev[j], ev[j-1] = ev[j-1], ev[j]
		  }
	  }
  }

  func composeDetail(v HoldView, idx transferIndex) DetailView {
	   debit := idx.flowOf(v.DebitKey)
	   rel := idx.flowOf(v.ReleaseKey)
	   return DetailView{
		   Hold: v,
		   Timeline: buildTimeline(v, debit, rel),
		  Debit:    debit,
		  Release:  rel,
	  }
   }

// GetDetail 单笔占用明细。
   func (f *Funds) GetDetail(ctx context.Context, userID, holdID string) (DetailView, error) {
	  v, err := f.GetHoldView(ctx, userID, holdID)
	   if err != nil {
		  return DetailView{}, err
	   }
	   idx, err := f.occupancyIndex(ctx, strings.TrimSpace(userID))
	  if err != nil {
		  return DetailView{}, err
	   }
	   return composeDetail(v, idx), nil
   }

// ListDetailViews 明细分页（每条带时间线）。
   func (f *Funds) ListDetailViews(ctx context.Context, q OccupancyQuery) (DetailPage, error) {
	   q.prepare()
	  views, idx, err := f.loadHoldViews(ctx, q.UserID)
	   if err != nil {
		   return DetailPage{}, err
	  }
	   matched := filterHolds(views, q)
	   sortHoldViews(matched, q.SortAsc)
	  page, meta := slicePage(matched, q.Page, q.PageSize)
	   items := make([]DetailView, 0, len(page))
	   for _, v := range page {
		  items = append(items, composeDetail(v, idx))
	  }
	  return DetailPage{Items: items,
		  Page: meta}, nil
   }

   func ageOf(v HoldView, now time.Time) time.Duration {
	  if now.IsZero() {
		  now = time.Now().UTC()
	  }
	  ts := parseRFC3339(v.CreatedAt)
	  if ts.IsZero() {
		   return 0
	   }
	   if now.Before(ts) {
		   return 0
	  }
	   return now.Sub(ts)
   }

// OpenHoldAge 开放占用存续时间（运维读）。
   func (f *Funds) OpenHoldAge(ctx context.Context, userID, holdID string) (time.Duration, error) {
	  v, err := f.GetHoldView(ctx, userID, holdID)
	   if err != nil {
		  return 0, err
	  }
	  if !v.Open {
		   return 0, nil
	  }
	   return ageOf(v, time.Now().UTC()), nil
  }
