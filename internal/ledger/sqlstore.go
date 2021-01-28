// MySQL 账本和单据持久化。
// 作者: Knox
// 日期: 2021.01.06
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type SQLStore struct {
	db      *sql.DB
	extra   []*sql.DB
	dialect persist.Dialect
	seal    Sealer
}

  func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

  func NewSQLStoreDialect(db *sql.DB, d persist.Dialect) *SQLStore {
	  return &SQLStore{db: db, dialect: d}
   }

type txKey struct{}

// conn 先用上下文里的事务；否则按用户号选片。嵌套调用复用同一把 tx。
  func (s *SQLStore) conn(ctx context.Context) queryer {
	  var inner persist.Queryer
	   if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		  inner = tx
	   } else {
		   inner = s.pickDB(ctx)
	   }
	  if s != nil && s.dialect == persist.Postgres {
		  return persist.WrapQueryer(inner, persist.Postgres)
	  }
	   return inner
  }

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context,
		query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// InTx 为资金变更开启可重复读事务。上下文里已有事务时直接复用。
// 冻结、解冻、流水、出箱必须落在同一次提交。严禁拆成两次提交。
// 余额按读到的快照做条件更新。死锁、快照冲突或快照已过期时，整笔回滚后重开，最多三次。
// 锁等待超时不重试。本事务把锁等待上限收到 2 秒；MySQL 用完后把会话超时恢复为 50 秒，避免连接回到池里拖住其它会话。
// 严禁在 fn 内发起链上请求或其它外部调用。外部调用必须放在事务提交之后。
   func (s *SQLStore) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	   if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := persist.SleepTxRetry(ctx, attempt); err != nil {
				return err
			}
		}
		last = s.inTxOnce(ctx, fn)
		if last == nil || !retryMoney(last) {
			return last
		}
	}
	return last
}

func (s *SQLStore) inTxOnce(ctx context.Context, fn func(ctx context.Context) error) error {
	db := s.pickDB(ctx)
	if s.dialect == persist.Postgres {
		return s.inTxOn(ctx, db, fn, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '2s'`)
			return err
		})
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SET SESSION innodb_lock_wait_timeout = 2`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SET SESSION innodb_lock_wait_timeout = 50`)
	tx, err := conn.BeginTx(ctx, persist.MoneyTxOptions())
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, txKey{}, tx)
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *SQLStore) inTxOn(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error, prep func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, persist.MoneyTxOptions())
	if err != nil {
		return err
	}
	if prep != nil {
		if err := prep(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	ctx = context.WithValue(ctx, txKey{}, tx)
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// GetAccount 读取用户某科目某资产的可用与冻结。
// 行不存在时插入零余额再读回。并发插入撞上唯一键时改为读取已有行，不得再插一条。
// 读到的余额记入快照，供随后的条件更新核对。本查询不加行锁。
// 并发写入靠条件更新失败后由外层整笔重开，不在这里排队。
   func (s *SQLStore) GetAccount(ctx context.Context, userID string, typ AccountType, asset string) (*Account, error) {
	   q := s.conn(ctx)
	   qstr := `SELECT id, user_id, account_type, asset, available, frozen FROM ledger_accounts WHERE user_id=? AND account_type=? AND asset=?`
	   row := q.QueryRowContext(ctx, qstr, userID, typ, asset)
	  a := &Account{}
	   var avail, frozen string
	   err := row.Scan(&a.ID,
		   &a.UserID, &a.AccountType, &a.Asset, &avail, &frozen)
	   if errors.Is(err, sql.ErrNoRows) {
		  _, ierr := q.ExecContext(ctx, `INSERT INTO ledger_accounts (user_id, account_type, asset, available, frozen) VALUES (?,?,?,0,0)`, userID, typ, asset)
		  if ierr != nil && !dupKey(ierr) {
			  return nil, ierr
		   }
		   row = q.QueryRowContext(ctx, qstr, userID, typ, asset)
		   err = row.Scan(&a.ID, &a.UserID, &a.AccountType, &a.Asset, &avail, &frozen)
		   if err != nil {
			   return nil, err
		   }
	  }
	   if err != nil {
		  return nil, err
	   }
	  a.Available, err = ParseBook(avail)
	   if err != nil {
		  return nil, err
	  }
	   a.Frozen, err = ParseBook(frozen)
	  if err != nil {
		  return nil, err
	  }
	  a.stampBook()
	  return a, nil
  }

// UpsertAccount 把可用、冻结写回科目行。
// 可用或冻结为负时拒绝。库表另有非负约束，应用层不得依赖约束失败来代替校验。
// 事务内必须携带 GetAccount 记下的快照。没有快照时拒绝写入。严禁按内存里的绝对值覆盖科目行。
// 事务外且没有快照时按绝对值更新，只用于建账和夹具。资金冻结、入账、回滚不得走这条。
// 快照与当前行不一致，或更新行数不是 1，返回 ErrStaleBook。调用方必须整笔重开，不得改数再写一次。
  func (s *SQLStore) UpsertAccount(ctx context.Context, a *Account) error {
	   if a == nil {
		   return ErrStaleBook
	   }
	   if a.Available.IsNegative() || a.Frozen.IsNegative() {
		   return ErrNegative
	   }
	   if !a.snapOK {
		   if _, inTx := ctx.Value(txKey{}).(*sql.Tx); inTx {
			   return ErrStaleBook
		   }
		   _, err := s.conn(ctx).ExecContext(ctx, `UPDATE ledger_accounts SET available=?, frozen=? WHERE user_id=? AND account_type=? AND asset=?`,
			   a.Available.StringFixed(18), a.Frozen.StringFixed(18), a.UserID, a.AccountType, a.Asset)
		   return err
	   }
	   if a.Available.Equal(a.snapAvail) && a.Frozen.Equal(a.snapFrozen) {
		   return nil
	   }
	   res, err := s.conn(ctx).ExecContext(ctx, `UPDATE ledger_accounts SET available=?, frozen=? WHERE user_id=? AND account_type=? AND asset=? AND available=? AND frozen=?`,
		   a.Available.StringFixed(18), a.Frozen.StringFixed(18), a.UserID, a.AccountType, a.Asset, a.snapAvail.StringFixed(18), a.snapFrozen.StringFixed(18))
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	   if n != 1 {
		   return ErrStaleBook
	   }
	   a.stampBook()
	   return nil
  }

const transferCols = `id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, output_index, state, risk_flags, fail_reason, created_at, updated_at, fence_epoch, fence_token, observed_amount, observed_asset, credited_height, credited_block, kyt_decision`

// InsertTransfer 写入划转单。幂等键有唯一约束，撞键返回 ErrIdempotency，由上层按原单接着，严禁再冻一次。
// 备注、标签、地址提示在写入前封存。调用方结构体保持明文，不得把封存结果回写到入参。
// 地址密文沿用入参里的 to_address_enc，本函数不再改其内容。
   func (s *SQLStore) InsertTransfer(ctx context.Context, t *Transfer) error {
	   flags, _ := json.Marshal(t.RiskFlags)
	  obsAmt := interface{}(nil)
	   if !t.ObservedAmount.IsZero() {
		  obsAmt = t.ObservedAmount.StringFixed(18)
	  }
	   memo, tag, tagName, err := s.sealHints(t.MemoHint, t.TagHint, t.TagNameHint)
	   if err != nil {
		   return err
	   }
	   toHint, err := s.sealOne(t.ToAddressHint)
	   if err != nil {
		   return err
	   }
	   fromHint, err := s.sealOne(t.FromAddressHint)
	   if err != nil {
		   return err
	   }
	   _, err = s.conn(ctx).ExecContext(ctx, `INSERT INTO transfers (id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, output_index, state, risk_flags, fail_reason, fence_epoch, fence_token, observed_amount, observed_asset, credited_height, credited_block, kyt_decision) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   t.ID,
		  t.IdempotencyKey, t.UserID, t.Direction, t.Asset, t.Amount.StringFixed(18), t.Chain, t.ToAddressEnc, toHint, fromHint, memo, tag, tagName, t.TxHash, t.OutputIndex, t.State, flags, t.FailReason, t.FenceEpoch, t.FenceToken, obsAmt, t.ObservedAsset, t.CreditedHeight, t.CreditedBlock, t.KYTDecision)
	  if err != nil {
		  if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			   return ErrIdempotency
		   }
		   return err
	  }
	  return nil
  }

