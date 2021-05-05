package feequotelease

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/feeeng"
)

// FeeEngAdapter 将 feeeng.Book 适配为 FeeScheduleReader。

type FeeEngAdapter struct {
	book *feeeng.Book
}

// NewFeeEngAdapter 创建适配器实例。

  func NewFeeEngAdapter(book *feeeng.Book) *FeeEngAdapter {
	  return &FeeEngAdapter{book: book}
  }

// GetSchedule 从 feeeng.Book 只读获取 Schedule 快照。
   func (a *FeeEngAdapter) GetSchedule(_ context.Context, kind, asset string) (ScheduleSnapshot, error) {
	   if a == nil || a.book == nil {
		  return ScheduleSnapshot{}, fmt.Errorf("fee eng adapter: book nil")
	  }
	   sch, err := a.book.Get(feeeng.Kind(kind), asset)
	  if err != nil {
		  return ScheduleSnapshot{}, err
	  }
	   return ScheduleSnapshot{
		   ID:     sch.ID,
		   Kind:   string(sch.Kind),
		  Asset:  sch.Asset,
		  BPS:    sch.BPS,
		  MinFee: sch.MinFee,
		   MaxFee: sch.MaxFee,
		   Flat: sch.Flat,
		  Version: sch.Version,
	  }, nil
  }
