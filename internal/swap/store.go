// 兑换订单存储。
// 作者: Span
// 日期: 2020.11.12
package swap

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

const orderSelect = `SELECT id, idempotency_key, user_id, from_asset, to_asset, from_amount, quote_rate, filled_amount, to_amount, fee_asset, fee_amount, status, fail_reason, chain, tx_hash, created_at, COALESCE(freeze_lease_id,''), COALESCE(thaw_proof_id,'') FROM swap_orders`

type orderBag struct {
	byID   map[string]*Order
	byIdem map[string]string
}

// insertOrder 落兑换单；幂等键冲突交给上层 resume。

  func (s *Service) insertOrder(ctx context.Context, o *Order) error {
	  if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   _, err := sqls.Exec(ctx, `INSERT INTO swap_orders (id, idempotency_key, user_id, from_asset, to_asset, from_amount, quote_rate, filled_amount, to_amount, fee_asset, fee_amount, status, fail_reason, chain, tx_hash, freeze_lease_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			  o.ID, o.IdempotencyKey, o.UserID, o.FromAsset, o.ToAsset, o.FromAmount.StringFixed(18), o.QuoteRate.StringFixed(18), o.FilledAmount.StringFixed(18), o.ToAmount.StringFixed(18), o.FeeAsset, o.FeeAmount.StringFixed(18), o.Status, o.FailReason, o.Chain, o.TxHash, o.FreezeLeaseID)
		  if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			  return ledger.ErrIdempotency
		  }
		   return err
	   }
	   return s.memInsert(o)
  }

// stampOrder 持锁后盖 fence，丢租约的 worker 改不了这张兑换单。
   func (s *Service) stampOrder(ctx context.Context, o *Order, lease ledger.Guard) error {
	  token := ""
	  if lease != nil {
		  token = lease.Token()
	   }
	   if token == "" {
		   token = uuid.NewString()
	   }
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		  if _, err := sqls.Exec(ctx, `UPDATE swap_orders SET fence_epoch=fence_epoch+1, fence_token=? WHERE id=?`, token, o.ID); err != nil {
			  return err
		   }
		   var ep int64
		  row := sqls.QueryRow(ctx, `SELECT fence_epoch FROM swap_orders WHERE id=?`, o.ID)
		  if err := row.Scan(&ep); err != nil {
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
		  res, err := sqls.Exec(ctx, `UPDATE swap_orders SET filled_amount=?, to_amount=?, fee_amount=?, status=?, fail_reason=?, chain=?, tx_hash=?, thaw_proof_id=? WHERE id=? AND fence_epoch=? AND fence_token=?`,
			  o.FilledAmount.StringFixed(18), o.ToAmount.StringFixed(18), o.FeeAmount.StringFixed(18), o.Status, o.FailReason, o.Chain, o.TxHash, o.ThawProofID, o.ID, o.FenceEpoch, o.FenceToken)
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
			  o, err := scanOrderRow(rows)
			   if err != nil {
				  return nil, err
			  }
			  out = append(out, o)
		   }
		  return out, rows.Err()
	  }
	   return s.memList(userID), nil
   }

  func (s *Service) getByIdem(ctx context.Context, key string) (*Order, error) {
	  if sqls, ok := s.st.(*ledger.SQLStore); ok {
		  return scanOrder(sqls.QueryRow(ctx, orderSelect+` WHERE idempotency_key=?`, key))
	  }
	   return s.memGetByIdem(key)
  }

   func (s *Service) getByID(ctx context.Context, id string) (*Order, error) {
	   if sqls, ok := s.st.(*ledger.SQLStore); ok {
		   return scanOrder(sqls.QueryRow(ctx, orderSelect+` WHERE id=?`, id))
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

  func (s *Service) memPut(o *Order) {
	   s.memMu.Lock()
	  defer s.memMu.Unlock()
	   if s.mem == nil {
		   s.mem = &orderBag{byID: map[string]*Order{}, byIdem: map[string]string{}}
	  }
	   cp := *o
	  s.mem.byID[o.ID] = &cp
	   s.mem.byIdem[o.IdempotencyKey] = o.ID
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
	  tagOrder(&cp)
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
	  tagOrder(&cp)
	   return &cp, nil
  }

   func (s *Service) memList(userID string) []*Order {
	  s.memMu.Lock()
	   defer s.memMu.Unlock()
	  if s.mem == nil {
		   return nil
	  }
	  var out []*Order
	   for _, o := range s.mem.byID {
		  if o.UserID == userID {
			   cp := *o
			  tagOrder(&cp)
			   out = append(out, &cp)
		  }
	  }
	  return out
  }

// listFrozen RecoverFrozen 扫卡住的冻结单。

  func (s *Service) listFrozen(ctx context.Context) ([]*Order, error) {
	  if sqls, ok := s.st.(*ledger.SQLStore); ok {
		  rows, err := sqls.Query(ctx, orderSelect+` WHERE status=? LIMIT 200`, StatusFrozen)
		  if err != nil {
			   return nil, err
		  }
		  defer rows.Close()
		   var out []*Order
		  for rows.Next() {
			  o, err := scanOrderRow(rows)
			   if err != nil {
				   return nil, err
			   }
			   out = append(out, o)
		  }
		   return out, rows.Err()
	   }
	  s.memMu.Lock()
	   defer s.memMu.Unlock()
	  if s.mem == nil {
		  return nil, nil
	   }
	  var out []*Order
	  for _, o := range s.mem.byID {
		  if o.Status == StatusFrozen {
			  cp := *o
			   out = append(out, &cp)
		   }
	  }
	  return out, nil
   }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanOrderRow(row rowScanner) (*Order, error) {
	  o := &Order{}
	   var fromAmt, rate, filled, toAmt, fee string
	  var fail, chain, txh sql.NullString
	   var created sql.NullTime
	   var freezeLeaseID, thawProofID string
	  err := row.Scan(&o.ID, &o.IdempotencyKey, &o.UserID, &o.FromAsset, &o.ToAsset, &fromAmt, &rate, &filled, &toAmt, &o.FeeAsset, &fee, &o.Status, &fail, &chain, &txh, &created, &freezeLeaseID, &thawProofID)
	   if err != nil {
		  return nil, err
	  }
	   if err := fillOrderDecimals(o, fromAmt, rate, filled, toAmt, fee, fail.String, chain.String, txh.String); err != nil {
		   return nil, err
	  }
	  if created.Valid {
		   o.CreatedAt = created.Time
	  }
	  o.FreezeLeaseID = freezeLeaseID
	   o.ThawProofID = thawProofID
	  return o, nil
   }

  func scanOrder(row *sql.Row) (*Order, error) {
	   o, err := scanOrderRow(row)
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ledger.ErrNotFound
	   }
	   return o, err
  }

   func fillOrderDecimals(o *Order, fromAmt, rate, filled, toAmt, fee, fail, chain, txh string) error {
	  var err error
	   if o.FromAmount, err = ledger.ParseBook(fromAmt); err != nil {
		  return err
	  }
	  if o.QuoteRate, err = ledger.ParseBook(rate); err != nil {
		  return err
	  }
	   if o.FilledAmount, err = ledger.ParseBook(filled); err != nil {
		  return err
	  }
	  if o.ToAmount, err = ledger.ParseBook(toAmt); err != nil {
		  return err
	  }
	   if o.FeeAmount, err = ledger.ParseBook(fee); err != nil {
		  return err
	  }
	   o.FailReason = fail
	  o.Chain = chain
	  o.TxHash = txh
	  tagOrder(o)
	  return nil
  }

   func tagOrder(o *Order) {
	  if o == nil {
		  return
	   }
	   o.Kind = "swap"
	  o.MovementID = o.ID
   }