// scanTransfer 把 transfers 行扫成领域对象，金额走 ParseBook。

  func (s *SQLStore) scanTransfer(row *sql.Row) (*Transfer, error) {
	  t := &Transfer{}
	  var amount string
	  var flags []byte
	  var fail sql.NullString
	  var txh sql.NullString
	   var enc []byte
	   var hint sql.NullString
	   var epoch int64
	  var token sql.NullString
	   var obsAmt, obsAsset, kytDec sql.NullString
	  var height sql.NullInt64
	   var block sql.NullString
	   var fromHint, memoHint, tagHint, tagNameHint sql.NullString
	   err := row.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.OutputIndex, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt, &epoch, &token, &obsAmt, &obsAsset, &height, &block, &kytDec)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	   if t.Amount, err = ParseBook(amount); err != nil {
		   return nil, err
	  }
	  t.ToAddressEnc = enc
	   if hint.Valid {
		  t.ToAddressHint = hint.String
	  }
	   if fromHint.Valid {
		   t.FromAddressHint = fromHint.String
	  }
	   if memoHint.Valid {
		   t.MemoHint = memoHint.String
	  }
	   if tagHint.Valid {
		  t.TagHint = tagHint.String
	  }
	   if tagNameHint.Valid {
		   t.TagNameHint = tagNameHint.String
	   }
	  if txh.Valid {
		   t.TxHash = txh.String
	  }
	  if fail.Valid {
		  t.FailReason = fail.String
	  }
	  t.FenceEpoch = epoch
	   t.FenceToken = token.String
	  if obsAmt.Valid {
		   t.ObservedAmount, err = ParseBook(obsAmt.String)
		   if err != nil {
			   return nil, err
		   }
	   }
	   t.ObservedAsset = obsAsset.String
	  if height.Valid {
		  t.CreditedHeight = height.Int64
	  }
	  t.CreditedBlock = block.String
	  t.KYTDecision = kytDec.String
	  _ = json.Unmarshal(flags, &t.RiskFlags)
	   s.reveal(t)
	   return t, nil
   }

   func (s *SQLStore) GetTransfer(ctx context.Context, id string) (*Transfer, error) {
	  return s.oneTransfer(ctx, `SELECT `+transferCols+` FROM transfers WHERE id=?`, id)
   }

// GetByIdempotency 幂等键命中已有单，Create 据此 resume 而不是再冻一次。
  func (s *SQLStore) GetByIdempotency(ctx context.Context, key string) (*Transfer, error) {
	  return s.oneTransfer(ctx, `SELECT `+transferCols+` FROM transfers WHERE idempotency_key=?`, key)
  }

// UpdateTransfer 推进状态、hash、失败原因；乐观条件由上层 fence 保证。
   func (s *SQLStore) UpdateTransfer(ctx context.Context, t *Transfer) error {
	   flags, _ := json.Marshal(t.RiskFlags)
	   obsAmt := interface{}(nil)
	  if !t.ObservedAmount.IsZero() {
		   obsAmt = t.ObservedAmount.StringFixed(18)
	  }
	  memo, tag, tagName, err := s.sealHints(t.MemoHint, t.TagHint, t.TagNameHint)
	  if err != nil {
		  return err
	  }
	  toHint, err := s.sealOne(t.ToAddressHint)
	  if err != nil {
		  return err
	  }
	  fromHint, err := s.sealOne(t.FromAddressHint)
	  if err != nil {
		  return err
	  }
	  res, err := s.conn(ctx).ExecContext(ctx, `UPDATE transfers SET tx_hash=?, output_index=?, state=?, risk_flags=?, fail_reason=?, to_address_enc=?, to_address_hint=?, from_address_hint=?, memo_hint=?, tag_hint=?, tag_name_hint=?, observed_amount=?, observed_asset=?, credited_height=?, credited_block=?, kyt_decision=? WHERE id=? AND fence_epoch=? AND fence_token=?`,
		  t.TxHash, t.OutputIndex, t.State, flags, t.FailReason, t.ToAddressEnc, toHint, fromHint, memo, tag, tagName, obsAmt, t.ObservedAsset, t.CreditedHeight, t.CreditedBlock, t.KYTDecision, t.ID, t.FenceEpoch, t.FenceToken)
	  if err != nil {
		   return err
	   }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrStaleFence
	  }
	   return nil
   }

