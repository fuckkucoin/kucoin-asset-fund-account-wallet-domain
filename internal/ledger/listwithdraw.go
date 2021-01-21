package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// WithdrawHistQuery 提现历史分页与保留窗查询（与充值历史同模式）。
// 超越 List(200)；page/pageSize；start/end；最长保留约一年。
// 超保留窗外拒绝；缺窗默认近保留窗上限。
type WithdrawHistQuery struct {
	UserID   string
	Page     int
	PageSize int
	Start time.Time
	End time.Time
	Currency string
	Status   string
	// InnerFilter: ""=全部 / "true"=仅内部转出 / "false"=仅链上出金（SQL 层）。
	InnerFilter string
}

const (
	WithdrawHistMaxPage     = DepositHistMaxPage
	WithdrawHistDefaultPage = DepositHistDefaultPage
	WithdrawHistRetention   = DepositHistRetention
)

var ErrWithdrawHistWindow = fmt.Errorf("withdraw history window exceeds retention")

// Normalize 校正分页与保留窗（同充值）。

   func (q *WithdrawHistQuery) Normalize(now time.Time) error {
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
		   q.PageSize = WithdrawHistDefaultPage
	  }
	   if q.PageSize > WithdrawHistMaxPage {
		  q.PageSize = WithdrawHistMaxPage
	   }
	  retStart := now.Add(-WithdrawHistRetention)
	   if q.Start.IsZero() && q.End.IsZero() {
		   q.Start = retStart
		  q.End = now
		   q.Currency = strings.ToUpper(strings.TrimSpace(q.Currency))
		   q.Status = strings.ToUpper(strings.TrimSpace(q.Status))
		  return nil
	   }
	  if q.Start.IsZero() {
		   q.Start = retStart
	   }
	   if q.End.IsZero() {
		   q.End = now
	   }
	   if q.End.Before(q.Start) {
		  return ErrWithdrawHistWindow
	   }
	  if q.Start.Before(retStart) {
		   return ErrWithdrawHistWindow
	  }
	  if q.End.Sub(q.Start) > WithdrawHistRetention {
		   return ErrWithdrawHistWindow
	   }
	  q.Currency = strings.ToUpper(strings.TrimSpace(q.Currency))
	  q.Status = strings.ToUpper(strings.TrimSpace(q.Status))
	   return nil
   }

   func (q WithdrawHistQuery) Offset() int {
	  return (q.Page - 1) * q.PageSize
   }

// SetInnerFilter 由 isInner 查询串设定出金内外过滤（SQL 层）。

  func (q *WithdrawHistQuery) SetInnerFilter(isInnerParam string) {
	   if q == nil {
		  return
	  }
	  switch strings.ToLower(strings.TrimSpace(isInnerParam)) {
	  case "true", "1", "yes", "inner":
		   q.InnerFilter = "true"
	   case "false", "0", "no", "chain":
		   q.InnerFilter = "false"
	  default:
		   q.InnerFilter = ""
	   }
   }

// ListWithdraws 提现历史分页（链上出金 + 内部转出）。

  func (s *Service) ListWithdraws(ctx context.Context, q WithdrawHistQuery) ([]*Transfer, int, error) {
	  if s == nil || s.store == nil {
		   return nil, 0, ErrNotFound
	  }
	   if err := q.Normalize(time.Now().UTC()); err != nil {
		  return nil, 0, err
	   }
	   return s.store.ListUserWithdrawsPaged(ctx, q)
  }

// ListIncomingInnerPaged 充值历史中内部转入真 SQL 分页。
   func (s *Service) ListIncomingInnerPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   if s == nil || s.store == nil {
		  return nil, 0, ErrNotFound
	   }
	  if err := q.Normalize(time.Now().UTC()); err != nil {
		   return nil, 0, err
	  }
	  return s.store.ListIncomingInnerPaged(ctx, q)
  }
