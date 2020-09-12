package persist

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type DepositSnap struct {
	Chain string
	Hash        string
	OutputIndex int
	UserID string
	Asset  string
	Amount string
	Epoch int64
	Token string
}

type WithdrawSnap struct {
	ID     string
	UserID string
	Asset string
	Amount string
	State string
	Idem  string
	Epoch int64
	Token string
}

type CustodySnap struct {
	Asset string
	Side  string
	Available string
	Frozen    string
	Epoch     int64
	Token string
}

type ReconReport struct {
	Deposits  int `json:"deposits"`
	Withdraws int `json:"withdraws"`
	Custody   int `json:"custody"`
	OK bool `json:"ok"`
	Drift []string `json:"drift,omitempty"`
}

   func (r ReconReport) Err() error {
	  if r.OK {
		  return nil
	  }
	   if len(r.Drift) == 0 {
		  return ErrRecon
	   }
	   return fmt.Errorf("%w: %s", ErrRecon, strings.Join(r.Drift, "; "))
  }

// ReconBooks 对账内存快照。入金金额必须过 Parse；出金 requested 侧与 custody 不在此轧平用户账。
// 漂移条件：重复 (chain,tx_hash,output_index)、科学计数、负库存、热/冷同一资产可用+冻结无法相加。
   func ReconBooks(deps []DepositSnap, wds []WithdrawSnap, inv []CustodySnap) ReconReport {
	  out := ReconReport{Deposits: len(deps),
		  Withdraws: len(wds), Custody: len(inv), OK: true}
	   seenHash := map[string]string{}
	  for i, d := range deps {
		   key := fmt.Sprintf("%s|%s|%d", strings.ToUpper(strings.TrimSpace(d.Chain)), strings.TrimSpace(d.Hash), d.OutputIndex)
		  if strings.TrimSpace(d.Chain) == "" || strings.TrimSpace(d.Hash) == "" {
			  out.OK = false
			  out.Drift = append(out.Drift, fmt.Sprintf("deposit[%d] empty key", i))
			   continue
		   }
		   if prev, ok := seenHash[key]; ok && prev != d.UserID {
			  out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("deposit %s claimed by %s and %s", key, prev, d.UserID))
		  }
		  seenHash[key] = d.UserID
		   if _, err := money.Parse(d.Amount); err != nil {
			  out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("deposit %s amount: %v", key, err))
		   }
	  }
	   seenIdem := map[string]string{}
	  for i, w := range wds {
		  if strings.TrimSpace(w.Idem) != "" {
			   if prev, ok := seenIdem[w.Idem]; ok && prev != w.ID {
				   out.OK = false
				   out.Drift = append(out.Drift, fmt.Sprintf("withdraw idem %s %s vs %s", w.Idem, prev, w.ID))
			   }
			  seenIdem[w.Idem] = w.ID
		   }
		  if _, err := money.Parse(w.Amount); err != nil {
			   out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("withdraw[%d] amount: %v", i, err))
		  }
	   }
	  for i, c := range inv {
		   side := strings.ToLower(strings.TrimSpace(c.Side))
		   if side != "hot" && side != "cold" {
			   out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("custody[%d] side %s", i, c.Side))
			  continue
		   }
		   av, err := money.ParseBook(orZero(c.Available))
		  if err != nil {
			   out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("custody %s/%s avail: %v", c.Asset, side, err))
			   continue
		   }
		  fr, err := money.ParseBook(orZero(c.Frozen))
		  if err != nil {
			  out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("custody %s/%s frozen: %v", c.Asset, side, err))
			   continue
		  }
		  if _, err := av.Add(fr); err != nil {
			  out.OK = false
			   out.Drift = append(out.Drift, fmt.Sprintf("custody %s/%s sum: %v", c.Asset, side, err))
		   }
	   }
	   return out
  }

  func orZero(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	  }
	  return s
   }

  func (s *SQLDB) LoadDeposits(ctx context.Context, limit int) ([]DepositSnap, error) {
	  if s == nil || s.DB == nil {
		  return nil, ErrTxClosed
	   }
	   if limit <= 0 {
		  limit = 512
	  }
	   rows, err := s.Conn(ctx).QueryContext(ctx,
		  `SELECT chain, tx_hash, output_index, user_id, asset, amount, fence_epoch, fence_token FROM aa_deposits ORDER BY created_at LIMIT ?`, limit)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]DepositSnap, 0, 16)
	   for rows.Next() {
		  var d DepositSnap
		  if err := rows.Scan(&d.Chain, &d.Hash, &d.OutputIndex, &d.UserID, &d.Asset, &d.Amount, &d.Epoch, &d.Token); err != nil {
			  return nil, err
		  }
		  out = append(out, d)
	  }
	  return out, rows.Err()
   }

   func (s *SQLDB) LoadWithdraws(ctx context.Context, limit int) ([]WithdrawSnap, error) {
	  if s == nil || s.DB == nil {
		  return nil, ErrTxClosed
	  }
	   if limit <= 0 {
		  limit = 512
	  }
	   rows, err := s.Conn(ctx).QueryContext(ctx,
		   `SELECT id, user_id, asset, amount, state, idem, fence_epoch, fence_token FROM aa_withdraws ORDER BY created_at LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]WithdrawSnap, 0, 16)
	   for rows.Next() {
		   var w WithdrawSnap
		   if err := rows.Scan(&w.ID, &w.UserID, &w.Asset, &w.Amount, &w.State, &w.Idem, &w.Epoch, &w.Token); err != nil {
			  return nil, err
		  }
		  out = append(out, w)
	   }
	   return out, rows.Err()
  }

  func (s *SQLDB) LoadCustody(ctx context.Context) ([]CustodySnap, error) {
	  if s == nil || s.DB == nil {
		  return nil, ErrTxClosed
	  }
	  rows, err := s.Conn(ctx).QueryContext(ctx,
		   `SELECT asset, side, available, frozen, fence_epoch, fence_token FROM aa_custody_inv ORDER BY asset, side`)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]CustodySnap, 0, 8)
	   for rows.Next() {
		  var c CustodySnap
		  if err := rows.Scan(&c.Asset, &c.Side, &c.Available, &c.Frozen, &c.Epoch, &c.Token); err != nil {
			   return nil, err
		   }
		   out = append(out, c)
	  }
	   return out, rows.Err()
  }

   func (s *SQLDB) Recon(ctx context.Context) (ReconReport, error) {
	  deps, err := s.LoadDeposits(ctx, 1024)
	  if err != nil {
		  return ReconReport{}, err
	   }
	   wds, err := s.LoadWithdraws(ctx, 1024)
	   if err != nil {
		  return ReconReport{}, err
	  }
	   inv, err := s.LoadCustody(ctx)
	   if err != nil {
		   return ReconReport{}, err
	   }
	  rep := ReconBooks(deps, wds, inv)
	   if err := s.reconFences(ctx, deps, wds, inv); err != nil {
		   rep.OK = false
		   rep.Drift = append(rep.Drift, err.Error())
	   }
	   return rep, nil
   }

   func (s *SQLDB) reconFences(ctx context.Context, deps []DepositSnap, wds []WithdrawSnap, inv []CustodySnap) error {
	  for _, d := range deps {
		   id := fmt.Sprintf("%s|%s|%d", strings.ToUpper(strings.TrimSpace(d.Chain)), strings.TrimSpace(d.Hash), d.OutputIndex)
		   live, err := s.LoadFence(ctx, id)
		   if err != nil {
			  return fmt.Errorf("fence deposit %s: %w", id, err)
		  }
		   if err := ReconcileFence(id, Fence{Epoch: d.Epoch, Token: d.Token}, live); err != nil {
			  return err
		   }
	  }
	  for _, w := range wds {
		  id := "wd|" + w.ID
		   live, err := s.LoadFence(ctx, id)
		   if err != nil {
			   return fmt.Errorf("fence withdraw %s: %w", id, err)
		   }
		   if err := ReconcileFence(id, Fence{Epoch: w.Epoch, Token: w.Token}, live); err != nil {
			  return err
		  }
	   }
	   for _, c := range inv {
		   id := "inv|" + strings.ToUpper(strings.TrimSpace(c.Asset)) + "|" + strings.ToLower(strings.TrimSpace(c.Side))
		  live, err := s.LoadFence(ctx, id)
		  if err != nil {
			  // 库存行可能尚未单独建围栏；缺行不当漂移，避免误伤只写 aa_custody_inv 的路径。
			  if errorsIsNotFound(err) {
				  continue
			  }
			  return err
		  }
		  if c.Epoch == 0 {
			   continue
		  }
		  if err := ReconcileFence(id, Fence{Epoch: c.Epoch, Token: c.Token}, live); err != nil {
			  return err
		   }
	  }
	   return nil
   }

   func errorsIsNotFound(err error) bool {
	  return err != nil && (err == ErrNotFound || strings.Contains(err.Error(), "not found"))
  }
