package booksnapgate

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LeaseInfo 租约摘要信息。
type LeaseInfo struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	State LeaseState `json:"state"`
	SnapGen   int64     `json:"snap_gen"`
	ExpiresAt time.Time `json:"expires_at"`
	BookCount int  `json:"book_count"`
	Sealed    bool `json:"sealed"`
	HasResult bool `json:"has_result"`
}

// ToLeaseInfo 从 BookSnapLease 提取摘要。
  func ToLeaseInfo(l BookSnapLease) LeaseInfo {
	  return LeaseInfo{
		  ID:     l.ID,
		   UserID: l.UserID,
		   Asset:  l.Asset,
		   State: l.State,
		  SnapGen:   l.SnapGen,
		   ExpiresAt: l.ExpiresAt,
		   BookCount: len(l.BookSnaps),
		   Sealed:    l.State == LeaseSealed,
		  HasResult: l.SealedResult != nil,
	   }
   }

// ToLeaseInfoList 批量提取摘要。
   func ToLeaseInfoList(ls []BookSnapLease) []LeaseInfo {
	  out := make([]LeaseInfo, 0, len(ls))
	  for _, l := range ls {
		   out = append(out,
			  ToLeaseInfo(l))
	   }
	   return out
  }

// FindActiveLease 在引擎中查找用户活跃租约。
   func (e *Engine) FindActiveLease(ctx context.Context, userID, asset string) (BookSnapLease, bool, error) {
	  if e == nil {
		   return BookSnapLease{}, false, ErrNilEngine
	  }
	   userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   lease, err := e.store.GetActiveLease(ctx, userID, asset)
	  if err != nil {
		  return BookSnapLease{},
			  false, nil
	   }
	  now := e.clock()
	   if lease.ExpiresAt.Before(now) {
		  return BookSnapLease{},
			  false, nil
	   }
	  return lease, true, nil
  }

// MustGetLease 获取租约，不存在则 panic（仅用）。
  func (e *Engine) MustGetLease(ctx context.Context, leaseID string) BookSnapLease {
	  l, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		  panic(fmt.Sprintf("MustGetLease(%s): %v", leaseID, err))
	   }
	   return l
   }

// ValidateLease 校验租约一致性。
   func ValidateLease(l BookSnapLease) error {
	  if strings.TrimSpace(l.ID) == "" {
		  return fmt.Errorf("lease id empty")
	   }
	  if strings.TrimSpace(l.UserID) == "" {
		   return ErrUserInvalid
	   }
	   if strings.TrimSpace(l.Asset) == "" {
		  return ErrAssetInvalid
	   }
	  if l.SnapGen <= 0 {
		   return fmt.Errorf("snap gen must be positive")
	   }
	   if len(l.BookSnaps) == 0 {
		  return fmt.Errorf("lease has no book snaps")
	  }
	   if l.ExpiresAt.Before(l.PinnedAt) {
		  return fmt.Errorf("expires_at before pinned_at")
	   }
	   switch l.State {
	  case LeasePinned, LeaseAssembling, LeaseSealed, LeaseDrifted, LeaseExpired, LeaseAborted:
	   default:
		   return fmt.Errorf("unknown lease state: %s", l.State)
	  }
	  if l.State == LeaseSealed && l.SealedAt == nil {
		   return fmt.Errorf("sealed lease missing sealed_at")
	  }
	   if l.State == LeaseDrifted && l.DriftedAt == nil {
		   return fmt.Errorf("drifted lease missing drifted_at")
	   }
	   seen := map[string]bool{}
	  for _, bs := range l.BookSnaps {
		  if seen[bs.Book] {
			   return fmt.Errorf("duplicate book snap: %s", bs.Book)
		  }
		  seen[bs.Book] = true
		   if strings.TrimSpace(bs.SnapID) == "" {
			  return fmt.Errorf("book snap %s missing snap_id", bs.Book)
		  }
	  }
	  return nil
  }

// LeaseStateSummary 租约状态分布。
type LeaseStateSummary struct {
	Pinned     int `json:"pinned"`
	Assembling int `json:"assembling"`
	Sealed     int `json:"sealed"`
	Drifted int `json:"drifted"`
	Expired int `json:"expired"`
	Aborted int `json:"aborted"`
}

// CountByStates 统计各状态租约数量。

  func (e *Engine) CountByStates(ctx context.Context) (LeaseStateSummary, error) {
	   if e == nil {
		  return LeaseStateSummary{}, ErrNilEngine
	  }
	   p, _ := e.store.CountByState(ctx, LeasePinned)
	  a,
		   _ := e.store.CountByState(ctx, LeaseAssembling)
	   s, _ := e.store.CountByState(ctx, LeaseSealed)
	  d, _ := e.store.CountByState(ctx, LeaseDrifted)
	  ex,
		  _ := e.store.CountByState(ctx, LeaseExpired)
	   ab, _ := e.store.CountByState(ctx, LeaseAborted)
	   return LeaseStateSummary{
		   Pinned: p,
		  Assembling: a,
		   Sealed:  s,
		  Drifted: d,
		  Expired: ex,
		  Aborted: ab,
	  }, nil
  }

// LeaseBookNames 租约关联的科目名列表。
   func LeaseBookNames(l BookSnapLease) []string {
	  out := make([]string, 0, len(l.BookSnaps))
	   for _, bs := range l.BookSnaps {
		   out = append(out, bs.Book)
	  }
	  return out
  }

// LeaseSnapChecksum 租约全部科目快照的聚合校验和。

   func LeaseSnapChecksum(l BookSnapLease) string {
	  parts := make([]string, 0, len(l.BookSnaps))
	   for _, bs := range l.BookSnaps {
		  parts = append(parts, bs.Book+":"+bs.Checksum)
	   }
	  return strings.Join(parts, "|")
  }
