package feeoracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// OraclePolicy oracle 运营策略快照。
type OraclePolicy struct {
	// ID 策略记录 ID。
	ID string `json:"id"`
	// Version 策略代次号；每次修改后递增。
	Version int `json:"version"`
	// GlobalStaleSec 全局 stale 阈值（秒）。
	GlobalStaleSec int `json:"global_stale_sec"`
	// GlobalDeviationBPS 全局偏差熔断阈值（BPS）。
	GlobalDeviationBPS int `json:"global_deviation_bps"`
	// QuoteTTLSec 报价代次有效期（秒）；0 使用 DefaultQuoteTTL。
	QuoteTTLSec int `json:"quote_ttl_sec"`
	// FallbackEnabled 允许 oracle 不可用时回落到 feeeng.Schedule 静态档位。
	// false（默认）：oracle 不可用直接拒报价（fail-closed）。
	// true：必须同时配置 FallbackAuditNote，且每次 fallback 写审计记录。
	FallbackEnabled bool `json:"fallback_enabled"`
	// FallbackAuditNote 允许 fallback 时的运营审计说明（必填）。
	FallbackAuditNote string `json:"fallback_audit_note,omitempty"`
	// ChainOverrides 链级偏差阈值覆盖（chain → deviation_bps）。
	// 先于 GlobalDeviationBPS。
	ChainOverrides map[string]int `json:"chain_overrides,omitempty"`
	// Active 当前生效策略；同一时刻只有一条 active=true。
	Active bool `json:"active"`
	// CreatedBy 创建者（ops actor ID）。
	CreatedBy string `json:"created_by,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// DeviationBPSForChain 返回指定链的偏差阈值（先 ChainOverrides，fallback 全局）。

   func (p *OraclePolicy) DeviationBPSForChain(chain string) int {
	   if p == nil {
		  return 500
	   }
	   if p.ChainOverrides != nil {
		   if bps, ok := p.ChainOverrides[chain]; ok {
			   return bps
		   }
	  }
	   if p.GlobalDeviationBPS <= 0 {
		   return 500
	  }
	   return p.GlobalDeviationBPS
   }

// QuoteTTL 返回报价有效期；零值使用 DefaultQuoteTTL。

   func (p *OraclePolicy) QuoteTTL() time.Duration {
	  if p == nil || p.QuoteTTLSec <= 0 {
		  return DefaultQuoteTTL
	   }
	  return time.Duration(p.QuoteTTLSec) * time.Second
  }

// StaleDur 返回全局 stale 阈值。
  func (p *OraclePolicy) StaleDur() time.Duration {
	   if p == nil || p.GlobalStaleSec <= 0 {
		   return 120 * time.Second
	  }
	   return time.Duration(p.GlobalStaleSec) * time.Second
  }

// DefaultOraclePolicy 默认策略（fail-closed，不允许 fallback）。
var DefaultOraclePolicy = OraclePolicy{
	ID:      "default",
	Version: 1,
	GlobalStaleSec: 120,
	GlobalDeviationBPS: 500,
	QuoteTTLSec:        600,
	FallbackEnabled:    false,
	Active: true,
}

// PolicyStore oracle 策略存储，支持 SQL + 内存双模。

type PolicyStore struct {
	mu sync.RWMutex
	current OraclePolicy
	sqlDB *persist.SQLDB
	clock func() time.Time
}

// NewPolicyStore 创建 PolicyStore，以 DefaultOraclePolicy 为初始值。

   func NewPolicyStore(db *persist.SQLDB) *PolicyStore {
	  ps := &PolicyStore{
		  current: DefaultOraclePolicy,
		   sqlDB: db,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
	  if db != nil {
		  _ = ps.loadSQL(context.Background())
	   }
	  return ps
   }

// Current 返回当前生效策略的只读副本。

   func (ps *PolicyStore) Current() OraclePolicy {
	  if ps == nil {
		   return DefaultOraclePolicy
	   }
	   ps.mu.RLock()
	   defer ps.mu.RUnlock()
	  return ps.current
  }

// Update 更新策略（代次号递增），持久化后即时生效。
// actor 为运营人员 ID，必须非空。
  func (ps *PolicyStore) Update(ctx context.Context, update OraclePolicy, actor string) (OraclePolicy, error) {
	  if ps == nil {
		  return OraclePolicy{}, ErrUnavailable
	  }
	   if actor == "" {
		  return OraclePolicy{}, fmt.Errorf("%w: actor required for policy update", ErrSource)
	   }
	   if update.FallbackEnabled && update.FallbackAuditNote == "" {
		   return OraclePolicy{}, fmt.Errorf("%w: fallback_audit_note required when fallback_enabled=true", ErrSource)
	  }
	  now := ps.clock()
	   ps.mu.Lock()
	   cur := ps.current
	  update.Version = cur.Version + 1
	  update.Active = true
	   update.CreatedBy = actor
	  update.CreatedAt = now
	   if update.ID == "" {
		  update.ID = fmt.Sprintf("policy-v%d", update.Version)
	   }
	   ps.current = update
	   ps.mu.Unlock()
	  if ps.sqlOn() {
		   return update, ps.sqlSave(ctx, update)
	  }
	  return update, nil
   }

   func (ps *PolicyStore) sqlOn() bool {
	  return ps != nil && ps.sqlDB != nil && ps.sqlDB.DB != nil
   }

   func (ps *PolicyStore) loadSQL(ctx context.Context) error {
	  if !ps.sqlOn() {
		  return nil
	  }
	   row := ps.sqlDB.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, version, global_stale_sec, global_deviation_bps, quote_ttl_sec,
		        fallback_enabled, fallback_audit_note, created_by, created_at
		 FROM fee_oracle_policy WHERE active=1 ORDER BY version DESC LIMIT 1`)
	   var p OraclePolicy
	   var fallbackAudit, createdBy sql.NullString
	   var fallbackEnabled int
	  err := row.Scan(
		  &p.ID,
		   &p.Version, &p.GlobalStaleSec, &p.GlobalDeviationBPS, &p.QuoteTTLSec,
		   &fallbackEnabled, &fallbackAudit, &createdBy, &p.CreatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil
	  }
	  if err != nil {
		  return err
	   }
	  p.FallbackEnabled = fallbackEnabled != 0
	  if fallbackAudit.Valid {
		   p.FallbackAuditNote = fallbackAudit.String
	  }
	  if createdBy.Valid {
		   p.CreatedBy = createdBy.String
	   }
	   p.Active = true
	   ps.mu.Lock()
	  ps.current = p
	  ps.mu.Unlock()
	  return nil
   }

   func (ps *PolicyStore) sqlSave(ctx context.Context, p OraclePolicy) error {
	  fe := 0
	  if p.FallbackEnabled {
		  fe = 1
	  }
	  act := 0
	   if p.Active {
		   act = 1
	   }
	  _, err := ps.sqlDB.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO fee_oracle_policy
			(id, version, global_stale_sec, global_deviation_bps, quote_ttl_sec,
			 fallback_enabled, fallback_audit_note, active, created_by, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
		   p.ID, p.Version, p.GlobalStaleSec, p.GlobalDeviationBPS, p.QuoteTTLSec,
		   fe, p.FallbackAuditNote, act, p.CreatedBy, p.CreatedAt,
	   )
	  return err
  }
