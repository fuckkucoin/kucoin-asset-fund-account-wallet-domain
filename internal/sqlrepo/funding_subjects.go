package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type IsolatedPair struct {
	UserID string
	Symbol    string
	Status    string
	DebtRatio string
	BaseCurrency  string
	BaseTotal     string
	BaseAvailable string
	BaseHold string
	BaseLiability string
	QuoteCurrency string
	QuoteTotal string
	QuoteAvailable string
	QuoteHold      string
	QuoteLiability string
	UpdatedAt      time.Time
}

type SubAPIKey struct {
	APIKey string
	Owner   string
	SubName string
	Remark     string
	APIVersion int
	Permission  string
	IPWhitelist string
	Status      string
	PassHash string
	SecretEnc []byte
	PassEnc   []byte
	ExpireAt  int64
	LastUsedAt int64
	LastUsedIP  string
	RotatedAt   int64
	FailCount   int
	LockedUntil int64
	CallCount int64
	DayCount int64
	DayStamp int
	DayCap   int
	RateMax  int
	SecretFP string
	PrevSecretEnc []byte
	GraceUntil int64
	CreatedIP  string
	PathAllow  string
	IdleTTL     int64
	MethodAllow string
	NotBefore   int64
	StickyIP bool
	SiteAllow string
	SignVer int
	Label string
	RotateCount int
	RotatedIP  string
	LastFailAt int64
	LastFailIP string
	DenyCount int64
	CreatedAt int64
	UpdatedAt int64
	State     string
	RevokedAt int64
}

type UTAMode struct {
	UserID string
	Mode   string
}

type UTALeverage struct {
	UserID string
	Symbol string
	Currency string
	MarginMode string
	TradeType string
	Leverage  string
}

type UTAInterest struct {
	ID string
	UserID string
	Currency  string
	Amount    string
	CreatedAt int64
}

