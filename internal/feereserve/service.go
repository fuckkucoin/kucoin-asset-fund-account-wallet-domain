// 功能：service.go
// 出金手续费储备入账——领域服务编排。
// Service 负责：
// - Reserve：在出金广播前冻结预估费，创建储备记录（幂等）；
// - Settle：广播确认后传入实际费，计算并落账 delta 凭证；
// - Release：广播取消时退还冻结；
// - ListPendingDeltas：Ops 查询待处理 delta（over/under）列表；
// - Health：服务健康状态。
// SQL 模式下储备记录持久化到 fee_reserves 表，审计记录到 fee_reserve_audit。
// 内存模式退化为 sync.Map（ / 内嵌场景）。
// CreateDate: 2020.07.17

// 作者：Cason
// 日期：2021.05.10

package feereserve

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Service 手续费储备服务。
type Service struct {
	mu    sync.Mutex
	mem   map[string]*FeeReserve // withdraw_id → reserve（内存模式）
	sqlDB *persist.SQLDB
	clock func() time.Time
}

// New 创建 Service。sqlDB 非 nil 时使用 SQL 持久化。

   func New(db *persist.SQLDB) *Service {
	  return &Service{
		   mem: map[string]*FeeReserve{},
		   sqlDB: db,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// NewMem 纯内存模式（用）。

  func NewMem() *Service { return New(nil) }

// WithClock 替换时钟（用）。

  func (s *Service) WithClock(c func() time.Time) *Service {
	   if s != nil && c != nil {
		   s.clock = c
	  }
	  return s
   }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
   }

  func (s *Service) sqlOn() bool {
	  return s != nil && s.sqlDB != nil && s.sqlDB.DB != nil
  }

// Reserve 在出金广播前冻结预估手续费，创建储备记录。
// 幂等：相同 WithdrawID 重复调用返回已有记录。
   func (s *Service) Reserve(ctx context.Context, req ReserveRequest) (FeeReserve, error) {
	   if s == nil {
		   return FeeReserve{},
			   ErrUnavailable
	   }
	  wid := strings.TrimSpace(req.WithdrawID)
	  if wid == "" {
		  return FeeReserve{}, ErrWithdrawID
	   }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		   return FeeReserve{}, err
	   }
	   asset, err := validate.Asset(req.Asset)
	  if err != nil {
		   return FeeReserve{}, ErrAmount
	  }
	  chain, err := validate.Chain(req.Chain)
	   if err != nil {
		   return FeeReserve{}, err
	  }
	   reservedAmt, err := money.ParseBook(strings.TrimSpace(req.ReservedFee))
	   if err != nil || !reservedAmt.IsPositive() {
		  return FeeReserve{}, fmt.Errorf("%w: reserved_fee", ErrAmount)
	  }
	   now := s.now()
	  if s.sqlOn() {
		   return s.sqlReserve(ctx, wid, uid, asset, chain, req.VersionID, reservedAmt, now)
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if existing, ok := s.mem[wid]; ok {
		   return *existing, nil
	   }
	   r := &FeeReserve{
		  ID:         uuid.NewString(),
		   WithdrawID: wid,
		  VersionID: strings.TrimSpace(req.VersionID),
		   UserID: uid,
		   Asset:  asset,
		  Chain:  chain,
		  Reserved:  reservedAmt.String(),
		  State:     StateReserved,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  s.mem[wid] = r
	   return *r, nil
   }

// Settle 广播确认后传入实际手续费，计算 delta 并将储备推进到 StateSettled。
// 返回结算结果（含 delta 凭证行）；实际凭证写入由调用方通过 ledger.Service 完成。
   func (s *Service) Settle(ctx context.Context, req SettleRequest) (SettleResult, error) {
	  if s == nil {
		   return SettleResult{}, ErrUnavailable
	  }
	   wid := strings.TrimSpace(req.WithdrawID)
	   if wid == "" {
		   return SettleResult{}, ErrWithdrawID
	   }
	  actualStr := strings.TrimSpace(req.ActualFee)
	   actualAmt, err := money.ParseBook(actualStr)
	  if err != nil {
		   return SettleResult{}, fmt.Errorf("%w: actual_fee", ErrAmount)
	   }
	   if s.sqlOn() {
		  return s.sqlSettle(ctx, wid, actualAmt, req.Actor)
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  r, ok := s.mem[wid]
	  if !ok {
		   return SettleResult{},
			   ErrNotFound
	   }
	  if err := r.CanSettle(); err != nil {
		  return SettleResult{}, err
	   }
	  cp := *r
	  delta, dir, err := cp.ComputeDelta(actualAmt.String())
	   if err != nil {
		   return SettleResult{}, err
	   }
	  reservedAmt, _ := money.ParseBook(cp.Reserved)
	  js, err := BuildSettleJournal(cp.Asset, reservedAmt, actualAmt, dir, delta)
	  if err != nil {
		   return SettleResult{}, err
	  }
	   now := s.now()
	  cp.Actual = actualAmt.String()
	   cp.Delta = delta.String()
	  cp.DeltaDir = dir
	  cp.State = StateSettled
	  cp.SettledAt = &now
	  cp.UpdatedAt = now
	   s.mem[wid] = &cp
	   return SettleResult{Reserve: cp, Lines: js.ToDeltaLines()}, nil
   }

// Release 取消出金时退还冻结（出金单未广播时调用）。
// 幂等：已 StateReleased 的记录直接返回。

  func (s *Service) Release(ctx context.Context, withdrawID string) (FeeReserve, error) {
	   if s == nil {
		   return FeeReserve{}, ErrUnavailable
	  }
	  wid := strings.TrimSpace(withdrawID)
	   if wid == "" {
		   return FeeReserve{}, ErrWithdrawID
	  }
	   if s.sqlOn() {
		   return s.sqlRelease(ctx, wid)
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  r, ok := s.mem[wid]
	  if !ok {
		  return FeeReserve{}, ErrNotFound
	   }
	   if r.State == StateReleased {
		  return *r, nil
	   }
	   if err := r.CanRelease(); err != nil {
		   return FeeReserve{}, err
	  }
	   cp := *r
	  now := s.now()
	  cp.State = StateReleased
	  cp.UpdatedAt = now
	  s.mem[wid] = &cp
	  return cp, nil
  }

// GetByWithdraw 按出金单 ID 查询储备记录。
   func (s *Service) GetByWithdraw(ctx context.Context, withdrawID string) (FeeReserve, error) {
	   if s == nil {
		   return FeeReserve{}, ErrUnavailable
	   }
	   wid := strings.TrimSpace(withdrawID)
	   if s.sqlOn() {
		  return s.sqlGet(ctx, wid)
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   r, ok := s.mem[wid]
	   if !ok {
		  return FeeReserve{}, ErrNotFound
	  }
	   return *r, nil
  }

// ListPendingDeltas 列出所有处于 StateSettled 且 DeltaDir != zero 的记录
// （供 ops 桌面查看待处理 delta 凭证）。
  func (s *Service) ListPendingDeltas(ctx context.Context, limit int) ([]FeeReserve, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	  }
	  if s.sqlOn() {
		   return s.sqlListPendingDeltas(ctx, limit)
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []FeeReserve
	  for _, r := range s.mem {
		  if r.State == StateSettled && r.DeltaDir != DeltaZero {
			   out = append(out, *r)
			   if len(out) >= limit {
				   break
			  }
		  }
	   }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].CreatedAt.After(out[j].CreatedAt)
	  })
	  return out, nil
  }

// ListByUser 按用户 ID 列出储备记录。
  func (s *Service) ListByUser(ctx context.Context, userID string, limit int) ([]FeeReserve, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	   if s.sqlOn() {
		   return s.sqlListByUser(ctx, userID, limit)
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []FeeReserve
	   for _, r := range s.mem {
		   if userID == "" || r.UserID == userID {
			   out = append(out, *r)
		  }
		  if len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
  }

// HealthSummary 返回储备服务健康摘要（统计各状态计数）。

  func (s *Service) HealthSummary(ctx context.Context) (map[string]any, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.sqlOn() {
		  return s.sqlHealthSummary(ctx)
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  counts := map[string]int{
		  string(StateReserved): 0,
		   string(StateSettled): 0,
		   string(StateReleased): 0,
	   }
	   deltas := map[string]int{string(DeltaOver): 0, string(DeltaUnder): 0, string(DeltaZero): 0}
	  for _, r := range s.mem {
		  counts[string(r.State)]++
		  if r.DeltaDir != "" {
			   deltas[string(r.DeltaDir)]++
		   }
	  }
	  return map[string]any{
		  "state_counts": counts,
		  "delta_counts": deltas,
		   "backend": "memory",
	   }, nil
  }

// --- SQL 实现 ---

  func (s *Service) sqlReserve(ctx context.Context, wid, uid, asset, chain, versionID string,
	  reserved money.Amount, now time.Time) (FeeReserve, error) {
	   // 检查幂等。
	  if existing, err := s.sqlGet(ctx, wid); err == nil {
		  return existing, nil
	  }
	  id := uuid.NewString()
	  _, err := s.sqlDB.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO fee_reserves
			(id, withdraw_id, version_id, user_id, asset, chain, reserved, actual, delta, delta_dir,
			 state, ledger_reserve, ledger_settle, ledger_delta, settled_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   id, wid, versionID, uid, asset, chain, reserved.String(), "0", "0", string(DeltaZero),
		  string(StateReserved), "", "", "", nil, now, now,
	  )
	  if err != nil {
		  return FeeReserve{}, err
	  }
	   // 写审计。
	   s.writeAudit(ctx, id, wid, "reserve", "", string(StateReserved), "", now)
	   return s.sqlGet(ctx, wid)
   }

  func (s *Service) sqlSettle(ctx context.Context, wid string, actualAmt money.Amount, actor string) (SettleResult, error) {
	   r, err := s.sqlGet(ctx, wid)
	  if err != nil {
		   return SettleResult{}, ErrNotFound
	  }
	  if r.State == StateSettled {
		  return SettleResult{Reserve: r}, nil
	  }
	   if err := r.CanSettle(); err != nil {
		   return SettleResult{}, err
	  }
	   delta, dir, err := r.ComputeDelta(actualAmt.String())
	   if err != nil {
		   return SettleResult{}, err
	  }
	   reservedAmt, _ := money.ParseBook(r.Reserved)
	   js, err := BuildSettleJournal(r.Asset, reservedAmt, actualAmt, dir, delta)
	  if err != nil {
		  return SettleResult{}, err
	   }
	  now := s.now()
	   _, err = s.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_reserves SET actual=?, delta=?, delta_dir=?, state='settled', settled_at=?, updated_at=?
		 WHERE withdraw_id=? AND state='reserved'`,
		  actualAmt.String(), delta.String(), string(dir), now, now, wid,
	  )
	   if err != nil {
		  return SettleResult{}, err
	   }
	   s.writeAudit(ctx, r.ID, wid, "settle", string(StateReserved), string(StateSettled), actor, now)
	   updated, err := s.sqlGet(ctx, wid)
	   if err != nil {
		   return SettleResult{}, err
	   }
	  return SettleResult{Reserve: updated, Lines: js.ToDeltaLines()}, nil
  }

   func (s *Service) sqlRelease(ctx context.Context, wid string) (FeeReserve, error) {
	  r, err := s.sqlGet(ctx, wid)
	  if err != nil {
		  return FeeReserve{}, ErrNotFound
	   }
	  if r.State == StateReleased {
		   return r, nil
	  }
	  if err := r.CanRelease(); err != nil {
		   return FeeReserve{}, err
	  }
	  now := s.now()
	   _, err = s.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_reserves SET state='released', updated_at=? WHERE withdraw_id=? AND state='reserved'`,
		   now, wid,
	  )
	  if err != nil {
		   return FeeReserve{}, err
	   }
	  s.writeAudit(ctx, r.ID, wid, "release", string(StateReserved), string(StateReleased), "", now)
	  return s.sqlGet(ctx, wid)
  }

  func (s *Service) sqlGet(ctx context.Context, wid string) (FeeReserve, error) {
	  row := s.sqlDB.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, withdraw_id, version_id, user_id, asset, chain, reserved, actual, delta, delta_dir, 
		        state, ledger_reserve, ledger_settle, ledger_delta, settled_at, created_at, updated_at
		 FROM fee_reserves WHERE withdraw_id=?`, wid)
	  return scanReserve(row)
  }

   func (s *Service) sqlListPendingDeltas(ctx context.Context, limit int) ([]FeeReserve, error) {
	  rows,
		   err := s.sqlDB.Conn(ctx).QueryContext(ctx,
		  `SELECT id, withdraw_id, version_id, user_id, asset, chain, reserved, actual, delta, delta_dir,
		        state, ledger_reserve, ledger_settle, ledger_delta, settled_at, created_at, updated_at
		 FROM fee_reserves WHERE state='settled' AND delta_dir != 'zero'
		 ORDER BY created_at DESC LIMIT ?`, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanReserves(rows)
   }

  func (s *Service) sqlListByUser(ctx context.Context, userID string, limit int) ([]FeeReserve, error) {
	  rows, err := s.sqlDB.Conn(ctx).QueryContext(ctx,
		   `SELECT id, withdraw_id, version_id, user_id, asset, chain, reserved, actual, delta, delta_dir,
		        state, ledger_reserve, ledger_settle, ledger_delta, settled_at, created_at, updated_at
		 FROM fee_reserves WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanReserves(rows)
  }

   func (s *Service) sqlHealthSummary(ctx context.Context) (map[string]any, error) {
	   rows, err := s.sqlDB.Conn(ctx).QueryContext(ctx,
		  `SELECT state, COUNT(*) FROM fee_reserves GROUP BY state`)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  counts := map[string]int{}
	   for rows.Next() {
		   var st string
		  var cnt int
		  if e := rows.Scan(&st, &cnt); e == nil {
			   counts[st] = cnt
		   }
	  }
	  dRows, err := s.sqlDB.Conn(ctx).QueryContext(ctx,
		  `SELECT delta_dir, COUNT(*) FROM fee_reserves WHERE state='settled' GROUP BY delta_dir`)
	   if err != nil {
		  return nil, err
	  }
	   defer dRows.Close()
	   deltas := map[string]int{}
	  for dRows.Next() {
		   var dir string
		  var cnt int
		   if e := dRows.Scan(&dir, &cnt); e == nil {
			  deltas[dir] = cnt
		   }
	   }
	   return map[string]any{
		   "state_counts": counts,
		  "delta_counts": deltas,
		  "backend":      "sql",
	   }, nil
   }

  func (s *Service) writeAudit(ctx context.Context, reserveID, wid, event, from, to, actor string, now time.Time) {
	   if !s.sqlOn() {
		   return
	   }
	  id := uuid.NewString()
	   _, _ = s.sqlDB.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO fee_reserve_audit (id, reserve_id, withdraw_id, event, from_state, to_state, actor, note, at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		  id, reserveID, wid, event, from, to, actor, "", now,
	  )
  }

// scanReserve 扫描单行。
  func scanReserve(row *sql.Row) (FeeReserve, error) {
	   var r FeeReserve
	   var versionID,
		  actual, delta, deltaDir, ledgerRes, ledgerSet, ledgerDel sql.NullString
	   var settledAt sql.NullTime
	   err := row.Scan(
		  &r.ID, &r.WithdrawID, &versionID, &r.UserID, &r.Asset, &r.Chain,
		  &r.Reserved,
		  &actual, &delta, &deltaDir,
		   &r.State, &ledgerRes, &ledgerSet, &ledgerDel, &settledAt, &r.CreatedAt, &r.UpdatedAt,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		   return FeeReserve{}, ErrNotFound
	  }
	   if err != nil {
		  return FeeReserve{}, err
	  }
	   if versionID.Valid {
		   r.VersionID = versionID.String
	   }
	  if actual.Valid {
		  r.Actual = actual.String
	  }
	   if delta.Valid {
		  r.Delta = delta.String
	   }
	   if deltaDir.Valid {
		   r.DeltaDir = DeltaDir(deltaDir.String)
	   }
	  if ledgerRes.Valid {
		   r.LedgerReserve = ledgerRes.String
	   }
	  if ledgerSet.Valid {
		  r.LedgerSettle = ledgerSet.String
	   }
	  if ledgerDel.Valid {
		  r.LedgerDelta = ledgerDel.String
	   }
	  if settledAt.Valid {
		   t := settledAt.Time
		  r.SettledAt = &t
	   }
	   return r, nil
  }

   func scanReserves(rows *sql.Rows) ([]FeeReserve, error) {
	   var out []FeeReserve
	   for rows.Next() {
		  var r FeeReserve
		   var versionID,
			   actual, delta, deltaDir, ledgerRes, ledgerSet, ledgerDel sql.NullString
		  var settledAt sql.NullTime
		  if err := rows.Scan(
			   &r.ID, &r.WithdrawID, &versionID, &r.UserID, &r.Asset, &r.Chain,
			  &r.Reserved,
			  &actual, &delta, &deltaDir,
			  &r.State, &ledgerRes, &ledgerSet, &ledgerDel, &settledAt, &r.CreatedAt, &r.UpdatedAt,
		  ); err != nil {
			  return nil, err
		  }
		   if versionID.Valid {
			  r.VersionID = versionID.String
		  }
		   if actual.Valid {
			   r.Actual = actual.String
		  }
		   if delta.Valid {
			   r.Delta = delta.String
		   }
		  if deltaDir.Valid {
			   r.DeltaDir = DeltaDir(deltaDir.String)
		   }
		   if ledgerRes.Valid {
			   r.LedgerReserve = ledgerRes.String
		   }
		   if ledgerSet.Valid {
			  r.LedgerSettle = ledgerSet.String
		  }
		  if ledgerDel.Valid {
			  r.LedgerDelta = ledgerDel.String
		   }
		   if settledAt.Valid {
			  t := settledAt.Time
			   r.SettledAt = &t
		   }
		   out = append(out, r)
	  }
	  return out, rows.Err()
  }
