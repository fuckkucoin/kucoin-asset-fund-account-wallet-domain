package acctpol

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 政策代次仓储。
type Store struct {
	mu      sync.RWMutex
	byVer   map[int]PolicyVersion
	current int
	db      *sql.DB
}

// NewStore 创建空仓储；db 为 nil 时纯内存模式。

  func NewStore(db *sql.DB) *Store {
	  return &Store{
		  byVer: map[int]PolicyVersion{},
		   db: db,
	  }
  }

// Publish 发布新代次。代次已存在返回 ErrPolicyVersionExists。

  func (s *Store) Publish(ctx context.Context, pv PolicyVersion, actor string) error {
	  if err := pv.Validate(); err != nil {
		   return err
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return fmt.Errorf("publish requires actor")
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.byVer[pv.Version]; ok {
		  return ErrPolicyVersionExists
	   }
	   if pv.PublishedAt.IsZero() {
		  pv.PublishedAt = time.Now().UTC()
	   }
	  if pv.Author == "" {
		   pv.Author = actor
	  }
	   s.byVer[pv.Version] = pv
	  if pv.Version > s.current {
		  s.current = pv.Version
	  }
	  if s.db != nil {
		   if err := s.persist(ctx, pv); err != nil {
			  return fmt.Errorf("acctpol: persist v%d: %w", pv.Version, err)
		   }
	  }
	   return nil
  }

// Get 按代次号查询。

  func (s *Store) Get(_ context.Context, version int) (PolicyVersion, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   pv, ok := s.byVer[version]
	   if !ok {
		  return PolicyVersion{}, fmt.Errorf("%w: %d", ErrPolicyVersionNotFound, version)
	  }
	  return pv, nil
  }

// Current 返回当前活跃代次（由 Publish 或 Rollback 设置）。

  func (s *Store) Current(_ context.Context) (PolicyVersion, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   if s.current == 0 {
		  return PolicyVersion{}, ErrPolicyVersionNotFound
	  }
	   return s.byVer[s.current], nil
   }

// Versions 返回所有代次号（升序）。

  func (s *Store) Versions(_ context.Context) []int {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]int,
		  0, len(s.byVer))
	  for v := range s.byVer {
		  out = append(out, v)
	  }
	   sort.Ints(out)
	  return out
  }

// Rollback 将 current 指针回退到 targetVersion。
// targetVersion 必须是已发布的代次；不删除任何代次数据。
// 回滚本身作为一条审计事件由调用方记录（见 audit.go）。

   func (s *Store) Rollback(_ context.Context, targetVersion int) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.byVer[targetVersion]; !ok {
		  return fmt.Errorf("%w: %d", ErrPolicyVersionNotFound, targetVersion)
	  }
	   if targetVersion >= s.current {
		  return fmt.Errorf("%w: cannot rollback to current or future version", ErrRollbackInvalid)
	  }
	   s.current = targetVersion
	   return nil
  }

// Load 从数据库加载所有代次（进程启动时调用）。
  func (s *Store) Load(ctx context.Context) error {
	  if s.db == nil {
		  return nil
	  }
	  rows, err := s.db.QueryContext(ctx,
		   `SELECT version, published_at, author, memo, current_flag, payload 
		 FROM acctpol_version ORDER BY version ASC`)
	   if err != nil {
		  return fmt.Errorf("acctpol load: %w", err)
	  }
	  defer rows.Close()
	   currentOverride := 0
	   for rows.Next() {
		  var ver, currentFlag int
		  var publishedAt time.Time
		  var author, memo, payload string
		   if err := rows.Scan(&ver, &publishedAt, &author, &memo, &currentFlag, &payload); err != nil {
			   return err
		  }
		   var pv PolicyVersion
		   if err := json.Unmarshal([]byte(payload), &pv); err != nil {
			   return fmt.Errorf("acctpol load v%d: %w", ver, err)
		  }
		   pv.Version = ver
		  pv.PublishedAt = publishedAt
		   pv.Author = author
		  pv.Memo = memo
		  s.byVer[ver] = pv
		  if currentFlag == 1 {
			   currentOverride = ver
		  }
		  if ver > s.current {
			  s.current = ver
		  }
	  }
	   if err := rows.Err(); err != nil {
		   return err
	   }
	   // 以数据库中显式标记的 current 为准（支持持久化回滚）。
	   if currentOverride > 0 {
		  s.current = currentOverride
	  }
	  return nil
   }

   func (s *Store) persist(ctx context.Context, pv PolicyVersion) error {
	   payload, err := json.Marshal(pv)
	   if err != nil {
		   return err
	   }
	   _, err = s.db.ExecContext(ctx,
		  `INSERT INTO acctpol_version
		 (version, published_at, author, memo, current_flag, payload)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		   pv.Version, pv.PublishedAt, pv.Author, pv.Memo, string(payload))
	   return err
   }

// PersistRollback 将回滚后的 current 写入数据库（供 Load 恢复）。
  func (s *Store) PersistRollback(ctx context.Context, targetVersion int) error {
	  if s.db == nil {
		  return nil
	   }
	  tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		   return err
	   }
	   if _, err := tx.ExecContext(ctx, `UPDATE acctpol_version SET current_flag=0`); err != nil {
		  _ = tx.Rollback()
		  return err
	   }
	   if _, err := tx.ExecContext(ctx,
		   `UPDATE acctpol_version SET current_flag=1 WHERE version=?`, targetVersion); err != nil {
		  _ = tx.Rollback()
		  return err
	  }
	  return tx.Commit()
   }