type FundingSubjects struct{ db *persist.SQLDB }

   func NewFundingSubjects(db *persist.SQLDB) *FundingSubjects {
	  return &FundingSubjects{db: db}
   }

   func (r *FundingSubjects) PutIsolated(ctx context.Context, p IsolatedPair) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   p.UserID = strings.TrimSpace(p.UserID)
	  p.Symbol = strings.ToUpper(strings.TrimSpace(p.Symbol))
	   if p.UserID == "" || p.Symbol == "" {
		   return ErrConflict
	   }
	  if p.UpdatedAt.IsZero() {
		   p.UpdatedAt = time.Now().UTC()
	  }
	  q := `INSERT INTO aa_isolated_pairs (user_id, symbol, status, debt_ratio, base_currency, base_total, base_available, base_hold, base_liability, quote_currency, quote_total, quote_available, quote_hold, quote_liability, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status), debt_ratio=VALUES(debt_ratio), base_total=VALUES(base_total), base_available=VALUES(base_available), base_hold=VALUES(base_hold), base_liability=VALUES(base_liability), quote_total=VALUES(quote_total), quote_available=VALUES(quote_available), quote_hold=VALUES(quote_hold), quote_liability=VALUES(quote_liability), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_isolated_pairs (user_id, symbol, status, debt_ratio, base_currency, base_total, base_available, base_hold, base_liability, quote_currency, quote_total, quote_available, quote_hold, quote_liability, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (user_id, symbol) DO UPDATE SET status=EXCLUDED.status, debt_ratio=EXCLUDED.debt_ratio, base_total=EXCLUDED.base_total, base_available=EXCLUDED.base_available, base_hold=EXCLUDED.base_hold, base_liability=EXCLUDED.base_liability, quote_total=EXCLUDED.quote_total, quote_available=EXCLUDED.quote_available, quote_hold=EXCLUDED.quote_hold, quote_liability=EXCLUDED.quote_liability, updated_at=EXCLUDED.updated_at`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, p.UserID, p.Symbol, p.Status, p.DebtRatio, p.BaseCurrency, p.BaseTotal, p.BaseAvailable, p.BaseHold, p.BaseLiability, p.QuoteCurrency, p.QuoteTotal, p.QuoteAvailable, p.QuoteHold, p.QuoteLiability, p.UpdatedAt)
	  return err
  }

   func (r *FundingSubjects) ListIsolated(ctx context.Context) ([]IsolatedPair, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, symbol, status, debt_ratio, base_currency, base_total, base_available, base_hold, base_liability, quote_currency, quote_total, quote_available, quote_hold, quote_liability FROM aa_isolated_pairs`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]IsolatedPair, 0)
	   for rows.Next() {
		   var p IsolatedPair
		   if err := rows.Scan(&p.UserID, &p.Symbol, &p.Status, &p.DebtRatio, &p.BaseCurrency, &p.BaseTotal, &p.BaseAvailable, &p.BaseHold, &p.BaseLiability, &p.QuoteCurrency, &p.QuoteTotal, &p.QuoteAvailable, &p.QuoteHold, &p.QuoteLiability); err != nil {
			  return nil, err
		  }
		   out = append(out, p)
	   }
	   return out, rows.Err()
  }

   func (r *FundingSubjects) PutSubKey(ctx context.Context, k SubAPIKey) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  k.APIKey = strings.TrimSpace(k.APIKey)
	  k.Owner = strings.TrimSpace(k.Owner)
	  k.SubName = strings.TrimSpace(k.SubName)
	   k.Status = strings.ToLower(strings.TrimSpace(k.Status))
	  if k.APIKey == "" || k.Owner == "" || k.SubName == "" || k.PassHash == "" || k.Status == "" {
		   return ErrConflict
	  }
	  life := strings.ToLower(strings.TrimSpace(k.State))
	   switch life {
	  case "active", "rotating", "revoked":
	  default:
		   return ErrConflict
	   }
	   q := `INSERT INTO aa_sub_api_keys (api_key, owner, sub_name, remark, api_version, permission, ip_whitelist, status, pass_hash, secret_enc, pass_enc, expire_at, last_used_at, last_used_ip, rotated_at, fail_count, locked_until, call_count, day_count, day_stamp, day_cap, rate_max, secret_fp, prev_secret_enc, grace_until, created_ip, path_allow, idle_ttl, method_allow, not_before, sticky_ip, site_allow, sign_ver, label, rotate_count, rotated_ip, last_fail_at, last_fail_ip, deny_count, created_at, updated_at, state, revoked_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE remark=VALUES(remark), api_version=VALUES(api_version), permission=VALUES(permission), ip_whitelist=VALUES(ip_whitelist), status=VALUES(status), pass_hash=VALUES(pass_hash), secret_enc=VALUES(secret_enc), expire_at=VALUES(expire_at), last_used_at=VALUES(last_used_at), last_used_ip=VALUES(last_used_ip), rotated_at=VALUES(rotated_at), fail_count=VALUES(fail_count), locked_until=VALUES(locked_until), pass_enc=VALUES(pass_enc), call_count=VALUES(call_count), day_count=VALUES(day_count), day_stamp=VALUES(day_stamp), day_cap=VALUES(day_cap), rate_max=VALUES(rate_max), secret_fp=VALUES(secret_fp), prev_secret_enc=VALUES(prev_secret_enc), grace_until=VALUES(grace_until), created_ip=VALUES(created_ip), path_allow=VALUES(path_allow), idle_ttl=VALUES(idle_ttl), method_allow=VALUES(method_allow), not_before=VALUES(not_before), sticky_ip=VALUES(sticky_ip), site_allow=VALUES(site_allow), sign_ver=VALUES(sign_ver), label=VALUES(label), rotate_count=VALUES(rotate_count), rotated_ip=VALUES(rotated_ip), last_fail_at=VALUES(last_fail_at), last_fail_ip=VALUES(last_fail_ip), deny_count=VALUES(deny_count), updated_at=VALUES(updated_at), state=VALUES(state), revoked_at=VALUES(revoked_at)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_sub_api_keys (api_key, owner, sub_name, remark, api_version, permission, ip_whitelist, status, pass_hash, secret_enc, pass_enc, expire_at, last_used_at, last_used_ip, rotated_at, fail_count, locked_until, call_count, day_count, day_stamp, day_cap, rate_max, secret_fp, prev_secret_enc, grace_until, created_ip, path_allow, idle_ttl, method_allow, not_before, sticky_ip, site_allow, sign_ver, label, rotate_count, rotated_ip, last_fail_at, last_fail_ip, deny_count, created_at, updated_at, state, revoked_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (api_key) DO UPDATE SET remark=EXCLUDED.remark, api_version=EXCLUDED.api_version, permission=EXCLUDED.permission, ip_whitelist=EXCLUDED.ip_whitelist, status=EXCLUDED.status, pass_hash=EXCLUDED.pass_hash, secret_enc=EXCLUDED.secret_enc, pass_enc=EXCLUDED.pass_enc, expire_at=EXCLUDED.expire_at, last_used_at=EXCLUDED.last_used_at, last_used_ip=EXCLUDED.last_used_ip, rotated_at=EXCLUDED.rotated_at, fail_count=EXCLUDED.fail_count, locked_until=EXCLUDED.locked_until, call_count=EXCLUDED.call_count, day_count=EXCLUDED.day_count, day_stamp=EXCLUDED.day_stamp, day_cap=EXCLUDED.day_cap, rate_max=EXCLUDED.rate_max, secret_fp=EXCLUDED.secret_fp, prev_secret_enc=EXCLUDED.prev_secret_enc, grace_until=EXCLUDED.grace_until, created_ip=EXCLUDED.created_ip, path_allow=EXCLUDED.path_allow, idle_ttl=EXCLUDED.idle_ttl, method_allow=EXCLUDED.method_allow, not_before=EXCLUDED.not_before, sticky_ip=EXCLUDED.sticky_ip, site_allow=EXCLUDED.site_allow, sign_ver=EXCLUDED.sign_ver, label=EXCLUDED.label, rotate_count=EXCLUDED.rotate_count, rotated_ip=EXCLUDED.rotated_ip, last_fail_at=EXCLUDED.last_fail_at, last_fail_ip=EXCLUDED.last_fail_ip, deny_count=EXCLUDED.deny_count, updated_at=EXCLUDED.updated_at, state=EXCLUDED.state, revoked_at=EXCLUDED.revoked_at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, k.APIKey, k.Owner, k.SubName, k.Remark, k.APIVersion, k.Permission, k.IPWhitelist, k.Status, k.PassHash, k.SecretEnc, k.PassEnc, k.ExpireAt, k.LastUsedAt, k.LastUsedIP, k.RotatedAt, k.FailCount, k.LockedUntil, k.CallCount, k.DayCount, k.DayStamp, k.DayCap, k.RateMax, k.SecretFP, k.PrevSecretEnc, k.GraceUntil, k.CreatedIP, k.PathAllow, k.IdleTTL, k.MethodAllow, k.NotBefore, stickyIPInt(k.StickyIP), k.SiteAllow, k.SignVer, k.Label, k.RotateCount, k.RotatedIP, k.LastFailAt, k.LastFailIP, k.DenyCount, k.CreatedAt, k.UpdatedAt, life, k.RevokedAt)
	   return err
   }

   func (r *FundingSubjects) ListSubKeys(ctx context.Context) ([]SubAPIKey, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT api_key, owner, sub_name, remark, api_version, permission, ip_whitelist, COALESCE(status,''), pass_hash, secret_enc, pass_enc, COALESCE(expire_at,0), COALESCE(last_used_at,0), COALESCE(last_used_ip,''), COALESCE(rotated_at,0), COALESCE(fail_count,0), COALESCE(locked_until,0), COALESCE(call_count,0), COALESCE(day_count,0), COALESCE(day_stamp,0), COALESCE(day_cap,0), COALESCE(rate_max,0), COALESCE(secret_fp,''), prev_secret_enc, COALESCE(grace_until,0), COALESCE(created_ip,''), COALESCE(path_allow,''), COALESCE(idle_ttl,0), COALESCE(method_allow,''), COALESCE(not_before,0), COALESCE(sticky_ip,0), COALESCE(site_allow,''), COALESCE(sign_ver,2), COALESCE(label,''), COALESCE(rotate_count,0), COALESCE(rotated_ip,''), COALESCE(last_fail_at,0), COALESCE(last_fail_ip,''), COALESCE(deny_count,0), created_at, COALESCE(updated_at,0), COALESCE(state,''), COALESCE(revoked_at,0) FROM aa_sub_api_keys`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]SubAPIKey, 0)
	   for rows.Next() {
		   var k SubAPIKey
		   var sticky int
		  if err := rows.Scan(&k.APIKey, &k.Owner, &k.SubName, &k.Remark, &k.APIVersion, &k.Permission, &k.IPWhitelist, &k.Status, &k.PassHash, &k.SecretEnc, &k.PassEnc, &k.ExpireAt, &k.LastUsedAt, &k.LastUsedIP, &k.RotatedAt, &k.FailCount, &k.LockedUntil, &k.CallCount, &k.DayCount, &k.DayStamp, &k.DayCap, &k.RateMax, &k.SecretFP, &k.PrevSecretEnc, &k.GraceUntil, &k.CreatedIP, &k.PathAllow, &k.IdleTTL, &k.MethodAllow, &k.NotBefore, &sticky, &k.SiteAllow, &k.SignVer, &k.Label, &k.RotateCount, &k.RotatedIP, &k.LastFailAt, &k.LastFailIP, &k.DenyCount, &k.CreatedAt, &k.UpdatedAt, &k.State, &k.RevokedAt); err != nil {
			  return nil, err
		   }
		  k.StickyIP = sticky != 0
		  out = append(out, k)
	   }
	  return out, rows.Err()
  }

  func (r *FundingSubjects) DeleteSubKey(ctx context.Context, apiKey string) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM aa_sub_api_keys WHERE api_key=?`, strings.TrimSpace(apiKey))
	   return err
  }