// StampFence 在持有用户锁之后为本单递增围栏序号并写入令牌。
// 租约丢失后，过期的执行体令牌对不上，写不进这张单。严禁在未持锁时调用。
   func (s *SQLStore) StampFence(ctx context.Context, id, token string) (int64, error) {
	  _, err := s.conn(ctx).ExecContext(ctx, `UPDATE transfers SET fence_epoch=fence_epoch+1, fence_token=? WHERE id=?`, token, id)
	   if err != nil {
		  return 0, err
	   }
	   var ep int64
	   err = s.conn(ctx).QueryRowContext(ctx, `SELECT fence_epoch FROM transfers WHERE id=?`, id).Scan(&ep)
	   if errors.Is(err, sql.ErrNoRows) {
		  return 0, ErrNotFound
	   }
	  return ep, err
   }

// AppendJournal 写入复式流水。写入前必须通过 ValidateJournal。
// 同一批次按资产借贷必须轧平，一行不得又借又贷。校验失败则整批不写。
// 严禁先插入部分分录再在循环中途返回。中途失败会留下对不上的流水。

   func (s *SQLStore) AppendJournal(ctx context.Context, entries []JournalEntry) error {
	  if err := ValidateJournal(entries); err != nil {
		   return err
	   }
	  q := s.conn(ctx)
	  var b strings.Builder
	   args := make([]any, 0, len(entries)*8)
	   b.WriteString(`INSERT INTO ledger_journal (transfer_id, entry_seq, user_id, account_type, asset, debit, credit, memo) VALUES `)
	   for i, e := range entries {
		  if i > 0 {
			   b.WriteByte(',')
		   }
		   b.WriteString(`(?,?,?,?,?,?,?,?)`)
		  args = append(args, e.TransferID, e.Seq, e.UserID, e.AccountType, e.Asset, e.Debit.StringFixed(18), e.Credit.StringFixed(18), e.Memo)
	   }
	   _, err := q.ExecContext(ctx, b.String(), args...)
	   return err
  }

  func (s *SQLStore) ListJournal(ctx context.Context, transferID string) ([]JournalEntry, error) {
	  rows, err := s.conn(ctx).QueryContext(ctx, `SELECT transfer_id, entry_seq, user_id, account_type, asset, debit, credit, memo FROM ledger_journal WHERE transfer_id=? ORDER BY entry_seq`, transferID)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   var out []JournalEntry
	  for rows.Next() {
		  var e JournalEntry
		  var d, c string
		   if err := rows.Scan(&e.TransferID, &e.Seq, &e.UserID, &e.AccountType, &e.Asset, &d, &c, &e.Memo); err != nil {
			   return nil, err
		   }
		   if e.Debit, err = ParseBook(d); err != nil {
			  return nil, err
		  }
		  if e.Credit, err = ParseBook(c); err != nil {
			  return nil, err
		  }
		   out = append(out, e)
	   }
	  return out, rows.Err()
  }

// SumDebitCredit 按科目汇总借贷，对账用：账面必须等于流水轧差。

   func (s *SQLStore) SumDebitCredit(ctx context.Context, userID string, typ AccountType, asset string) (decimal.Decimal, decimal.Decimal, error) {
	  row := s.conn(ctx).QueryRowContext(ctx, `SELECT COALESCE(SUM(debit),0), COALESCE(SUM(credit),0) FROM ledger_journal WHERE user_id=? AND account_type=? AND asset=?`, userID, typ, asset)
	   var d, c string
	  if err := row.Scan(&d, &c); err != nil {
		  return decimal.Zero, decimal.Zero, err
	  }
	  deb, err := ParseBook(d)
	  if err != nil {
		   return decimal.Zero, decimal.Zero, err
	   }
	  cred, err := ParseBook(c)
	  if err != nil {
		   return decimal.Zero, decimal.Zero, err
	  }
	   return deb, cred, nil
  }

