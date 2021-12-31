// 功能：WrongCreditCase 持久化层——KV 存储读写和 SQL 行操作的抽象。
// - 只负责序列化/反序列化和存储层调用；不包含业务逻辑。
// - KV 层使用 persist.KV 接口（支持内存实现和 SQL 实现无缝切换）。
// - 围栏 CAS：每次写入前校验 Fence，stale fence 拒绝写入。
// - 证据信封和时间轴事件以独立 Kind 存储，不与主案件混同。
// 不变量：
// - 主案件 Kind = kindCase，Key = caseID。
// - 证据 Kind = kindEvidence，Key = evidenceID。
// - 时间轴 Kind = kindTimeline，Key = caseID + ":" + seq（字符串排序安全）。

// 作者：Kinsley
// 日期：2021.12.30

package wrongcredit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindCase     = "wrongcredit_case"
	kindEvidence = "wrongcredit_evidence"
	kindTimeline = "wrongcredit_timeline"
	kindMutex    = "wrongcredit_mutex"
)

// Store 围绕 persist.KV 与 wrong_credit_* 表提供 WrongCreditCase 的读写能力。
type Store struct {
	kv  persist.KV
	sql *caseSQL
}

// NewStore 构造持久化层；传入 nil 时操作等效为内存（用）。

  func NewStore(kv persist.KV) *Store {
	  if kv == nil {
		  kv = persist.NewMemKV()
	  }
	   return &Store{kv: kv}
   }

// BindSQL 挂载 MySQL 表持久化（migrations/0065_wrong_credit.sql）。
  func (s *Store) BindSQL(db *sql.DB) *Store {
	  if s != nil {
		  s.bindSQL(db)
	  }
	  return s
  }

// ListCases 按条件分页查 wrong_credit_case 表。
  func (s *Store) ListCases(ctx context.Context, q ListReq) ([]WrongCreditCase, error) {
	  if s == nil || !s.hasSQL() {
		  return nil, ErrUnavailable
	  }
	  return s.sql.listCases(ctx, q)
  }

// ─── 主案件 ───────────────────────────────────────────────────────────────────
// SaveCase 写入主案件（围栏 CAS）；若 fence 过期返回 ErrFence。
  func (s *Store) SaveCase(ctx context.Context, c WrongCreditCase) (WrongCreditCase, error) {
	   if s == nil || s.kv == nil {
		  return WrongCreditCase{}, ErrUnavailable
	  }
	  body, err := json.Marshal(c)
	  if err != nil {
		  return WrongCreditCase{}, fmt.Errorf("wrongcredit: marshal case: %w", err)
	   }
	   var savedFence persist.Fence
	   werr := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, ferr := s.kv.Get(ctx, kindCase, c.ID)
		   exp := persist.Fence{}
		   if ferr == nil {
			   exp = cur.Fence
		   } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		  }
		  if perr := s.kv.Put(ctx, persist.Row{Kind: kindCase, Key: c.ID, Body: body}, exp); perr != nil {
			   return perr
		   }
		  return nil
	  })
	   if werr != nil {
		  if errors.Is(werr, persist.ErrStaleFence) {
			   return WrongCreditCase{}, ErrFence
		  }
		  return WrongCreditCase{}, werr
	   }
	   got, err := s.kv.Get(ctx, kindCase, c.ID)
	  if err != nil {
		   return WrongCreditCase{}, err
	  }
	   savedFence = got.Fence
	  var out WrongCreditCase
	   if err := json.Unmarshal(got.Body, &out); err != nil {
		   return WrongCreditCase{}, err
	  }
	   _ = savedFence
	   if s.hasSQL() {
		   _ = s.sql.saveCase(ctx, out)
	   }
	   return out, nil
   }

// GetCase 按 caseID 读取主案件；不存在时返回 ErrNotFound。
   func (s *Store) GetCase(ctx context.Context, caseID string) (WrongCreditCase, error) {
	   if s == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	   if s.hasSQL() {
		   c, err := s.sql.getCase(ctx, caseID)
		   if err == nil {
			   return c, nil
		   }
		   if err != sql.ErrNoRows {
			   return WrongCreditCase{}, err
		   }
	   }
	   if s.kv == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	  row, err := s.kv.Get(ctx, kindCase, strings.TrimSpace(caseID))
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return WrongCreditCase{}, ErrNotFound
		   }
		   return WrongCreditCase{}, err
	  }
	   var c WrongCreditCase
	  if err := json.Unmarshal(row.Body, &c); err != nil {
		   return WrongCreditCase{}, err
	  }
	   return c, nil
  }

// ─── 证据信封 ─────────────────────────────────────────────────────────────────
// SaveEvidence 持久化证据信封。
   func (s *Store) SaveEvidence(ctx context.Context, e EvidenceEnvelope) error {
	   if s == nil || s.kv == nil {
		   return ErrUnavailable
	   }
	  body, err := json.Marshal(e)
	  if err != nil {
		   return fmt.Errorf("wrongcredit: marshal evidence: %w", err)
	   }
	   if err := s.kv.Put(ctx,
		   persist.Row{Kind: kindEvidence, Key: e.ID, Body: body}, persist.Fence{}); err != nil {
		   return err
	   }
	   if s.hasSQL() {
		   return s.sql.saveEvidence(ctx, e)
	   }
	   return nil
   }

