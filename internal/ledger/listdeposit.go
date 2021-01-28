package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DepositHistQuery 充值历史分页与保留窗查询。
// 超越 List(200)；page/pageSize；start/end；最长保留约一年。
// 超保留窗外拒绝；缺窗默认近保留窗上限。
type DepositHistQuery struct {
	UserID string
	Page   int
	PageSize int
	Start    time.Time
	End      time.Time
	Currency string
	Status    string
	InnerMode string // ""=双源合并 / inner / chain；见 DepositInner*
}

const (
	DepositHistMaxPage     = 100
	DepositHistDefaultPage = 20
	DepositHistRetention   = 365 * 24 * time.Hour
)

var ErrHistWindow = fmt.Errorf("deposit history window exceeds retention")

// Normalize 校正分页与保留窗。

  func (q *DepositHistQuery) Normalize(now time.Time) error {
	  if q == nil {
		   return fmt.Errorf("query nil")
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  if q.Page <= 0 {
		   q.Page = 1
	   }
	  if q.PageSize <= 0 {
		   q.PageSize = DepositHistDefaultPage
	   }
	   if q.PageSize > DepositHistMaxPage {
		   q.PageSize = DepositHistMaxPage
	   }
	  retStart := now.Add(-DepositHistRetention)
	   if q.Start.IsZero() && q.End.IsZero() {
		   q.Start = retStart
		  q.End = now
		   return nil
	  }
	   if q.Start.IsZero() {
		  q.Start = retStart
	  }
	   if q.End.IsZero() {
		   q.End = now
	  }
	   if q.End.Before(q.Start) {
		   return ErrHistWindow
	   }
	  if q.Start.Before(retStart) {
		   return ErrHistWindow
	   }
	   if q.End.Sub(q.Start) > DepositHistRetention {
		   return ErrHistWindow
	   }
	   q.Currency = strings.ToUpper(strings.TrimSpace(q.Currency))
	   q.Status = strings.ToUpper(strings.TrimSpace(q.Status))
	   return nil
   }

   func (q DepositHistQuery) Offset() int {
	  return (q.Page - 1) * q.PageSize
  }

// ListDeposits 充值历史分页（链上入金方向）。
  func (s *Service) ListDeposits(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   if s == nil || s.store == nil {
		   return nil, 0, ErrNotFound
	   }
	   if err := q.Normalize(time.Now().UTC()); err != nil {
		  return nil, 0, err
	   }
	  return s.store.ListUserTransfersPaged(ctx, q)
  }