// SumToday 汇总当日已占用额度。失败和已回滚的单据不计入，避免被拒绝的单把日限额占满。

   func (s *SQLStore) SumToday(ctx context.Context, userID, asset string) (decimal.Decimal, error) {
	  row := s.conn(ctx).QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM transfers WHERE user_id=? AND asset=? AND created_at >= UTC_DATE() AND state NOT IN ('failed','rolled_back')`, userID, asset)
	  var v string
	  if err := row.Scan(&v); err != nil {
		   return decimal.Zero, err
	   }
	  return ParseBook(v)
   }

// SumTodayByDirection 分方向日占用。充提与内部划转各自轧自己的池。
  func (s *SQLStore) SumTodayByDirection(ctx context.Context, userID, asset string, dir Direction) (decimal.Decimal, error) {
	  var q string
	   switch directionPool(dir) {
	  case "withdraw":
		  q = `SELECT COALESCE(SUM(amount),0) FROM transfers WHERE user_id=? AND asset=? AND created_at >= UTC_DATE() AND state NOT IN ('failed','rolled_back') AND direction='wallet_withdraw'`
	   case "deposit":
		  q = `SELECT COALESCE(SUM(amount),0) FROM transfers WHERE user_id=? AND asset=? AND created_at >= UTC_DATE() AND state NOT IN ('failed','rolled_back') AND direction='wallet_deposit'`
	  default:
		   q = `SELECT COALESCE(SUM(amount),0) FROM transfers WHERE user_id=? AND asset=? AND created_at >= UTC_DATE() AND state NOT IN ('failed','rolled_back') AND direction NOT IN ('wallet_withdraw','wallet_deposit')`
	  }
	  row := s.conn(ctx).QueryRowContext(ctx, q, userID, asset)
	   var v string
	  if err := row.Scan(&v); err != nil {
		  return decimal.Zero, err
	   }
	  return ParseBook(v)
   }

// CountSince 统计 since 之后、同一方向池、尚未失败的单据数。口径与频率闸一致。
// 统计失败时调用方必须拒绝本笔，不得按零笔放行。
func (s *SQLStore) CountSince(ctx context.Context, userID string, dir Direction, since time.Time) (int, error) {
	var q string
	switch directionPool(dir) {
	case "withdraw":
		q = `SELECT COUNT(1) FROM transfers WHERE user_id=? AND created_at>? AND state NOT IN ('failed','rolled_back') AND direction='wallet_withdraw'`
	case "deposit":
		q = `SELECT COUNT(1) FROM transfers WHERE user_id=? AND created_at>? AND state NOT IN ('failed','rolled_back') AND direction='wallet_deposit'`
	default:
		q = `SELECT COUNT(1) FROM transfers WHERE user_id=? AND created_at>? AND state NOT IN ('failed','rolled_back') AND direction NOT IN ('wallet_withdraw','wallet_deposit')`
	}
	var n int
	err := s.conn(ctx).QueryRowContext(ctx, q, userID, since.UTC()).Scan(&n)
	return n, err
}

  func (s *SQLStore) AppendAudit(ctx context.Context, e AuditEvent) error {
	  detail, _ := json.Marshal(e.Detail)
	  _, err := s.conn(ctx).ExecContext(ctx, `INSERT INTO audit_log (entity_type, entity_id, action, actor, detail) VALUES (?,?,?,?,?)`,
		  e.EntityType, e.EntityID, e.Action, e.Actor, detail)
	   return err
   }

  func (s *SQLStore) ListAudit(ctx context.Context, entityType, entityID string, limit int) ([]AuditEvent, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	   rows, err := s.conn(ctx).QueryContext(ctx, `SELECT entity_type, entity_id, action, actor, detail, created_at FROM audit_log WHERE (?='' OR entity_type=?) AND (?='' OR entity_id=?) ORDER BY id DESC LIMIT ?`,
		   entityType, entityType, entityID, entityID, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []AuditEvent
	  for rows.Next() {
		   var e AuditEvent
		   var detail []byte
		   if err := rows.Scan(&e.EntityType, &e.EntityID, &e.Action, &e.Actor, &detail, &e.CreatedAt); err != nil {
			   return nil, err
		  }
		   _ = json.Unmarshal(detail, &e.Detail)
		  out = append(out, e)
	  }
	  return out, rows.Err()
   }

  func (s *SQLStore) Allowlisted(ctx context.Context, userID, chain, addr string) (bool, error) {
	   chains := allowlistChains(chain)
	  args := []any{userID, addr}
	  var b strings.Builder
	  b.WriteString(`SELECT COUNT(1) FROM address_allowlist WHERE user_id=? AND address=? AND COALESCE(disabled,0)=0 AND chain IN (`)
	   for i, c := range chains {
		  if i > 0 {
			   b.WriteByte(',')
		   }
		  b.WriteByte('?')
		   args = append(args, c)
	  }
	   b.WriteByte(')')
	  var n int
	  err := s.conn(ctx).QueryRowContext(ctx, b.String(), args...).Scan(&n)
	   return n > 0, err
  }

func (s *SQLStore) AllowlistCount(ctx context.Context, userID, chain string) (int, error) {
	chains := allowlistChains(chain)
	args := []any{userID}
	var b strings.Builder
	b.WriteString(`SELECT COUNT(1) FROM address_allowlist WHERE user_id=? AND COALESCE(disabled,0)=0 AND chain IN (`)
	for i, c := range chains {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
		args = append(args, c)
	}
	b.WriteByte(')')
	var n int
	err := s.conn(ctx).QueryRowContext(ctx, b.String(), args...).Scan(&n)
	return n, err
}

   func (s *SQLStore) ListAddressBook(ctx context.Context, userID string) ([]AddressBookEntry, error) {
	   // 列出出金簿；含 memo / 停用 / 审计列。
	   rows, err := s.conn(ctx).QueryContext(ctx, `SELECT user_id, chain, address, label, COALESCE(memo,''), COALESCE(asset,''), COALESCE(disabled,0), created_at, updated_at, COALESCE(actor,'') FROM address_allowlist WHERE user_id=? ORDER BY chain, address`, userID)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]AddressBookEntry, 0)
	  for rows.Next() {
		  var e AddressBookEntry
		  var disabled int
		  var created,
			   updated sql.NullTime
		  if err := rows.Scan(&e.UserID, &e.Chain, &e.Address, &e.Label, &e.Memo, &e.Asset, &disabled, &created, &updated, &e.Actor); err != nil {
			  return nil, err
		   }
		   e.Disabled = disabled != 0
		  e.Whitelist = !e.Disabled
		  if created.Valid {
			   e.CreatedAt = created.Time.UTC()
		   }
		   if updated.Valid {
			   e.UpdatedAt = updated.Time.UTC()
		  }
		  out = append(out, e)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) upsertSQL(mysql, pg string) string {
	  if s != nil && s.dialect == persist.Postgres {
		  return pg
	  }
	   return mysql
  }

  func (s *SQLStore) UpsertAddressBook(ctx context.Context, e AddressBookEntry) error {
	   // upsert 出金簿含 memo/停用/审计；created_at 仅首次写入。
	   disabled := 0
	  if e.Disabled {
		  disabled = 1
	  }
	  created, updated := e.CreatedAt, e.UpdatedAt
	  if created.IsZero() {
		  created = time.Now().UTC()
	  }
	   if updated.IsZero() {
		  updated = created
	  }
	   q := s.upsertSQL(
		  `INSERT INTO address_allowlist (user_id, chain, address, label, memo, asset, disabled, created_at, updated_at, actor) VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE label=VALUES(label), memo=VALUES(memo), asset=VALUES(asset), disabled=VALUES(disabled), updated_at=VALUES(updated_at), actor=VALUES(actor)`,
		   `INSERT INTO address_allowlist (user_id, chain, address, label, memo, asset, disabled, created_at, updated_at, actor) VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (user_id, chain, address) DO UPDATE SET label=EXCLUDED.label, memo=EXCLUDED.memo, asset=EXCLUDED.asset, disabled=EXCLUDED.disabled, updated_at=EXCLUDED.updated_at, actor=EXCLUDED.actor`,
	  )
	  _, err := s.conn(ctx).ExecContext(ctx, q, e.UserID, e.Chain, e.Address, e.Label, e.Memo, e.Asset, disabled, created, updated, e.Actor)
	   return err
   }

  func (s *SQLStore) DeleteAddressBook(ctx context.Context, userID, chain, addr string) error {
	  res, err := s.conn(ctx).ExecContext(ctx, `DELETE FROM address_allowlist WHERE user_id=? AND chain=? AND address=?`, userID, chain, addr)
	   if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrNotFound
	   }
	  return nil
  }

// ListAccounts 扫描 ledger_accounts。三方对账用它汇总用户负债，禁止只扫单个用户漏掉对手方。
   func (s *SQLStore) ListAccounts(ctx context.Context, limit int) ([]Account, error) {
	  if limit <= 0 {
		  limit = 5000
	  }
	   rows, err := s.conn(ctx).QueryContext(ctx, `SELECT id, user_id, account_type, asset, available, frozen FROM ledger_accounts LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanAccounts(rows)
   }

   func scanAccounts(rows *sql.Rows) ([]Account, error) {
	  var out []Account
	  for rows.Next() {
		   var a Account
		  var avail, frozen string
		  if err := rows.Scan(&a.ID, &a.UserID, &a.AccountType, &a.Asset, &avail, &frozen); err != nil {
			  return nil, err
		  }
		   var err error
		  if a.Available, err = ParseBook(avail); err != nil {
			  return nil, err
		  }
		  if a.Frozen, err = ParseBook(frozen); err != nil {
			   return nil, err
		   }
		  out = append(out, a)
	   }
	   return out, rows.Err()
   }

// ListUserBooks 用户各科目余额，给钱包页和余额缓存。
  func (s *SQLStore) ListUserBooks(ctx context.Context, userID string) ([]Account, error) {
	  rows, err := s.conn(ctx).QueryContext(ctx, `SELECT id, user_id, account_type, asset, available, frozen FROM ledger_accounts WHERE user_id=?`, userID)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []Account
	   for rows.Next() {
		  var a Account
		  var avail, frozen string
		  if err := rows.Scan(&a.ID, &a.UserID, &a.AccountType, &a.Asset, &avail, &frozen); err != nil {
			   return nil, err
		  }
		  if a.Available, err = ParseBook(avail); err != nil {
			   return nil, err
		   }
		   if a.Frozen, err = ParseBook(frozen); err != nil {
			  return nil, err
		   }
		  out = append(out, a)
	   }
	  return out, rows.Err()
   }

// ListByState worker 按状态扫卡住的单（冻结超时、已提交待确认）。

   func (s *SQLStore) ListByState(ctx context.Context, st State, limit int) ([]*Transfer, error) {
	  rows, err := s.conn(ctx).QueryContext(ctx, `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE state=? ORDER BY created_at LIMIT ?`, st, limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []*Transfer
	   for rows.Next() {
		   t := &Transfer{}
		  var amount string
		   var flags []byte
		   var fail, txh, hint sql.NullString
		  var enc []byte
		   if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			   return nil, err
		   }
		  amt, err := ParseBook(amount)
		  if err != nil {
			   return nil, err
		  }
		  t.Amount = amt
		   t.ToAddressEnc = enc
		   t.ToAddressHint = hint.String
		   t.TxHash = txh.String
		   t.FailReason = fail.String
		  _ = json.Unmarshal(flags,
			   &t.RiskFlags)
		   s.reveal(t); out = append(out, t)
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) ListUserTransfers(ctx context.Context, userID string, limit int) ([]*Transfer, error) {
	  q := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE user_id=? ORDER BY created_at DESC`
	  var rows *sql.Rows
	   var err error
	  if limit > 0 {
		   rows, err = s.conn(ctx).QueryContext(ctx, q+` LIMIT ?`, userID, limit)
	  } else {
		  rows, err = s.conn(ctx).QueryContext(ctx, q, userID)
	  }
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []*Transfer
	   for rows.Next() {
		  t := &Transfer{}
		  var amount string
		  var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		  var enc []byte
		  if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			  return nil, err
		  }
		  amt, err := ParseBook(amount)
		  if err != nil {
			   return nil, err
		  }
		   t.Amount = amt
		  t.ToAddressEnc = enc
		  t.ToAddressHint = hint.String
		   t.FromAddressHint = fromHint.String
		  t.MemoHint = memoHint.String
		   t.TagHint = tagHint.String
		   t.TagNameHint = tagNameHint.String
		   t.TxHash = txh.String
		   t.FailReason = fail.String
		  _ = json.Unmarshal(flags, &t.RiskFlags)
		  s.reveal(t); out = append(out, t)
	  }
	  return out, rows.Err()
   }

  func (s *SQLStore) ListIncomingInner(ctx context.Context, userID string, limit int) ([]*Transfer, error) {
	  q := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE direction=? AND to_address_hint=? ORDER BY created_at DESC`
	  var rows *sql.Rows
	  var err error
	   if limit > 0 {
		   rows, err = s.conn(ctx).QueryContext(ctx, q+` LIMIT ?`, string(SpotSend), userID, limit)
	  } else {
		   rows, err = s.conn(ctx).QueryContext(ctx, q, string(SpotSend), userID)
	  }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []*Transfer
	  for rows.Next() {
		   t := &Transfer{}
		  var amount string
		  var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		  var enc []byte
		  if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			  return nil, err
		   }
		   if t.UserID == userID {
			  continue
		   }
		   amt, err := ParseBook(amount)
		  if err != nil {
			  return nil, err
		   }
		  t.Amount = amt
		   t.ToAddressEnc = enc
		  t.ToAddressHint = hint.String
		  t.FromAddressHint = fromHint.String
		   t.MemoHint = memoHint.String
		   t.TagHint = tagHint.String
		   t.TagNameHint = tagNameHint.String
		   t.TxHash = txh.String
		  t.FailReason = fail.String
		  _ = json.Unmarshal(flags, &t.RiskFlags)
		  s.reveal(t); out = append(out, t)
	   }
	  return out, rows.Err()
  }

// FindByChainTxHash 按 (chain, hash, output_index) 查单。

  func (s *SQLStore) FindByChainTxHash(ctx context.Context, chain, hash string, outputIndex int) (*Transfer, error) {
	   if hash == "" || IsInternalTxHash(hash) {
		  return nil, ErrNotFound
	  }
	  return s.oneTransfer(ctx, `SELECT `+transferCols+` FROM transfers WHERE chain=? AND tx_hash=? AND output_index=? LIMIT 1`, chain, hash, outputIndex)
  }

func (s *SQLStore) oneTransfer(ctx context.Context, query string, args ...any) (*Transfer, error) {
	var found *Transfer
	err := s.visit(ctx, func(q queryer) error {
		t, err := s.scanTransfer(q.QueryRowContext(ctx, query, args...))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		found = t
		return errFound
	})
	if errors.Is(err, errFound) {
		return found, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, ErrNotFound
}

type BookAdapter struct{ S Store }

   func (b BookAdapter) ListUserBooks(ctx context.Context, userID string) ([]wallet.BookRow, error) {
	  accs, err := b.S.ListUserBooks(ctx, userID)
	   if err != nil {
		  return nil, err
	  }
	  out := make([]wallet.BookRow, 0, len(accs))
	   for _, a := range accs {
		   out = append(out,
			   wallet.BookRow{AccountType: string(a.AccountType), Asset: a.Asset, Available: a.Available, Frozen: a.Frozen})
	  }
	   return out, nil
   }

   func (s *SQLStore) Ping(ctx context.Context) error {
	   return s.db.PingContext(ctx)
  }

   func (s *SQLStore) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	   return s.conn(ctx).ExecContext(ctx, query, args...)
  }

   func (s *SQLStore) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	   return s.conn(ctx).QueryRowContext(ctx, query, args...)
  }

  func (s *SQLStore) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	  return s.conn(ctx).QueryContext(ctx, query, args...)
   }

