// @FileName: bookstore.go
// @Summary: 出金出站 Saga 簿本进程表夹具：共享 rows SQL 重启。
// @Author: Draven
// @Created: 2020.10.27

package withdraw

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// BookTable 进程内 aa_wd_outbound 行语义夹具。自然键 transfer_id。

type BookTable struct {
	mu     sync.Mutex
	rows   map[string]sqlrepo.WDOutboundRun
	byIdem map[string]string
	failList bool
	failWrite  bool
	writeCount int
}

  func NewBookTable() *BookTable {
	   return &BookTable{rows: map[string]sqlrepo.WDOutboundRun{}, byIdem: map[string]string{}}
   }

   func OpenBookTable(rows map[string]sqlrepo.WDOutboundRun) *BookTable {
	  if rows == nil {
		   rows = map[string]sqlrepo.WDOutboundRun{}
	  }
	   by := map[string]string{}
	  for k, v := range rows {
		  if v.Idem != "" {
			  by[v.Idem] = k
		  }
	  }
	  return &BookTable{rows: rows,
		   byIdem: by}
  }

  func (t *BookTable) Rows() map[string]sqlrepo.WDOutboundRun {
	   if t == nil {
		  return nil
	  }
	  return t.rows
  }

   func (t *BookTable) FailNextList()  { t.failList = true }
   func (t *BookTable) FailNextWrite() { t.failWrite = true }

  func (t *BookTable) PersistRun(_ context.Context, row sqlrepo.WDOutboundRun) error {
	   if t == nil || t.rows == nil {
		  return sqlrepo.ErrUnavailable
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   if t.failWrite {
		  t.failWrite = false
		   return sqlrepo.ErrUnavailable
	   }
	  row.TransferID = strings.TrimSpace(row.TransferID)
	  row.Idem = strings.TrimSpace(row.Idem)
	   if row.TransferID == "" {
		  return sqlrepo.ErrConflict
	  }
	   if row.Idem == "" {
		   row.Idem = "xfer:" + row.TransferID
	   }
	   if tid, ok := t.byIdem[row.Idem]; ok && tid != row.TransferID {
		   return sqlrepo.ErrTaken
	  }
	  if cur, ok := t.rows[row.TransferID]; ok && cur.FailSealed {
		  if row.Step == string(StepConfirmed) || row.Step == string(StepSubmitted) {
			  return sqlrepo.ErrConflict
		   }
		   row.FailSealed = true
		  if row.Step == "" {
			   row.Step = string(StepFailed)
		  }
	  }
	   if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	   }
	  if row.CreatedAt.IsZero() {
		   if cur, ok := t.rows[row.TransferID]; ok && !cur.CreatedAt.IsZero() {
			  row.CreatedAt = cur.CreatedAt
		   } else {
			  row.CreatedAt = row.UpdatedAt
		  }
	   }
	   t.rows[row.TransferID] = row
	   t.byIdem[row.Idem] = row.TransferID
	   t.writeCount++
	   return nil
  }

   func (t *BookTable) Get(_ context.Context, transferID string) (sqlrepo.WDOutboundRun, error) {
	  if t == nil {
		   return sqlrepo.WDOutboundRun{}, sqlrepo.ErrUnavailable
	  }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	   row, ok := t.rows[strings.TrimSpace(transferID)]
	  if !ok {
		  return sqlrepo.WDOutboundRun{},
			  sqlrepo.ErrNotFound
	  }
	  return row, nil
  }

  func (t *BookTable) GetByIdem(_ context.Context, idem string) (sqlrepo.WDOutboundRun, error) {
	  if t == nil {
		  return sqlrepo.WDOutboundRun{}, sqlrepo.ErrUnavailable
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   tid, ok := t.byIdem[strings.TrimSpace(idem)]
	  if !ok {
		   return sqlrepo.WDOutboundRun{}, sqlrepo.ErrNotFound
	  }
	   return t.rows[tid], nil
  }

  func (t *BookTable) ListAll(_ context.Context, limit int) ([]sqlrepo.WDOutboundRun, error) {
	  if t == nil {
		   return nil, sqlrepo.ErrUnavailable
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   if t.failList {
		   t.failList = false
		   return nil, sqlrepo.ErrUnavailable
	   }
	  out := make([]sqlrepo.WDOutboundRun,
		   0, len(t.rows))
	  for _, row := range t.rows {
		  out = append(out, row)
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

// Claim 同 transfer 幂等回放；同 idem 异 transfer 冲突。已 debit_once 不得再记一笔扣款。

   func (t *BookTable) Claim(_ context.Context, transferID, idem, userID, asset, amount, chain string) (sqlrepo.WDOutboundRun, error) {
	  if t == nil || t.rows == nil {
		  return sqlrepo.WDOutboundRun{}, sqlrepo.ErrUnavailable
	   }
	  transferID = strings.TrimSpace(transferID)
	   idem = strings.TrimSpace(idem)
	   userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   if transferID == "" || idem == "" || userID == "" {
		  return sqlrepo.WDOutboundRun{}, sqlrepo.ErrConflict
	  }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	   if tid, ok := t.byIdem[idem]; ok && tid != transferID {
		   return sqlrepo.WDOutboundRun{}, sqlrepo.ErrTaken
	   }
	  if cur, ok := t.rows[transferID]; ok {
		  if cur.UserID != userID || cur.Asset != asset {
			  return sqlrepo.WDOutboundRun{}, sqlrepo.ErrTaken
		   }
		   return cur, nil
	  }
	   now := time.Now().UTC()
	   row := sqlrepo.WDOutboundRun{
		   TransferID: transferID, UserID: userID, Asset: asset, Amount: amount,
		   Chain: chain,
		   Idem:  idem, Step: string(StepRequested), CreatedAt: now, UpdatedAt: now,
	  }
	   t.rows[transferID] = row
	  t.byIdem[idem] = transferID
	  return row, nil
  }
