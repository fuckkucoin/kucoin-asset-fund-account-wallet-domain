// 现货订单存储。
// 作者: Span
// 日期: 2021.01.19
package spot

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
)

const orderSelect = `SELECT id, idempotency_key, user_id, symbol, base_asset, quote_asset, side, type, size_amt, price, filled, quote_amount, fee_amount, fee_asset, status, fail_reason, created_at FROM spot_orders`

type orderBag struct {
	byID   map[string]*Order
	byIdem map[string]string
}

   func (s *Service) insertOrder(ctx context.Context, o *Order) error {
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		  _, err := sqls.Exec(ctx, `INSERT INTO spot_orders (id, idempotency_key, user_id, symbol, base_asset, quote_asset, side, type, size_amt, price, filled, quote_amount, fee_amount, fee_asset, status, fail_reason) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			  o.ID, o.IdempotencyKey, o.UserID, o.Symbol, o.Base, o.Quote, o.Side, o.Type, o.Size.StringFixed(18), o.Price.StringFixed(18), o.Filled.StringFixed(18), o.QuoteAmount.StringFixed(18), o.FeeAmount.StringFixed(18), o.FeeAsset, o.Status, o.FailReason)
		  if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			  return ledger.ErrIdempotency
		  }
		  return err
	  }
	   return s.memInsert(o)
  }

  func (s *Service) stampOrder(ctx context.Context, o *Order, lease ledger.Guard) error {
	   token := ""
	  if lease != nil {
		   token = lease.Token()
	  }
	  if token == "" {
		  token = uuid.NewString()
	  }
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   if _, err := sqls.Exec(ctx, `UPDATE spot_orders SET fence_epoch=fence_epoch+1, fence_token=? WHERE id=?`, token, o.ID); err != nil {
			  return err
		   }
		  var ep int64
		  if err := sqls.QueryRow(ctx, `SELECT fence_epoch FROM spot_orders WHERE id=?`, o.ID).Scan(&ep); err != nil {
			   return err
		  }
		  o.FenceEpoch = ep
		   o.FenceToken = token
		  return nil
	  }
	  s.memMu.Lock()
	   defer s.memMu.Unlock()
	   cur, ok := s.mem.byID[o.ID]
	  if !ok {
		  return ledger.ErrNotFound
	  }
	   cur.FenceEpoch++
	  cur.FenceToken = token
	   o.FenceEpoch = cur.FenceEpoch
	   o.FenceToken = token
	   return nil
   }

   func (s *Service) updateOrder(ctx context.Context, o *Order) error {
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   res, err := sqls.Exec(ctx, `UPDATE spot_orders SET filled=?, quote_amount=?, fee_amount=?, status=?, fail_reason=? WHERE id=? AND fence_epoch=? AND fence_token=?`,
			  o.Filled.StringFixed(18), o.QuoteAmount.StringFixed(18), o.FeeAmount.StringFixed(18), o.Status, o.FailReason, o.ID, o.FenceEpoch, o.FenceToken)
		  if err != nil {
			   return err
		  }
		   n, _ := res.RowsAffected()
		  if n == 0 {
			  metrics.FenceRejected.Inc()
			   return ledger.ErrStaleFence
		   }
		  return nil
	  }
	   s.memMu.Lock()
	  defer s.memMu.Unlock()
	  cur, ok := s.mem.byID[o.ID]
	  if !ok {
		  return ledger.ErrNotFound
	   }
	   if cur.FenceEpoch != o.FenceEpoch || cur.FenceToken != o.FenceToken {
		  metrics.FenceRejected.Inc()
		   return ledger.ErrStaleFence
	   }
	  cp := *o
	   s.mem.byID[o.ID] = &cp
	  s.mem.byIdem[o.IdempotencyKey] = o.ID
	  return nil
   }

   func (s *Service) listOrders(ctx context.Context, userID string) ([]*Order, error) {
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   rows, err := sqls.Query(ctx, orderSelect+` WHERE user_id=? ORDER BY created_at DESC LIMIT 100`, userID)
		   if err != nil {
			   return nil, err
		  }
		   defer rows.Close()
		  var out []*Order
		   for rows.Next() {
			  o, err := scanSpotRow(rows)
			  if err != nil {
				  return nil, err
			  }
			   out = append(out, o)
		   }
		  return out, rows.Err()
	   }
	  return s.memList(userID, ""), nil
  }

  func (s *Service) listOpen(ctx context.Context) ([]*Order, error) {
	  if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   rows, err := sqls.Query(ctx, orderSelect+` WHERE status IN (?,?) LIMIT 200`, StatusOpen, StatusPartial)
		   if err != nil {
			  return nil, err
		   }
		   defer rows.Close()
		   var out []*Order
		   for rows.Next() {
			  o, err := scanSpotRow(rows)
			  if err != nil {
				  return nil, err
			   }
			   out = append(out, o)
		   }
		   return out, rows.Err()
	  }
	   return s.memList("", StatusOpen), nil
   }

  func (s *Service) getByIdem(ctx context.Context, key string) (*Order, error) {
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		  return scanSpot(sqls.QueryRow(ctx, orderSelect+` WHERE idempotency_key=?`, key))
	   }
	  return s.memGetByIdem(key)
   }

  func (s *Service) getByID(ctx context.Context, id string) (*Order, error) {
	  if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   return scanSpot(sqls.QueryRow(ctx, orderSelect+` WHERE id=?`, id))
	   }
	   return s.memGetByID(id)
   }

  func (s *Service) memInsert(o *Order) error {
	  s.memMu.Lock()
	   defer s.memMu.Unlock()
	  if s.mem == nil {
		  s.mem = &orderBag{byID: map[string]*Order{}, byIdem: map[string]string{}}
	   }
	  if id, exists := s.mem.byIdem[o.IdempotencyKey]; exists && id != o.ID {
		   return ledger.ErrIdempotency
	  }
	  if o.CreatedAt.IsZero() {
		  o.CreatedAt = time.Now().UTC()
	  }
	  cp := *o
	  s.mem.byID[o.ID] = &cp
	   s.mem.byIdem[o.IdempotencyKey] = o.ID
	  return nil
   }

   func (s *Service) memGetByIdem(key string) (*Order, error) {
	   s.memMu.Lock()
	   defer s.memMu.Unlock()
	   if s.mem == nil {
		  return nil, ledger.ErrNotFound
	   }
	   id, ok := s.mem.byIdem[key]
	  if !ok {
		   return nil, ledger.ErrNotFound
	   }
	   o, ok := s.mem.byID[id]
	   if !ok {
		   return nil, ledger.ErrNotFound
	  }
	   cp := *o
	  return &cp, nil
   }

  func (s *Service) memGetByID(id string) (*Order, error) {
	  s.memMu.Lock()
	  defer s.memMu.Unlock()
	   if s.mem == nil {
		  return nil, ledger.ErrNotFound
	  }
	  o, ok := s.mem.byID[id]
	   if !ok {
		   return nil, ledger.ErrNotFound
	  }
	   cp := *o
	   return &cp, nil
  }

   func (s *Service) memList(userID string, status Status) []*Order {
	  s.memMu.Lock()
	   defer s.memMu.Unlock()
	   if s.mem == nil {
		   return nil
	   }
	  var out []*Order
	   for _, o := range s.mem.byID {
		   if userID != "" && o.UserID != userID {
			   continue
		   }
		  if status != "" && o.Status != status && !(status == StatusOpen && o.Status == StatusPartial) {
			  continue
		  }
		   cp := *o
		  out = append(out, &cp)
	  }
	   return out
   }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanSpotRow(row rowScanner) (*Order, error) {
	   o := &Order{}
	  var size, price, filled, quoteAmt, fee string
	  var fail sql.NullString
	  var created sql.NullTime
	   err := row.Scan(&o.ID, &o.IdempotencyKey, &o.UserID, &o.Symbol, &o.Base, &o.Quote, &o.Side, &o.Type, &size, &price, &filled, &quoteAmt, &fee, &o.FeeAsset, &o.Status, &fail, &created)
	   if err != nil {
		  return nil, err
	  }
	  if o.Size, err = ledger.ParseBook(size); err != nil {
		   return nil, err
	   }
	  if o.Price, err = ledger.ParseBook(price); err != nil {
		   return nil, err
	  }
	   if o.Filled, err = ledger.ParseBook(filled); err != nil {
		   return nil, err
	  }
	   if o.QuoteAmount, err = ledger.ParseBook(quoteAmt); err != nil {
		  return nil, err
	   }
	   if o.FeeAmount, err = ledger.ParseBook(fee); err != nil {
		  return nil, err
	  }
	   o.FailReason = fail.String
	  if created.Valid {
		  o.CreatedAt = created.Time
	  }
	  return o, nil
  }

   func scanSpot(row *sql.Row) (*Order, error) {
	  o, err := scanSpotRow(row)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ledger.ErrNotFound
	   }
	   return o, err
   }
