package sqlrepo

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// WDFeeBump 出金手续费抬价持久化行。
type WDFeeBump struct {
	TransferID string
	Chain   string
	OldHash string
	NewHash string
	OldFee  string
	NewFee string
	Replaced bool
	BumpedAt time.Time
	Memo string
	Tag            string
	TagName        string
	DestinationTag string
}

// WDBroadcast 出金广播确认跟踪持久化行。
type WDBroadcast struct {
	TransferID string
	Chain      string
	TxHash string
	Asset  string
	Amount string
	Status        string
	Confirmations int
	Need          int
	Height int64
	BlockHash string
	Err       string
	UpdatedAt time.Time
	SagaPhase string
	OldTxHash string
	ResubmitN int
	SagaNote  string
}

// WDTrackStore 抬价与广播确认 SQL 面。

type WDTrackStore struct{ db *persist.SQLDB }

  func NewWDTrack(db *persist.SQLDB) *WDTrackStore { return &WDTrackStore{db: db} }

// PutFeeBump upsert 抬价记录。
  func (r *WDTrackStore) PutFeeBump(ctx context.Context, row WDFeeBump) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   row.TransferID = strings.TrimSpace(row.TransferID)
	  if row.TransferID == "" {
		  return ErrNotFound
	   }
	   if row.BumpedAt.IsZero() {
		  row.BumpedAt = time.Now().UTC()
	  }
	   rep := 0
	   if row.Replaced {
		  rep = 1
	   }
	   row.Memo = strings.TrimSpace(row.Memo)
	   row.Tag = strings.TrimSpace(row.Tag)
	   row.TagName = strings.TrimSpace(row.TagName)
	   row.DestinationTag = strings.TrimSpace(row.DestinationTag)
	  stmt := `INSERT INTO aa_wd_fee_bumps (transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at, memo, tag, tag_name, destination_tag)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE chain=VALUES(chain), old_hash=VALUES(old_hash), new_hash=VALUES(new_hash),
 old_fee=VALUES(old_fee), new_fee=VALUES(new_fee), replaced=VALUES(replaced), bumped_at=VALUES(bumped_at),
 memo=VALUES(memo), tag=VALUES(tag), tag_name=VALUES(tag_name), destination_tag=VALUES(destination_tag)`
	  if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_wd_fee_bumps (transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at, memo, tag, tag_name, destination_tag)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (transfer_id) DO UPDATE SET chain=EXCLUDED.chain, old_hash=EXCLUDED.old_hash, new_hash=EXCLUDED.new_hash,
 old_fee=EXCLUDED.old_fee, new_fee=EXCLUDED.new_fee, replaced=EXCLUDED.replaced, bumped_at=EXCLUDED.bumped_at,
 memo=EXCLUDED.memo, tag=EXCLUDED.tag, tag_name=EXCLUDED.tag_name, destination_tag=EXCLUDED.destination_tag`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  row.TransferID, strings.ToUpper(row.Chain), row.OldHash, row.NewHash, row.OldFee, row.NewFee, rep, row.BumpedAt,
		   row.Memo,
		   row.Tag, row.TagName, row.DestinationTag)
	  if err != nil {
		  // 兼容：旧库无 memo 列时回退仅抬价字段。
		   stmt2 := `INSERT INTO aa_wd_fee_bumps (transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE chain=VALUES(chain), old_hash=VALUES(old_hash), new_hash=VALUES(new_hash),
 old_fee=VALUES(old_fee), new_fee=VALUES(new_fee), replaced=VALUES(replaced), bumped_at=VALUES(bumped_at)`
		  if r.db.Dialect == persist.Postgres {
			  stmt2 = `INSERT INTO aa_wd_fee_bumps (transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (transfer_id) DO UPDATE SET chain=EXCLUDED.chain, old_hash=EXCLUDED.old_hash, new_hash=EXCLUDED.new_hash,
 old_fee=EXCLUDED.old_fee, new_fee=EXCLUDED.new_fee, replaced=EXCLUDED.replaced, bumped_at=EXCLUDED.bumped_at`
		  }
		  _, err2 := r.db.Conn(ctx).ExecContext(ctx, stmt2,
			   row.TransferID, strings.ToUpper(row.Chain), row.OldHash, row.NewHash, row.OldFee, row.NewFee, rep, row.BumpedAt)
		  if err2 == nil {
			   return nil
		   }
		   return err
	  }
	  return nil
   }

// ListFeeBumps 启动 Hydrate 全量抬价。
   func (r *WDTrackStore) ListFeeBumps(ctx context.Context) ([]WDFeeBump, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows,
		  err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at, memo, tag, tag_name, destination_tag FROM aa_wd_fee_bumps`)
	  if err != nil {
		  // 兼容：旧库无 memo 列。
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT transfer_id, chain, old_hash, new_hash, old_fee, new_fee, replaced, bumped_at FROM aa_wd_fee_bumps`)
		  if err != nil {
			   return nil, err
		   }
		   defer rows.Close()
		   out := make([]WDFeeBump, 0)
		   for rows.Next() {
			  var row WDFeeBump
			   var rep int
			   if err := rows.Scan(&row.TransferID, &row.Chain, &row.OldHash, &row.NewHash, &row.OldFee, &row.NewFee, &rep, &row.BumpedAt); err != nil {
				  return nil, err
			   }
			   row.Replaced = rep != 0
			  out = append(out, row)
		   }
		   return out, rows.Err()
	   }
	  defer rows.Close()
	   out := make([]WDFeeBump, 0)
	  for rows.Next() {
		   var row WDFeeBump
		   var rep int
		  if err := rows.Scan(&row.TransferID,
			   &row.Chain, &row.OldHash, &row.NewHash, &row.OldFee, &row.NewFee, &rep, &row.BumpedAt,
			   &row.Memo, &row.Tag, &row.TagName, &row.DestinationTag); err != nil {
			   return nil, err
		   }
		   row.Replaced = rep != 0
		  out = append(out, row)
	   }
	   return out, rows.Err()
  }

// PutBroadcast upsert 广播确认跟踪。

  func (r *WDTrackStore) PutBroadcast(ctx context.Context, row WDBroadcast) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   row.TransferID = strings.TrimSpace(row.TransferID)
	   if row.TransferID == "" {
		   return ErrNotFound
	  }
	  if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	   }
	   stmt := `INSERT INTO aa_wd_broadcasts (transfer_id, chain, tx_hash, asset, amount, status, confirmations, need_conf, height, block_hash, err, updated_at, saga_phase, old_tx_hash, resubmit_n, saga_note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE chain=VALUES(chain), tx_hash=VALUES(tx_hash), asset=VALUES(asset), amount=VALUES(amount),
 status=VALUES(status), confirmations=VALUES(confirmations), need_conf=VALUES(need_conf), height=VALUES(height),
 block_hash=VALUES(block_hash), err=VALUES(err), updated_at=VALUES(updated_at),
 saga_phase=VALUES(saga_phase), old_tx_hash=VALUES(old_tx_hash), resubmit_n=VALUES(resubmit_n), saga_note=VALUES(saga_note)`
	   if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_wd_broadcasts (transfer_id, chain, tx_hash, asset, amount, status, confirmations, need_conf, height, block_hash, err, updated_at, saga_phase, old_tx_hash, resubmit_n, saga_note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (transfer_id) DO UPDATE SET chain=EXCLUDED.chain, tx_hash=EXCLUDED.tx_hash, asset=EXCLUDED.asset, amount=EXCLUDED.amount,
 status=EXCLUDED.status, confirmations=EXCLUDED.confirmations, need_conf=EXCLUDED.need_conf, height=EXCLUDED.height,
 block_hash=EXCLUDED.block_hash, err=EXCLUDED.err, updated_at=EXCLUDED.updated_at,
 saga_phase=EXCLUDED.saga_phase, old_tx_hash=EXCLUDED.old_tx_hash, resubmit_n=EXCLUDED.resubmit_n, saga_note=EXCLUDED.saga_note`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		   row.TransferID, strings.ToUpper(row.Chain), row.TxHash, strings.ToUpper(row.Asset), row.Amount,
		  row.Status, row.Confirmations, row.Need, row.Height, row.BlockHash, row.Err, row.UpdatedAt,
		  row.SagaPhase, row.OldTxHash, row.ResubmitN, row.SagaNote)
	  return err
   }

// ListBroadcasts 启动 Hydrate 全量广播跟踪。

  func (r *WDTrackStore) ListBroadcasts(ctx context.Context) ([]WDBroadcast, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT transfer_id, chain, tx_hash, asset, amount, status, confirmations, need_conf, height, block_hash, err, updated_at, 
		 COALESCE(saga_phase,''), COALESCE(old_tx_hash,''), COALESCE(resubmit_n,0), COALESCE(saga_note,'') FROM aa_wd_broadcasts`)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]WDBroadcast, 0)
	  for rows.Next() {
		   var row WDBroadcast
		   var errMsg sql.NullString
		  if err := rows.Scan(&row.TransferID, &row.Chain, &row.TxHash, &row.Asset, &row.Amount, &row.Status,
			   &row.Confirmations, &row.Need, &row.Height, &row.BlockHash, &errMsg, &row.UpdatedAt,
			  &row.SagaPhase, &row.OldTxHash, &row.ResubmitN, &row.SagaNote); err != nil {
			   return nil, err
		   }
		  row.Err = errMsg.String
		   out = append(out, row)
	   }
	  return out, rows.Err()
   }

// CountBroadcasts SQL 广播行数（ops hydrate ）。

  func (r *WDTrackStore) CountBroadcasts(ctx context.Context) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	   var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_wd_broadcasts`).Scan(&n)
	  return n, err
   }

// CountFeeBumps SQL 抬价行数（ops hydrate ）。

  func (r *WDTrackStore) CountFeeBumps(ctx context.Context) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	  }
	   var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_wd_fee_bumps`).Scan(&n)
	   return n, err
  }

// CountBySagaPhase 按 saga_phase 计数（ops/metrics）。

  func (r *WDTrackStore) CountBySagaPhase(ctx context.Context) (map[string]int, error) {
	  out := map[string]int{}
	  if !sqlOn(r.db) {
		   return out, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT COALESCE(NULLIF(saga_phase,''),'none'), COUNT(*) FROM aa_wd_broadcasts GROUP BY 1`)
	   if err != nil {
		   return out, err
	   }
	   defer rows.Close()
	  for rows.Next() {
		   var phase string
		   var n int
		  if err := rows.Scan(&phase, &n); err != nil {
			   return out, err
		  }
		   out[phase] = n
	   }
	  return out, rows.Err()
  }
