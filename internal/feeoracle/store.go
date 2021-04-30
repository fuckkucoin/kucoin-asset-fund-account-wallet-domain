package feeoracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// QuoteVersionStore 报价代次存储。
// sqlDB 为 nil 时退化为内存存储（仅限非/）。

type QuoteVersionStore struct {
	mu    sync.Mutex
	mem   map[string]*QuoteVersion
	byWD  map[string]string // withdraw_id → version_id
	sqlDB *persist.SQLDB
	clock func() time.Time
}

// NewVersionStore 创建存储实例。sqlDB 非 nil 时使用 SQL 持久化。

   func NewVersionStore(db *persist.SQLDB) *QuoteVersionStore {
	   return &QuoteVersionStore{
		   mem:   map[string]*QuoteVersion{},
		  byWD:  map[string]string{},
		   sqlDB: db,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 替换时钟函数（用）。
  func (s *QuoteVersionStore) WithClock(c func() time.Time) *QuoteVersionStore {
	  if s != nil && c != nil {
		   s.clock = c
	   }
	   return s
   }

   func (s *QuoteVersionStore) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	  return s.clock()
   }

// Save 首次写入一个新代次（StateActive）。幂等：若 ID 已存在则返回已有代次。

   func (s *QuoteVersionStore) Save(ctx context.Context, v QuoteVersion) (QuoteVersion, error) {
	  if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	   }
	  if strings.TrimSpace(v.ID) == "" {
		   return QuoteVersion{}, fmt.Errorf("%w: version id required", ErrVersionNotFound)
	   }
	   if s.sqlOn() {
		   if existing, err := s.sqlGet(ctx, v.ID); err == nil {
			   return existing, nil
		  }
		  return v, s.sqlSave(ctx, v)
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if existing, ok := s.mem[v.ID]; ok {
		  return *existing, nil
	  }
	   cp := v
	   s.mem[v.ID] = &cp
	   return cp, nil
  }

// Find 按代次 ID 查询。
   func (s *QuoteVersionStore) Find(ctx context.Context, versionID string) (QuoteVersion, error) {
	   if s == nil {
		  return QuoteVersion{}, ErrUnavailable
	  }
	  if s.sqlOn() {
		  return s.sqlGet(ctx, versionID)
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  v, ok := s.mem[versionID]
	   if !ok {
		   return QuoteVersion{}, ErrVersionNotFound
	   }
	   return *v, nil
  }

// FindByWithdraw 按出金单 ID 查询已绑定的代次。

   func (s *QuoteVersionStore) FindByWithdraw(ctx context.Context, withdrawID string) (QuoteVersion, error) {
	  if s == nil {
		  return QuoteVersion{},
			   ErrUnavailable
	  }
	  wid := strings.TrimSpace(withdrawID)
	   if wid == "" {
		  return QuoteVersion{}, fmt.Errorf("%w: withdraw_id required", ErrVersionNotFound)
	   }
	   if s.sqlOn() {
		   return s.sqlGetByWithdraw(ctx, wid)
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   vid, ok := s.byWD[wid]
	  if !ok {
		   return QuoteVersion{}, ErrVersionNotFound
	   }
	  v, ok := s.mem[vid]
	  if !ok {
		   return QuoteVersion{}, ErrVersionNotFound
	  }
	  return *v, nil
  }

// Bind 将代次绑定到出金单。幂等：相同出金单重复绑定返回已有代次。
// 代次必须处于 StateActive 且未过期；否则返回对应错误。

  func (s *QuoteVersionStore) Bind(ctx context.Context, versionID, withdrawID string) (QuoteVersion, error) {
	  if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	  }
	  versionID = strings.TrimSpace(versionID)
	  withdrawID = strings.TrimSpace(withdrawID)
	   if versionID == "" {
		  return QuoteVersion{}, fmt.Errorf("%w: version_id required", ErrVersionNotFound)
	  }
	  if withdrawID == "" {
		   return QuoteVersion{}, fmt.Errorf("%w: withdraw_id required", ErrVersionConflict)
	  }
	  now := s.now()
	  if s.sqlOn() {
		   return s.sqlBind(ctx, versionID, withdrawID, now)
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   // 先检查出金单是否已绑定其他代次。
	  if existingVID, ok := s.byWD[withdrawID]; ok && existingVID != versionID {
		  return QuoteVersion{}, fmt.Errorf("%w: withdraw=%s already bound to version=%s", ErrVersionConflict, withdrawID, existingVID)
	  }
	  v, ok := s.mem[versionID]
	   if !ok {
		   return QuoteVersion{}, ErrVersionNotFound
	   }
	   cp := *v
	   // 幂等：已绑定到同一出金单直接返回。
	  if cp.State == StateBound && cp.WithdrawID == withdrawID {
		  return cp, nil
	  }
	   if err := cp.CanBind(now); err != nil {
		  return QuoteVersion{}, err
	  }
	   t := now
	   cp.State = StateBound
	   cp.WithdrawID = withdrawID
	  cp.BoundAt = &t
	  s.mem[versionID] = &cp
	   s.byWD[withdrawID] = versionID
	   return cp, nil
  }

// Settle 广播确认后记录实际费用，将代次推进为 StateSettled。
// actualFee 为实际链上手续费（用户资产计价），只读写入，不触碰 conserve FSM。

   func (s *QuoteVersionStore) Settle(ctx context.Context, versionID, actualFee string) (QuoteVersion, error) {
	   if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	  }
	   versionID = strings.TrimSpace(versionID)
	  if versionID == "" {
		   return QuoteVersion{}, ErrVersionNotFound
	   }
	  if s.sqlOn() {
		   return s.sqlSettle(ctx, versionID, actualFee)
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   v, ok := s.mem[versionID]
	   if !ok {
		  return QuoteVersion{},
			  ErrVersionNotFound
	   }
	   cp := *v
	   if cp.State == StateSettled {
		   return cp, nil
	   }
	   cp.State = StateSettled
	  s.mem[versionID] = &cp
	   return cp, nil
  }

// ExpireOld 把所有 StateActive 且已超过 ExpiresAt 的代次推进为 StateExpired。
// 返回推进的代次数量；供后台 cron 定期调用。

   func (s *QuoteVersionStore) ExpireOld(ctx context.Context) (int, error) {
	   if s == nil {
		  return 0, ErrUnavailable
	   }
	  now := s.now()
	  if s.sqlOn() {
		   return s.sqlExpireOld(ctx, now)
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   count := 0
	   for id, v := range s.mem {
		   if v.State == StateActive && v.IsExpired(now) {
			   cp := *v
			  cp.State = StateExpired
			  s.mem[id] = &cp
			  count++
		   }
	  }
	   return count, nil
  }

// ListByWithdraw 列出某出金单相关的所有代次（含历史）。

   func (s *QuoteVersionStore) ListByWithdraw(ctx context.Context, withdrawID string) ([]QuoteVersion, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   wid := strings.TrimSpace(withdrawID)
	  if s.sqlOn() {
		  return s.sqlListByWithdraw(ctx,
			  wid)
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []QuoteVersion
	  for _, v := range s.mem {
		   if v.WithdrawID == wid {
			  out = append(out, *v)
		  }
	   }
	  return out, nil
   }

// --- SQL 实现 ---

  func (s *QuoteVersionStore) sqlOn() bool {
	  return s != nil && s.sqlDB != nil && s.sqlDB.DB != nil
  }

// sqlSave 首次插入代次行（INSERT IGNORE / ON CONFLICT DO NOTHING）。
  func (s *QuoteVersionStore) sqlSave(ctx context.Context, v QuoteVersion) error {
	   tickJSON,
		   err := json.Marshal(v.Tick)
	  if err != nil {
		   return err
	  }
	   var boundAt *time.Time
	  if v.BoundAt != nil {
		  t := *v.BoundAt
		   boundAt = &t
	   }
	   q := `INSERT INTO fee_quote_versions 
		(id, withdraw_id, chain, family, asset, gross, quote_fee, quote_net,
		 fee_rate, gas_limit, base_fee, priority, native_fee, tick_id, tick_json,
		 schedule_id, bps, state, expires_at, bound_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE id=id`
	   if s.sqlDB.Dialect == persist.Postgres {
		  q = `INSERT INTO fee_quote_versions 
			(id, withdraw_id, chain, family, asset, gross, quote_fee, quote_net,
			 fee_rate, gas_limit, base_fee, priority, native_fee, tick_id, tick_json,
			 schedule_id, bps, state, expires_at, bound_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT (id) DO NOTHING`
	  }
	  feeRate := v.Tick.Normalized.NativeAmount.String()
	   gasLimit := int64(0)
	  if v.Tick.Input.GasLimit > 0 {
		   gasLimit = v.Tick.Input.GasLimit
	   }
	   baseFee := v.Tick.Input.BaseFeeGwei.String()
	   priority := v.Tick.Input.PriorityFeeGwei.String()
	   nativeFee := v.Tick.Normalized.NativeAmount.String()
	   _, err = s.sqlDB.Conn(ctx).ExecContext(ctx, q,
		  v.ID,
		   v.WithdrawID, v.Chain, string(v.Family), v.Asset, v.Gross, v.QuoteFee, v.QuoteNet,
		  feeRate, gasLimit, baseFee, priority, nativeFee, v.Tick.SourceID, string(tickJSON),
		  v.ScheduleID, v.BPS, string(v.State), v.ExpiresAt, boundAt, v.CreatedAt,
	   )
	   return err
  }

// sqlGet 按代次 ID 查询。
   func (s *QuoteVersionStore) sqlGet(ctx context.Context, versionID string) (QuoteVersion, error) {
	   row := s.sqlDB.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, withdraw_id, chain, family, asset, gross, quote_fee, quote_net,
		        schedule_id, bps, state, expires_at, bound_at, created_at, tick_json
		 FROM fee_quote_versions WHERE id=?`, versionID)
	  return s.scanVersion(row)
   }

// sqlGetByWithdraw 按出金单 ID 查询已绑定代次。
   func (s *QuoteVersionStore) sqlGetByWithdraw(ctx context.Context, withdrawID string) (QuoteVersion, error) {
	  row := s.sqlDB.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, withdraw_id, chain, family, asset, gross, quote_fee, quote_net,
		        schedule_id, bps, state, expires_at, bound_at, created_at, tick_json
		 FROM fee_quote_versions WHERE withdraw_id=? AND state IN ('active','bound','settled')
		 ORDER BY created_at DESC LIMIT 1`, withdrawID)
	  return s.scanVersion(row)
  }

// sqlBind 在 SQL 中绑定代次到出金单（CAS 更新，防并发冲突）。
  func (s *QuoteVersionStore) sqlBind(ctx context.Context, versionID, withdrawID string, now time.Time) (QuoteVersion, error) {
	  v, err := s.sqlGet(ctx, versionID)
	  if err != nil {
		  return QuoteVersion{},
			   ErrVersionNotFound
	   }
	   // 幂等检查。
	   if v.State == StateBound && v.WithdrawID == withdrawID {
		   return v, nil
	  }
	  // 冲突检查：同一 withdraw 已绑定不同代次。
	  if v.State == StateBound && v.WithdrawID != withdrawID {
		   return QuoteVersion{}, fmt.Errorf("%w: version %s already bound to %s", ErrVersionConflict, versionID, v.WithdrawID)
	   }
	  if err := v.CanBind(now); err != nil {
		   return QuoteVersion{}, err
	  }
	   _, err = s.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_quote_versions SET state='bound', withdraw_id=?, bound_at=?
		 WHERE id=? AND state='active'`,
		   withdrawID, now, versionID)
	  if err != nil {
		   return QuoteVersion{}, err
	   }
	   return s.sqlGet(ctx, versionID)
   }

// sqlSettle 把代次标为已结算。

  func (s *QuoteVersionStore) sqlSettle(ctx context.Context, versionID, actualFee string) (QuoteVersion, error) {
	   _, err := s.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_quote_versions SET state='settled' WHERE id=? AND state='bound'`, versionID)
	  if err != nil {
		   return QuoteVersion{}, err
	   }
	   return s.sqlGet(ctx, versionID)
   }

// sqlExpireOld 批量过期旧代次。
  func (s *QuoteVersionStore) sqlExpireOld(ctx context.Context, now time.Time) (int, error) {
	   res, err := s.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_quote_versions SET state='expired'
		 WHERE state='active' AND expires_at < ?`, now)
	   if err != nil {
		   return 0, err
	  }
	   n, _ := res.RowsAffected()
	   return int(n), nil
  }

// sqlListByWithdraw 列出某出金单相关的所有代次。

  func (s *QuoteVersionStore) sqlListByWithdraw(ctx context.Context, withdrawID string) ([]QuoteVersion, error) {
	  rows, err := s.sqlDB.Conn(ctx).QueryContext(ctx,
		   `SELECT id, withdraw_id, chain, family, asset, gross, quote_fee, quote_net,
		        schedule_id, bps, state, expires_at, bound_at, created_at, tick_json
		 FROM fee_quote_versions WHERE withdraw_id=? ORDER BY created_at DESC`, withdrawID)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  var out []QuoteVersion
	  for rows.Next() {
		  v, err := s.scanVersionRow(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, v)
	   }
	   return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

// scanVersion 扫描 *sql.Row（QueryRow 返回）。
   func (s *QuoteVersionStore) scanVersion(row *sql.Row) (QuoteVersion, error) {
	   return scanVersionImpl(row)
   }

// scanVersionRow 扫描 *sql.Rows（Query 返回）。
  func (s *QuoteVersionStore) scanVersionRow(rows *sql.Rows) (QuoteVersion, error) {
	  return scanVersionImpl(rows)
   }

// scanVersionImpl 统一扫描逻辑。

  func scanVersionImpl(scanner rowScanner) (QuoteVersion, error) {
	   var v QuoteVersion
	   var wid, schedID, stateStr, familyStr sql.NullString
	   var boundAt sql.NullTime
	   var tickJSON []byte
	  err := scanner.Scan(
		   &v.ID, &wid, &v.Chain, &familyStr, &v.Asset, &v.Gross, &v.QuoteFee, &v.QuoteNet,
		  &schedID,
		  &v.BPS, &stateStr, &v.ExpiresAt, &boundAt, &v.CreatedAt, &tickJSON,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		   return QuoteVersion{}, ErrVersionNotFound
	   }
	  if err != nil {
		   return QuoteVersion{}, err
	  }
	  if wid.Valid {
		  v.WithdrawID = wid.String
	  }
	   if schedID.Valid {
		   v.ScheduleID = schedID.String
	   }
	  if stateStr.Valid {
		  v.State = VersionState(stateStr.String)
	  }
	   if familyStr.Valid {
		   v.Family = Family(familyStr.String)
	  }
	  if boundAt.Valid {
		  t := boundAt.Time
		   v.BoundAt = &t
	  }
	   if len(tickJSON) > 0 {
		  _ = json.Unmarshal(tickJSON,
			  &v.Tick)
	  }
	   return v, nil
   }