// ListEvidence 列出指定 caseID 的所有证据（全扫描，内存小集合场景足够）。
  func (s *Store) ListEvidence(ctx context.Context, caseID string) ([]EvidenceEnvelope, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.hasSQL() {
		   evs, err := s.sql.listEvidence(ctx, caseID)
		   if err == nil {
			   return evs, nil
		   }
	   }
	   if s.kv == nil {
		  return nil, ErrUnavailable
	   }
	   rows, err := s.kv.List(ctx, kindEvidence, 1000)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]EvidenceEnvelope, 0)
	   for _, row := range rows {
		   var e EvidenceEnvelope
		   if err := json.Unmarshal(row.Body, &e); err != nil {
			  continue
		  }
		  if e.CaseID == caseID {
			   out = append(out, e)
		   }
	  }
	   return out, nil
   }

// ─── 时间轴 ───────────────────────────────────────────────────────────────────
// SaveTimelineEvent 持久化时间轴事件。
   func (s *Store) SaveTimelineEvent(ctx context.Context, ev TimelineEvent) error {
	   if s == nil || s.kv == nil {
		  return ErrUnavailable
	   }
	  body, err := json.Marshal(ev)
	  if err != nil {
		  return fmt.Errorf("wrongcredit: marshal timeline: %w", err)
	  }
	  key := fmt.Sprintf("%s:%06d", ev.CaseID, ev.Seq)
	   if err := s.kv.Put(ctx, persist.Row{Kind: kindTimeline, Key: key, Body: body}, persist.Fence{}); err != nil {
		  return err
	   }
	   if s.hasSQL() {
		   return s.sql.saveTimeline(ctx, ev)
	   }
	   return nil
  }

// ListTimeline 列出指定 caseID 的时间轴事件。
  func (s *Store) ListTimeline(ctx context.Context, caseID string) ([]TimelineEvent, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.hasSQL() {
		   tl, err := s.sql.listTimeline(ctx, caseID)
		   if err == nil {
			   return tl, nil
		   }
	   }
	   if s.kv == nil {
		  return nil, ErrUnavailable
	   }
	   rows, err := s.kv.ListAfter(ctx, kindTimeline, caseID+":", 5000)
	  if err != nil {
		   return nil, err
	  }
	   out := make([]TimelineEvent, 0)
	  for _, row := range rows {
		   if !strings.HasPrefix(row.Key, caseID+":") {
			   break
		  }
		  var ev TimelineEvent
		  if err := json.Unmarshal(row.Body, &ev); err != nil {
			   continue
		  }
		   out = append(out, ev)
	   }
	  return out, nil
   }

// ─── 互斥锁持久化 ─────────────────────────────────────────────────────────────
// SaveMutexClaim 持久化互斥锁记录（节点重启后可重建 CreditMutex 内存状态）。
   func (s *Store) SaveMutexClaim(ctx context.Context, c creditClaim) error {
	   if s == nil || s.kv == nil {
		   return ErrUnavailable
	   }
	  body, err := json.Marshal(c)
	   if err != nil {
		   return fmt.Errorf("wrongcredit: marshal mutex: %w", err)
	   }
	   if err := s.kv.Put(ctx, persist.Row{Kind: kindMutex, Key: c.CreditRef, Body: body}, persist.Fence{}); err != nil {
		   return err
	   }
	   if s.hasSQL() {
		   if c.ClaimedAt.IsZero() {
			   c.ClaimedAt = time.Now().UTC()
		   }
		   return s.sql.saveMutex(ctx, c)
	   }
	   return nil
   }

// DeleteMutexClaim 删除互斥锁记录（案件关闭/abort 时调用）。

   func (s *Store) DeleteMutexClaim(ctx context.Context, creditRef string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   if s.kv != nil {
		   if err := s.kv.Delete(ctx, kindMutex, creditRef, persist.Fence{}); err != nil && !s.hasSQL() {
			   return err
		   }
	   }
	   if s.hasSQL() {
		   return s.sql.deleteMutex(ctx, creditRef)
	   }
	   return nil
   }

// LoadMutexClaims 启动时加载所有活跃互斥锁记录，用于重建内存 CreditMutex。
   func (s *Store) LoadMutexClaims(ctx context.Context) ([]creditClaim, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if s.hasSQL() {
		   claims, err := s.sql.loadMutex(ctx)
		   if err == nil {
			   return claims, nil
		   }
	  }
	  if s.kv == nil {
		  return nil, ErrUnavailable
	  }
	  rows,
		   err := s.kv.List(ctx, kindMutex, 10000)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]creditClaim, 0, len(rows))
	   for _, row := range rows {
		   var c creditClaim
		   if err := json.Unmarshal(row.Body, &c); err != nil {
			  continue
		   }
		   out = append(out, c)
	   }
	   return out, nil
  }
