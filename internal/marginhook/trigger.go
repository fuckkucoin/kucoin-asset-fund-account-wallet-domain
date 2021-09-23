package marginhook

import "time"

// BorrowSnapshot 是 liqplan 所需的借款只读快照。
// 本类型与 liqplan.BorrowSnapshot 平行定义，避免循环依赖。

type BorrowSnapshot struct {
	BorrowID string
	UserID       string
	BorrowAsset  string
	BorrowAmount string
	CollateralID string
	CollateralAsset  string
	CollateralAmount string
	LTVBPS           int
	CapBPS           int
}

// LTVObserver 允许外部（如 liqplan）只读观察当前 LTV 状态。
// 不暴露 Book 内部指针；数据以快照形式返回。

type LTVObserver struct {
	svc *Service
}

// NewLTVObserver 创建 LTV 只读观察器（不改写 Book 状态）。
  func NewLTVObserver(svc *Service) *LTVObserver {
	   return &LTVObserver{svc: svc}
   }

// CurrentLTV 实现 liqplan.LTVReader 接口：返回借款当前 LTV（万分比）与 cap。
   func (o *LTVObserver) CurrentLTV(borrowID string) (ltvBPS, capBPS int, err error) {
	  if o == nil || o.svc == nil {
		  return 0, 0, ErrUnavailable
	  }
	  r, ok := o.svc.book.Borrow(borrowID)
	  if !ok {
		   return 0, 0, ErrNotFound
	   }
	   h, ok := o.svc.book.Hold(r.Collateral)
	  if !ok {
		   return 0, 0, ErrNotFound
	   }
	  ltv, err := LTV(r.Amount, h.Amount)
	  if err != nil {
		  return 0, 0, err
	   }
	  return ltv, h.CapBPS, nil
   }

// BorrowInfo 实现 liqplan.BorrowReader 接口：返回借款只读快照字段。
  func (o *LTVObserver) BorrowInfo(borrowID string) (
	  userID, borrowAsset, borrowAmount, collateralID, collateralAsset, collateralAmount string, err error,
   ) {
	  if o == nil || o.svc == nil {
		  return "", "", "", "", "", "", ErrUnavailable
	  }
	   r, ok := o.svc.book.Borrow(borrowID)
	   if !ok {
		  return "", "", "", "", "", "", ErrNotFound
	   }
	   h, ok := o.svc.book.Hold(r.Collateral)
	  if !ok {
		  return "", "", "", "", "", "", ErrNotFound
	  }
	  return r.UserID, r.Asset, r.Amount, h.ID, h.Asset, h.Amount, nil
   }

// ListOpenBorrows 实现 liqplan.BorrowLister 接口：返回开仓借款快照列表。

  func (o *LTVObserver) ListOpenBorrows() []BorrowSnapshot {
	  if o == nil || o.svc == nil {
		   return nil
	  }
	  borrows := o.svc.book.OpenBorrows()
	   out := make([]BorrowSnapshot,
		   0, len(borrows))
	   for _, r := range borrows {
		   h, ok := o.svc.book.Hold(r.Collateral)
		   if !ok {
			   continue
		  }
		   ltv, _ := LTV(r.Amount, h.Amount)
		   out = append(out, BorrowSnapshot{
			  BorrowID: r.ID,
			  UserID: r.UserID,
			   BorrowAsset:  r.Asset,
			   BorrowAmount: r.Amount,
			  CollateralID:     h.ID,
			   CollateralAsset:  h.Asset,
			  CollateralAmount: h.Amount,
			  LTVBPS: ltv,
			  CapBPS: h.CapBPS,
		   })
	  }
	   return out
   }

// TriggerHook 是 liqplan 回调接口，当 LTV 扫描发现穿仓时调用。
// marginhook 保持只读；liqplan 负责实际计划生成。

type TriggerHook interface {
	OnLTVBreach(snap BorrowSnapshot) error
}

// ScanWithHook 在口径 Scan 之外，对每个穿仓借款调用 hook.OnLTVBreach。
// hook 可以是 nil（降级为口径 Scan）。

  func (s *Service) ScanWithHook(hook TriggerHook) ScanReport {
	   out := ScanReport{}
	   if s == nil {
		  return out
	   }
	  seen := map[string]struct{}{}
	  for _, r := range s.book.OpenBorrows() {
		  if _, ok := seen[r.UserID]; !ok {
			   seen[r.UserID] = struct{}{}
			  out.Users++
		  }
		   out.Checked++
		  h, ok := s.book.Hold(r.Collateral)
		  if !ok {
			   out.Errs++
			  continue
		  }
		  if err := CheckLTV(r.Amount, h.Amount, h.CapBPS); err == nil {
			   out.Inside++
			   continue
		   }
		  ltv, _ := LTV(r.Amount, h.Amount)
		   if hook != nil {
			  snap := BorrowSnapshot{
				   BorrowID: r.ID,
				  UserID:       r.UserID,
				  BorrowAsset:  r.Asset,
				   BorrowAmount: r.Amount,
				  CollateralID: h.ID,
				  CollateralAsset:  h.Asset,
				  CollateralAmount: h.Amount,
				  LTVBPS: ltv,
				   CapBPS: h.CapBPS,
			   }
			  if err := hook.OnLTVBreach(snap); err != nil {
				  out.Errs++
				  continue
			   }
		   }
		   if _, err := s.FreezeForLTV(r.UserID, r.ID); err != nil {
			   out.Errs++
			  continue
		   }
		  out.Frozen++
	  }
	   return out
  }

// MarkObserver 为标价世代观察器接口（只读消费 oracle/fxmark）。
// 与行情主路径解耦；此接口供 liqplan.MarkReader 对接用。
type MarkObserver interface {
	LatestMark(asset string) (price string, generation int64, observedAt time.Time, err error)
}
