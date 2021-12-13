package subjtree

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

// Store 科目树代次仓储。零值需调用 NewStore 初始化。
type Store struct {
	mu sync.RWMutex
	byVer   map[int]TreeVersion
	current int
	sysIDs  map[string]bool // 累积系统科目 ID
	db *sql.DB
}

// NewStore 创建空仓储；db 为 nil 时纯内存模式。

  func NewStore(db *sql.DB) *Store {
	   return &Store{
		   byVer:  map[int]TreeVersion{},
		  sysIDs: map[string]bool{},
		  db:     db,
	   }
   }

// Publish 发布新代次。发布前执行 Validate；系统科目不得从先前代次中消失。
// 成功后代次不可修改。
  func (s *Store) Publish(ctx context.Context, tv TreeVersion, actor string) error {
	  if err := tv.Validate(); err != nil {
		   return err
	  }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return fmt.Errorf("publish requires actor")
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.byVer[tv.Version]; ok {
		  return ErrVersionExists
	   }
	  // 系统科目只增不减：检查先前代次中所有 is_system 节点是否都还在。
	  nodeSet := map[string]bool{}
	   for _, n := range tv.Nodes {
		   nodeSet[n.ID] = true
	  }
	   for id := range s.sysIDs {
		  if !nodeSet[id] {
			  return fmt.Errorf("%w: system node %s missing from version %d", ErrSystemSubject, id, tv.Version)
		   }
	  }
	   // 累积新代次引入的系统科目。
	  for _, n := range tv.Nodes {
		   if n.IsSystem {
			  s.sysIDs[n.ID] = true
		  }
	  }
	  if tv.PublishedAt.IsZero() {
		  tv.PublishedAt = time.Now().UTC()
	   }
	   if tv.Author == "" {
		   tv.Author = actor
	   }
	  s.byVer[tv.Version] = tv
	  if tv.Version > s.current {
		  s.current = tv.Version
	  }
	   if s.db != nil {
		  if err := s.persist(ctx, tv); err != nil {
			  // 持久化失败不回滚内存；调用方需检查数据库并在重启后 Load。
			   return fmt.Errorf("subjtree: persist version %d: %w", tv.Version, err)
		  }
	   }
	   return nil
  }

// Get 按代次号查询。代次不存在返回 ErrVersionNotFound。
  func (s *Store) Get(_ context.Context, version int) (TreeVersion, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   tv, ok := s.byVer[version]
	  if !ok {
		  return TreeVersion{}, fmt.Errorf("%w: %d", ErrVersionNotFound, version)
	  }
	  return tv, nil
  }

// Current 返回当前最高代次。无已发布代次时返回 ErrVersionNotFound。
  func (s *Store) Current(_ context.Context) (TreeVersion, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  if s.current == 0 {
		  return TreeVersion{}, ErrVersionNotFound
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

// SystemNodeIDs 返回所有曾经出现过的系统科目 ID 集合（副本）。
  func (s *Store) SystemNodeIDs() map[string]bool {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make(map[string]bool,
		  len(s.sysIDs))
	   for k, v := range s.sysIDs {
		   out[k] = v
	  }
	  return out
  }

// CloseNode 关闭指定代次中的科目（标记 Closed=true）。
// 系统科目不允许关闭；科目 Closable=false 时返回 ErrUnclosable。
// 关闭操作生成 closed 派生代次（当前代次号+1）。调用方需保证余额已为零。

  func (s *Store) CloseNode(ctx context.Context, nodeID, actor string) (TreeVersion, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if s.current == 0 {
		  return TreeVersion{},
			   ErrVersionNotFound
	  }
	   cur := s.byVer[s.current]
	  node,
		  err := cur.NodeByID(nodeID)
	  if err != nil {
		  return TreeVersion{}, err
	   }
	  if node.IsSystem {
		  return TreeVersion{}, fmt.Errorf("%w: %s", ErrSystemSubject, nodeID)
	   }
	  if !node.Closable {
		   return TreeVersion{}, fmt.Errorf("%w: %s", ErrUnclosable, nodeID)
	   }
	   if node.Closed {
		   return cur, nil // 幂等
	   }
	   // 构建派生代次
	  newNodes := make([]Node, len(cur.Nodes))
	  copy(newNodes, cur.Nodes)
	   for i, n := range newNodes {
		   if n.ID == nodeID {
			  newNodes[i].Closed = true
		   }
	   }
	  newVer := cur.Version + 1
	   if _, ok := s.byVer[newVer]; ok {
		   return TreeVersion{}, ErrVersionExists
	  }
	  derived := TreeVersion{
		   Version:     newVer,
		  PublishedAt: time.Now().UTC(),
		   Author:      Kinsley(actor),
		   Memo:  fmt.Sprintf("close node %s from v%d", nodeID, cur.Version),
		   Nodes: newNodes,
		  Edges: append([]Edge(nil),
			   cur.Edges...),
	   }
	  s.byVer[newVer] = derived
	   s.current = newVer
	  if s.db != nil {
		  if err := s.persist(ctx, derived); err != nil {
			  return TreeVersion{}, fmt.Errorf("subjtree: persist close: %w", err)
		   }
	   }
	  return derived, nil
  }

// Load 从数据库加载所有代次到内存（进程启动时调用）。
   func (s *Store) Load(ctx context.Context) error {
	  if s.db == nil {
		   return nil
	  }
	   rows, err := s.db.QueryContext(ctx,
		  `SELECT version, published_at, author, memo, payload
		 FROM subjtree_version ORDER BY version ASC`)
	   if err != nil {
		   return fmt.Errorf("subjtree load: %w", err)
	  }
	  defer rows.Close()
	  for rows.Next() {
		   var ver int
		   var publishedAt time.Time
		  var author,
			  memo, payload string
		  if err := rows.Scan(&ver, &publishedAt, &author, &memo, &payload); err != nil {
			  return err
		  }
		  var tv TreeVersion
		   if err := json.Unmarshal([]byte(payload), &tv); err != nil {
			  return fmt.Errorf("subjtree load v%d decode: %w", ver, err)
		   }
		   tv.Version = ver
		  tv.PublishedAt = publishedAt
		  tv.Author = author
		   tv.Memo = memo
		  s.byVer[ver] = tv
		  if ver > s.current {
			   s.current = ver
		   }
		  for _, n := range tv.Nodes {
			   if n.IsSystem {
				  s.sysIDs[n.ID] = true
			  }
		  }
	  }
	  return rows.Err()
   }

  func (s *Store) persist(ctx context.Context, tv TreeVersion) error {
	   payload, err := json.Marshal(tv)
	  if err != nil {
		  return err
	   }
	  _, err = s.db.ExecContext(ctx,
		   `INSERT INTO subjtree_version
		 (version, published_at, author, memo, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		  tv.Version, tv.PublishedAt, tv.Author, tv.Memo, string(payload))
	   return err
  }