// AppendOutbox 把通知与账本变更写入同一个事务。
// 事务回滚则通知一并消失。严禁在账本提交前对外发出这条通知，也严禁先发通知再改账。
   func (s *SQLStore) AppendOutbox(ctx context.Context, e OutboxEvent) error {
	  if e.ID == "" {
		   e.ID = uuid.NewString()
	   }
	  if e.CreatedAt.IsZero() {
		   e.CreatedAt = time.Now().UTC()
	   }
	  payload, _ := json.Marshal(e.Payload)
	  _, err := s.conn(ctx).ExecContext(ctx, `INSERT INTO outbox_events (id, aggregate_type, aggregate_id, kind, action, user_id, trace_id, state, payload, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		   e.ID,
		  e.AggregateType, e.AggregateID, e.Kind, e.Action, e.UserID, e.TraceID, e.State, payload, e.CreatedAt)
	   return err
  }

// ListOutbox worker 拉取待投递事件；unpublishedOnly 只拿还没 mark 的。

   func (s *SQLStore) ListOutbox(ctx context.Context, unpublishedOnly bool, limit int) ([]OutboxEvent, error) {
	  if limit <= 0 {
		   limit = 50
	   }
	   q := `SELECT id, aggregate_type, aggregate_id, kind, action, user_id, trace_id, state, payload, published_at, created_at FROM outbox_events`
	  if unpublishedOnly {
		  q += ` WHERE published_at IS NULL`
	   }
	   q += ` ORDER BY created_at DESC LIMIT ?`
	  rows,
		   err := s.conn(ctx).QueryContext(ctx, q, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []OutboxEvent
	   for rows.Next() {
		  var e OutboxEvent
		  var payload []byte
		   var pub sql.NullTime
		  if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.Kind, &e.Action, &e.UserID, &e.TraceID, &e.State, &payload, &pub, &e.CreatedAt); err != nil {
			   return nil, err
		   }
		   if len(payload) > 0 {
			   _ = json.Unmarshal(payload, &e.Payload)
		   }
		   if pub.Valid {
			  t := pub.Time
			  e.PublishedAt = &t
		   }
		   out = append(out, e)
	   }
	   return out, rows.Err()
  }

// MarkOutboxPublished 投递成功后标记，避免重复发 wallet.movements。
  func (s *SQLStore) MarkOutboxPublished(ctx context.Context, ids []string) error {
	   if len(ids) == 0 {
		   return nil
	  }
	  now := time.Now().UTC()
	  q := s.conn(ctx)
	   for _, id := range ids {
		  if _, err := q.ExecContext(ctx, `UPDATE outbox_events SET published_at=? WHERE id=? AND published_at IS NULL`, now, id); err != nil {
			  return err
		  }
	  }
	   return nil
   }

   func (s *SQLStore) ListUserJournal(ctx context.Context, userID string, limit int) ([]JournalEntry, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   rows, err := s.conn(ctx).QueryContext(ctx, `SELECT transfer_id, entry_seq, user_id, account_type, asset, debit, credit, memo FROM ledger_journal WHERE user_id=? ORDER BY id DESC LIMIT ?`, userID, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  var out []JournalEntry
	  for rows.Next() {
		   var e JournalEntry
		  var d, c string
		  if err := rows.Scan(&e.TransferID, &e.Seq, &e.UserID, &e.AccountType, &e.Asset, &d, &c, &e.Memo); err != nil {
			   return nil, err
		   }
		  if e.Debit, err = ParseBook(d); err != nil {
			   return nil, err
		   }
		   if e.Credit, err = ParseBook(c); err != nil {
			  return nil, err
		  }
		   out = append(out, e)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) UserControlFrozen(ctx context.Context, userID string) (bool, error) {
	   var v int
	   err := s.conn(ctx).QueryRowContext(ctx, `SELECT frozen FROM user_controls WHERE user_id=?`, userID).Scan(&v)
	   if errors.Is(err, sql.ErrNoRows) {
		  return false, nil
	   }
	   if err != nil {
		   return false, err
	   }
	   return v != 0, nil
  }

   func (s *SQLStore) SetUserControlFrozen(ctx context.Context, userID string, frozen bool) error {
	   flag := 0
	   if frozen {
		  flag = 1
	  }
	   q := s.upsertSQL(
		   `INSERT INTO user_controls (user_id, frozen) VALUES (?,?) ON DUPLICATE KEY UPDATE frozen=VALUES(frozen)`,
		   `INSERT INTO user_controls (user_id, frozen) VALUES (?,?) ON CONFLICT (user_id) DO UPDATE SET frozen=EXCLUDED.frozen`,
	   )
	   _, err := s.conn(ctx).ExecContext(ctx, q, userID, flag)
	  return err
   }

   func (s *SQLStore) UserKYC(ctx context.Context, userID string) (string, error) {
	   var v string
	  err := s.conn(ctx).QueryRowContext(ctx, `SELECT kyc_level FROM user_controls WHERE user_id=?`, userID).Scan(&v)
	   if errors.Is(err, sql.ErrNoRows) {
		  return KYCBasic, nil
	   }
	  if err != nil {
		   return "", err
	  }
	  if v == "" {
		   return KYCBasic, nil
	   }
	   return v, nil
  }

  func (s *SQLStore) SetUserKYC(ctx context.Context, userID, level string) error {
	  q := s.upsertSQL(
		  `INSERT INTO user_controls (user_id, frozen, kyc_level) VALUES (?,0,?) ON DUPLICATE KEY UPDATE kyc_level=VALUES(kyc_level)`,
		   `INSERT INTO user_controls (user_id, frozen, kyc_level) VALUES (?,0,?) ON CONFLICT (user_id) DO UPDATE SET kyc_level=EXCLUDED.kyc_level`,
	  )
	  _, err := s.conn(ctx).ExecContext(ctx, q, userID, level)
	   return err
  }

// ListUserTransfersPaged SQL 充值历史分页 + 保留窗。

   func (s *SQLStore) ListUserTransfersPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   where := `user_id=? AND direction=?`
	   args := []any{q.UserID, string(WalletDeposit)}
	   if !q.Start.IsZero() {
		  where += ` AND created_at>=?`
		   args = append(args, q.Start.UTC())
	  }
	  if !q.End.IsZero() {
		   where += ` AND created_at<=?`
		   args = append(args, q.End.UTC())
	   }
	  if q.Currency != "" {
		  where += ` AND asset=?`
		  args = append(args, q.Currency)
	   }
	   where,
		   args = AppendStateFilter(where, args, q.Status)
	   var total int
	  if err := s.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM transfers WHERE `+where, args...).Scan(&total); err != nil {
		   return nil, 0, err
	   }
	   qsql := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE ` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	  args2 := append(append([]any{}, args...), q.PageSize, q.Offset())
	   rows, err := s.conn(ctx).QueryContext(ctx, qsql, args2...)
	   if err != nil {
		  return nil, 0, err
	  }
	  defer rows.Close()
	  out := make([]*Transfer, 0, q.PageSize)
	  for rows.Next() {
		  t := &Transfer{}
		   var amount string
		   var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		   var enc []byte
		   if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			   return nil, 0, err
		   }
		  amt, err := ParseBook(amount)
		   if err != nil {
			   return nil, 0, err
		  }
		   t.Amount = amt
		  t.ToAddressEnc = enc
		   t.ToAddressHint = hint.String
		  t.FromAddressHint = fromHint.String
		   t.MemoHint = memoHint.String
		  t.TagHint = tagHint.String
		  t.TagNameHint = tagNameHint.String
		   t.TxHash = txh.String
		  t.FailReason = fail.String
		   _ = json.Unmarshal(flags,
			   &t.RiskFlags)
		   s.reveal(t); out = append(out, t)
	  }
	  return out, total, rows.Err()
   }

// ListIncomingInnerPaged 内部转入真 SQL 分页 + 保留窗（与充值链上同口径）。
  func (s *SQLStore) ListIncomingInnerPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	  where := `direction=? AND to_address_hint=? AND user_id<>?`
	  args := []any{string(SpotSend), q.UserID, q.UserID}
	   if !q.Start.IsZero() {
		  where += ` AND created_at>=?`
		   args = append(args, q.Start.UTC())
	  }
	   if !q.End.IsZero() {
		  where += ` AND created_at<=?`
		  args = append(args, q.End.UTC())
	   }
	  if q.Currency != "" {
		   where += ` AND asset=?`
		  args = append(args, q.Currency)
	  }
	  where, args = AppendStateFilter(where, args, q.Status)
	   var total int
	  if err := s.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM transfers WHERE `+where, args...).Scan(&total); err != nil {
		  return nil, 0, err
	   }
	   qsql := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE ` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	   args2 := append(append([]any{},
		   args...), q.PageSize, q.Offset())
	   rows, err := s.conn(ctx).QueryContext(ctx, qsql, args2...)
	   if err != nil {
		  return nil, 0, err
	   }
	   defer rows.Close()
	   out := make([]*Transfer, 0, q.PageSize)
	   for rows.Next() {
		  t := &Transfer{}
		   var amount string
		  var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		   var enc []byte
		  if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			   return nil, 0, err
		   }
		   amt, err := ParseBook(amount)
		   if err != nil {
			   return nil, 0, err
		  }
		  t.Amount = amt
		   t.ToAddressEnc = enc
		   t.ToAddressHint = hint.String
		   t.FromAddressHint = fromHint.String
		   t.MemoHint = memoHint.String
		  t.TagHint = tagHint.String
		   t.TagNameHint = tagNameHint.String
		  t.TxHash = txh.String
		   t.FailReason = fail.String
		  _ = json.Unmarshal(flags, &t.RiskFlags)
		   s.reveal(t); out = append(out, t)
	  }
	  return out, total, rows.Err()
  }

// ListUserWithdrawsPaged SQL 提现历史分页 + 保留窗。
// status / isInner 在 SQL 层过滤，避免事后 match 导致页内空洞与 totalNum 漂移。
   func (s *SQLStore) ListUserWithdrawsPaged(ctx context.Context, q WithdrawHistQuery) ([]*Transfer, int, error) {
	  where := `user_id=?`
	  args := []any{q.UserID}
	   switch strings.ToLower(strings.TrimSpace(q.InnerFilter)) {
	   case "true", "1", "yes", "inner":
		  where += ` AND direction=?`
		   args = append(args, string(SpotSend))
	   case "false", "0", "no", "chain":
		   where += ` AND direction=?`
		   args = append(args, string(WalletWithdraw))
	  default:
		   where += ` AND direction IN (?,?)`
		   args = append(args, string(WalletWithdraw), string(SpotSend))
	   }
	   if !q.Start.IsZero() {
		   where += ` AND created_at>=?`
		   args = append(args,
			  q.Start.UTC())
	  }
	   if !q.End.IsZero() {
		  where += ` AND created_at<=?`
		  args = append(args, q.End.UTC())
	  }
	  if q.Currency != "" {
		  where += ` AND asset=?`
		   args = append(args, q.Currency)
	  }
	  where, args = AppendStateFilter(where, args, q.Status)
	   var total int
	   if err := s.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM transfers WHERE `+where, args...).Scan(&total); err != nil {
		  return nil, 0, err
	  }
	   qsql := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE ` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	   args2 := append(append([]any{},
		  args...), q.PageSize, q.Offset())
	   rows, err := s.conn(ctx).QueryContext(ctx, qsql, args2...)
	   if err != nil {
		   return nil, 0, err
	   }
	  defer rows.Close()
	   out := make([]*Transfer, 0, q.PageSize)
	   for rows.Next() {
		  t := &Transfer{}
		   var amount string
		  var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		  var enc []byte
		   if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			   return nil, 0, err
		  }
		   amt, err := ParseBook(amount)
		   if err != nil {
			  return nil, 0, err
		  }
		  t.Amount = amt
		   t.ToAddressEnc = enc
		  t.ToAddressHint = hint.String
		   t.FromAddressHint = fromHint.String
		  t.MemoHint = memoHint.String
		   t.TagHint = tagHint.String
		  t.TagNameHint = tagNameHint.String
		  t.TxHash = txh.String
		  t.FailReason = fail.String
		  _ = json.Unmarshal(flags, &t.RiskFlags)
		   s.reveal(t); out = append(out, t)
	  }
	  return out, total, rows.Err()
   }

// ListUserDepositsMergedPaged 链上入金+内部转入统一 SQL 按 created_at 降序合并分页。
// 双源同 WHERE OR + ORDER BY，避免两源各自 OFFSET 拼接乱序；InnerMode 收窄单源。
// status/currency 在 SQL 过滤；totalNum 与 items 同口径。

   func (s *SQLStore) ListUserDepositsMergedPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   var where string
	  var args []any
	   switch q.InnerMode {
	   case DepositInnerOnly:
		  where = `direction=? AND to_address_hint=? AND user_id<>?`
		   args = []any{string(SpotSend), q.UserID, q.UserID}
	   case DepositChainOnly:
		  where = `user_id=? AND direction=?`
		   args = []any{q.UserID, string(WalletDeposit)}
	  default:
		  where = `((user_id=? AND direction=?) OR (direction=? AND to_address_hint=? AND user_id<>?))`
		   args = []any{q.UserID,
			  string(WalletDeposit), string(SpotSend), q.UserID, q.UserID}
	   }
	   if !q.Start.IsZero() {
		   where += ` AND created_at>=?`
		  args = append(args, q.Start.UTC())
	  }
	   if !q.End.IsZero() {
		   where += ` AND created_at<=?`
		   args = append(args, q.End.UTC())
	   }
	  if q.Currency != "" {
		  where += ` AND asset=?`
		   args = append(args, q.Currency)
	  }
	   where, args = AppendStateFilter(where, args, q.Status)
	  var total int
	   if err := s.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM transfers WHERE `+where, args...).Scan(&total); err != nil {
		  return nil, 0, err
	  }
	   qsql := `SELECT id, idempotency_key, user_id, direction, asset, amount, chain, to_address_enc, to_address_hint, from_address_hint, memo_hint, tag_hint, tag_name_hint, tx_hash, state, risk_flags, fail_reason, created_at, updated_at FROM transfers WHERE ` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	  args2 := append(append([]any{}, args...), q.PageSize, q.Offset())
	  rows,
		   err := s.conn(ctx).QueryContext(ctx, qsql, args2...)
	  if err != nil {
		  return nil, 0, err
	  }
	   defer rows.Close()
	  out := make([]*Transfer,
		   0, q.PageSize)
	  for rows.Next() {
		   t := &Transfer{}
		  var amount string
		  var flags []byte
		  var fail, txh, hint, fromHint, memoHint, tagHint, tagNameHint sql.NullString
		   var enc []byte
		   if err := rows.Scan(&t.ID, &t.IdempotencyKey, &t.UserID, &t.Direction, &t.Asset, &amount, &t.Chain, &enc, &hint, &fromHint, &memoHint, &tagHint, &tagNameHint, &txh, &t.State, &flags, &fail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			   return nil, 0, err
		  }
		   amt, err := ParseBook(amount)
		   if err != nil {
			  return nil, 0, err
		  }
		  t.Amount = amt
		  t.ToAddressEnc = enc
		  t.ToAddressHint = hint.String
		  t.FromAddressHint = fromHint.String
		   t.MemoHint = memoHint.String
		  t.TagHint = tagHint.String
		  t.TagNameHint = tagNameHint.String
		   t.TxHash = txh.String
		  t.FailReason = fail.String
		   _ = json.Unmarshal(flags, &t.RiskFlags)
		  s.reveal(t); out = append(out, t)
	   }
	  return out, total, rows.Err()
  }
