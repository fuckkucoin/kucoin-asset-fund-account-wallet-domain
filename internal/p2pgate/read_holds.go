package p2pgate

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func (f *Funds) listOccupancyRows(ctx context.Context, userID string, openOnly bool) ([]holdcat.Row, error) {
	   if f == nil || f.hold == nil {
		   return nil, ErrHold
	   }
	   uid,
		  err := validate.UserID(userID)
	   if err != nil {
		  return nil, err
	  }
	   rows, err := f.hold.List(ctx, uid, openOnly)
	   if err != nil {
		  return nil, err
	   }
	  return occupancyRows(rows), nil
  }

   func (f *Funds) loadHoldViews(ctx context.Context, userID string) ([]HoldView, transferIndex, error) {
	   rows, err := f.listOccupancyRows(ctx, userID, false)
	  if err != nil {
		   return nil, transferIndex{}, err
	   }
	  idx,
		  err := f.occupancyIndex(ctx, userID)
	  if err != nil {
		  return nil, transferIndex{}, err
	  }
	  return viewsOf(rows,
		  idx), idx, nil
   }

// ListDetails 占用明细（含已释放）。默认全状态、最新在前。

   func (f *Funds) ListDetails(ctx context.Context, q OccupancyQuery) (HoldPage, error) {
	  q.prepare()
	  if strings.TrimSpace(q.UserID) == "" {
		   return HoldPage{}, ErrHold
	  }
	   views, _, err := f.loadHoldViews(ctx, q.UserID)
	   if err != nil {
		  return HoldPage{}, err
	  }
	   matched := filterHolds(views, q)
	   sortHoldViews(matched, q.SortAsc)
	   items,
		   meta := slicePage(matched, q.Page, q.PageSize)
	  if items == nil {
		  items = []HoldView{}
	   }
	  return HoldPage{Items: items, Page: meta}, nil
  }

// ListHoldViews 不过分页的全量明细（供汇总与对账）。
  func (f *Funds) ListHoldViews(ctx context.Context, q OccupancyQuery) ([]HoldView, error) {
	  q.prepare()
	  if strings.TrimSpace(q.UserID) == "" {
		   return nil, ErrHold
	   }
	   views, _, err := f.loadHoldViews(ctx, q.UserID)
	   if err != nil {
		  return nil, err
	  }
	   matched := filterHolds(views, q)
	  sortHoldViews(matched, q.SortAsc)
	   return matched, nil
  }

// GetHoldView 按占用 id 取视图；属主不匹配视为不存在。
  func (f *Funds) GetHoldView(ctx context.Context, userID, holdID string) (HoldView, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return HoldView{}, err
	  }
	  holdID = strings.TrimSpace(holdID)
	  if holdID == "" {
		   return HoldView{}, ErrHoldGone
	  }
	   if f == nil || f.hold == nil {
		   return HoldView{}, ErrHold
	   }
	   row, err := f.hold.Get(ctx, uid, holdID)
	   if err != nil {
		  if errors.Is(err, holdcat.ErrGone) || errors.Is(err, holdcat.ErrOwner) {
			  return HoldView{}, ErrHoldGone
		   }
		   return HoldView{}, err
	  }
	   if !IsOccupancyReason(row.Reason) {
		  return HoldView{}, ErrHoldGone
	   }
	  idx, err := f.occupancyIndex(ctx, uid)
	  if err != nil {
		   return HoldView{}, err
	   }
	  return viewHold(row, idx), nil
  }

// CountOpenByKind 开放占用计数（资金页徽标）。

   func (f *Funds) CountOpenByKind(ctx context.Context, userID string) (sell, merchant int, err error) {
	   rows, err := f.listOccupancyRows(ctx, userID, true)
	   if err != nil {
		  return 0, 0, err
	  }
	   for _, r := range rows {
		   switch r.Reason {
		  case holdcat.ReasonP2PSell:
			   sell++
		   case holdcat.ReasonP2PMargin:
			  merchant++
		   }
	  }
	   return sell, merchant, nil
   }