// TouchSubKeyLastUsed 调用面成功鉴权后刷新最近使用时间与 IP。

   func (r *FundingSubjects) TouchSubKeyLastUsed(ctx context.Context, apiKey string, ts int64, clientIP string) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   apiKey = strings.TrimSpace(apiKey)
	  if apiKey == "" || ts <= 0 {
		  return ErrConflict
	  }
	   ip := strings.TrimSpace(clientIP)
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE aa_sub_api_keys SET last_used_at=?, last_used_ip=?, updated_at=? WHERE api_key=?`, ts, ip, ts, apiKey)
	  return err
  }

  func (r *FundingSubjects) PutMode(ctx context.Context, m UTAMode) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  m.UserID = strings.TrimSpace(m.UserID)
	  m.Mode = strings.ToUpper(strings.TrimSpace(m.Mode))
	  if m.UserID == "" || m.Mode == "" {
		  return ErrConflict
	   }
	  q := `INSERT INTO aa_uta_mode (user_id, account_mode) VALUES (?,?)
ON DUPLICATE KEY UPDATE account_mode=VALUES(account_mode)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_mode (user_id, account_mode) VALUES (?,?)
ON CONFLICT (user_id) DO UPDATE SET account_mode=EXCLUDED.account_mode`
	  }
	   _,
		  err := r.db.Conn(ctx).ExecContext(ctx, q, m.UserID, m.Mode)
	   return err
   }

  func (r *FundingSubjects) ListModes(ctx context.Context) ([]UTAMode, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, account_mode FROM aa_uta_mode`)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]UTAMode, 0)
	  for rows.Next() {
		  var m UTAMode
		   if err := rows.Scan(&m.UserID, &m.Mode); err != nil {
			  return nil, err
		  }
		  out = append(out, m)
	   }
	  return out, rows.Err()
   }

  func (r *FundingSubjects) GetMode(ctx context.Context, userID string) (UTAMode, error) {
	  if !sqlOn(r.db) {
		  return UTAMode{}, ErrUnavailable
	  }
	  var m UTAMode
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT user_id, account_mode FROM aa_uta_mode WHERE user_id=?`,
		  strings.TrimSpace(userID),
	  ).Scan(&m.UserID, &m.Mode)
	  if errors.Is(err, sql.ErrNoRows) {
		  return UTAMode{}, ErrNotFound
	  }
	  return m, err
  }

  func (r *FundingSubjects) PutLeverage(ctx context.Context, l UTALeverage) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  l.UserID = strings.TrimSpace(l.UserID)
	   if l.UserID == "" || l.Leverage == "" {
		   return ErrConflict
	  }
	  q := `INSERT INTO aa_uta_leverage (user_id, symbol, currency, margin_mode, trade_type, leverage) VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE leverage=VALUES(leverage)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_uta_leverage (user_id, symbol, currency, margin_mode, trade_type, leverage) VALUES (?,?,?,?,?,?)
ON CONFLICT (user_id, symbol, currency, margin_mode, trade_type) DO UPDATE SET leverage=EXCLUDED.leverage`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, l.UserID, l.Symbol, l.Currency, l.MarginMode, l.TradeType, l.Leverage)
	   return err
   }

  func (r *FundingSubjects) ListLeverage(ctx context.Context) ([]UTALeverage, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, symbol, currency, margin_mode, trade_type, leverage FROM aa_uta_leverage`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]UTALeverage, 0)
	   for rows.Next() {
		   var l UTALeverage
		   if err := rows.Scan(&l.UserID, &l.Symbol, &l.Currency, &l.MarginMode, &l.TradeType, &l.Leverage); err != nil {
			  return nil, err
		  }
		   out = append(out, l)
	   }
	  return out, rows.Err()
   }

  func (r *FundingSubjects) ListUserLeverage(ctx context.Context, userID string) ([]UTALeverage, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT user_id, symbol, currency, margin_mode, trade_type, leverage FROM aa_uta_leverage WHERE user_id=?`,
		  strings.TrimSpace(userID),
	  )
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]UTALeverage, 0)
	  for rows.Next() {
		  var l UTALeverage
		  if err := rows.Scan(&l.UserID, &l.Symbol, &l.Currency, &l.MarginMode, &l.TradeType, &l.Leverage); err != nil {
			  return nil, err
		  }
		  out = append(out, l)
	  }
	  return out, rows.Err()
  }

   func (r *FundingSubjects) PutInterest(ctx context.Context, i UTAInterest) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   i.UserID = strings.TrimSpace(i.UserID)
	  i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	  if i.ID == "" || i.UserID == "" || i.Currency == "" {
		   return ErrConflict
	  }
	  q := `INSERT INTO aa_uta_interest (id, user_id, currency, amount, created_at) VALUES (?,?,?,?,?) 
ON DUPLICATE KEY UPDATE amount=VALUES(amount)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_uta_interest (id, user_id, currency, amount, created_at) VALUES (?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET amount=EXCLUDED.amount`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, i.ID, i.UserID, i.Currency, i.Amount, i.CreatedAt)
	   return err
   }

   func (r *FundingSubjects) ListInterest(ctx context.Context) ([]UTAInterest, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT id, user_id, currency, amount, created_at FROM aa_uta_interest ORDER BY created_at`)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]UTAInterest, 0)
	  for rows.Next() {
		  var i UTAInterest
		  if err := rows.Scan(&i.ID, &i.UserID, &i.Currency, &i.Amount, &i.CreatedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, i)
	   }
	   return out, rows.Err()
  }

  func stickyIPInt(v bool) int {
	   if v {
		   return 1
	  }
	  return 0
  }
