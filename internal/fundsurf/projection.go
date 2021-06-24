package fundsurf

import (
	"context"
	"account-assets.kucoin.net/funding/internal/fundmat"
	"account-assets.kucoin.net/funding/internal/fundread"
)

// ProjMeta 投影读附带的水位与代次。最终一致读在未追平时 Stale 为真。
type ProjMeta struct {
	Watermarks []fundmat.MatWatermark `json:"watermarks"`
	Version    fundmat.ViewVersion    `json:"version"`
	Stale      bool                   `json:"stale"`
	Fence fundmat.ReadFence `json:"fence"`
}

// BindProjection 把冻结明细切到物化读模型。未绑定时资金页仍直接读账本。
   func (b *Book) BindProjection(p *fundread.Service) {
	  if b == nil {
		   return
	   }
	  b.proj = p
  }

// Projection 返回已绑定的读模型，未绑定则为空。
  func (b *Book) Projection() *fundread.Service {
	   if b == nil {
		  return nil
	  }
	   return b.proj
   }

// ProjectedFrozen 从物化投影取冻结栏。strong 为真时水位不足返回投影未追平。

  func (b *Book) ProjectedFrozen(ctx context.Context, userID string, strong bool, fenceSeq int64) ([]FrozenRow, ProjMeta, error) {
	  if b == nil || b.proj == nil {
		  return nil, ProjMeta{}, ErrStore
	   }
	  q := fundread.Query{UserID: userID, FenceSeq: fenceSeq}
	   var (
		   view fundread.View
		   err error
	   )
	   if strong {
		  view, err = b.proj.Strong(ctx, q)
	   } else {
		  view, err = b.proj.Eventual(ctx, q)
	   }
	  meta := ProjMeta{Watermarks: view.Watermarks,
		   Version: view.Version, Stale: view.Stale, Fence: view.Fence}
	   if err != nil {
		  return nil, meta, err
	  }
	   return frozenFromView(view), meta, nil
   }

  func frozenFromView(view fundread.View) []FrozenRow {
	   out := make([]FrozenRow, 0)
	   for _, row := range view.Rows {
		   if row.Frozen == "" || row.Frozen == "0" {
			   continue
		   }
		  out = append(out, FrozenRow{
			   Asset:   row.Asset,
			  Book:    row.Subject,
			   Subject: row.Subject,
			   Frozen:  row.Frozen,
			  Reason:  "projection",
			  Ref: row.LastEventID,
		  })
	  }
	  return out
  }
