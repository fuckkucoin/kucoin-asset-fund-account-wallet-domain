package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ReconDiffRow 入金差事件一行。
type ReconDiffRow struct {
	TicketID string
	Event    string
	Kind     string
	Chain    string
	TxHash      string
	OutputIndex int
	Asset          string
	UserID         string
	ObservedAmount string
	CreditedAmount string
	ObserveState string
	CreditState  string
	WindowStart  string
	WindowEnd    string
	SecondCredit bool
	Blocked       bool
	DepositTicket string
	Phase         string
	ReverseEntry string
	AvailBefore string
	AvailAfter string
	Reason    string
	CreatedAt time.Time
}

   func bit(v bool) int {
	   if v {
		   return 1
	   }
	   return 0
  }

// PutReconDepositDiff 按工单号覆盖入金差字段。
   func PutReconDepositDiff(ctx context.Context, db *persist.SQLDB, row ReconDiffRow) error {
	   if !sqlOn(db) {
		  return ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	  qMy := `INSERT INTO aa_recon_deposit_diff
(ticket_id, event_name, kind, chain, tx_hash, output_index, asset, user_id, observed_amount, credited_amount, observe_state, credit_state, window_start, window_end, second_credit, blocked, deposit_ticket_id, phase, reverse_entry, avail_before, avail_after, reason, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE event_name=VALUES(event_name), kind=VALUES(kind), chain=VALUES(chain), tx_hash=VALUES(tx_hash), output_index=VALUES(output_index), asset=VALUES(asset), user_id=VALUES(user_id), observed_amount=VALUES(observed_amount), credited_amount=VALUES(credited_amount), observe_state=VALUES(observe_state), credit_state=VALUES(credit_state), window_start=VALUES(window_start), window_end=VALUES(window_end), second_credit=VALUES(second_credit), blocked=VALUES(blocked), deposit_ticket_id=VALUES(deposit_ticket_id), phase=VALUES(phase), reverse_entry=VALUES(reverse_entry), avail_before=VALUES(avail_before), avail_after=VALUES(avail_after), reason=VALUES(reason)`
	  qPg := `INSERT INTO aa_recon_deposit_diff
(ticket_id, event_name, kind, chain, tx_hash, output_index, asset, user_id, observed_amount, credited_amount, observe_state, credit_state, window_start, window_end, second_credit, blocked, deposit_ticket_id, phase, reverse_entry, avail_before, avail_after, reason, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (ticket_id) DO UPDATE SET event_name=EXCLUDED.event_name, kind=EXCLUDED.kind, chain=EXCLUDED.chain, tx_hash=EXCLUDED.tx_hash, output_index=EXCLUDED.output_index, asset=EXCLUDED.asset, user_id=EXCLUDED.user_id, observed_amount=EXCLUDED.observed_amount, credited_amount=EXCLUDED.credited_amount, observe_state=EXCLUDED.observe_state, credit_state=EXCLUDED.credit_state, window_start=EXCLUDED.window_start, window_end=EXCLUDED.window_end, second_credit=EXCLUDED.second_credit, blocked=EXCLUDED.blocked, deposit_ticket_id=EXCLUDED.deposit_ticket_id, phase=EXCLUDED.phase, reverse_entry=EXCLUDED.reverse_entry, avail_before=EXCLUDED.avail_before, avail_after=EXCLUDED.avail_after, reason=EXCLUDED.reason`
	  q := qMy
	  if db.Dialect == persist.Postgres {
		  q = qPg
	   }
	   _, err := db.Conn(ctx).ExecContext(ctx, q,
		  row.TicketID, row.Event, row.Kind, row.Chain, row.TxHash, row.OutputIndex, row.Asset, row.UserID,
		  row.ObservedAmount, row.CreditedAmount, row.ObserveState, row.CreditState, row.WindowStart, row.WindowEnd,
		  bit(row.SecondCredit), bit(row.Blocked), row.DepositTicket, row.Phase, row.ReverseEntry,
		   row.AvailBefore, row.AvailAfter, row.Reason, row.CreatedAt)
	  return err
   }

// GetReconDepositDiff 读取工单上的入金差字段。

  func GetReconDepositDiff(ctx context.Context, db *persist.SQLDB, ticketID string) (ReconDiffRow, error) {
	   if !sqlOn(db) {
		   return ReconDiffRow{}, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  var row ReconDiffRow
	  var second, blocked int
	  err := db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT ticket_id, event_name, kind, chain, tx_hash, output_index, asset, user_id, observed_amount, credited_amount, observe_state, credit_state, window_start, window_end, second_credit, blocked, deposit_ticket_id, phase, reverse_entry, avail_before, avail_after, reason, created_at FROM aa_recon_deposit_diff WHERE ticket_id=?`,
		   ticketID,
	   ).Scan(&row.TicketID, &row.Event, &row.Kind, &row.Chain, &row.TxHash, &row.OutputIndex, &row.Asset, &row.UserID,
		  &row.ObservedAmount,
		  &row.CreditedAmount, &row.ObserveState, &row.CreditState, &row.WindowStart, &row.WindowEnd,
		   &second, &blocked, &row.DepositTicket, &row.Phase, &row.ReverseEntry, &row.AvailBefore, &row.AvailAfter, &row.Reason, &row.CreatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return ReconDiffRow{}, ErrNotFound
	  }
	   row.SecondCredit = second != 0
	  row.Blocked = blocked != 0
	  return row, err
   }
