package wdpartial

import (
	"strings"
	"time"
	"github.com/google/uuid"
)

// DefaultLeaseTTL Resume 世代锁默认租约 TTL。

const DefaultLeaseTTL = 5 * time.Minute

// LeaseBook Resume 世代锁存储接口。

type LeaseBook interface {
	// GetByBatch 返回指定批次的当前租约（若不存在则 ErrLeaseNotFound）。
	GetByBatch(wdbatchID string) (*ResumeLease, error)
	// Save 首次写入租约。
	Save(lease *ResumeLease) error
	// Update 更新租约（世代递进、过期续租、主动释放）。
	Update(lease *ResumeLease) error
}

// MutexManager Resume 互斥世代锁管理器。

type MutexManager struct {
	book LeaseBook
	clock func() time.Time
	ttl time.Duration
}

// NewMutexManager 构造 MutexManager。

   func NewMutexManager(book LeaseBook) *MutexManager {
	   return &MutexManager{
		   book: book,
		  clock: func() time.Time { return time.Now().UTC() },
		  ttl:   DefaultLeaseTTL,
	   }
  }

// WithClock 注入时钟。

  func (m *MutexManager) WithClock(c func() time.Time) *MutexManager {
	  if m != nil && c != nil {
		   m.clock = c
	   }
	   return m
  }

   // WithTTL 覆盖租约 TTL（主要用于）。

   func (m *MutexManager) WithTTL(ttl time.Duration) *MutexManager {
	   if m != nil && ttl > 0 {
		   m.ttl = ttl
	  }
	   return m
   }

// Acquire 为指定批次获取 Resume 世代锁。
// - 若无现有租约，直接创建（Gen=1）。
// - 若现有租约未过期且 holder != caller，返回 ErrLeaseConflict。
// - 若现有租约已过期或 holder == caller（续租），递进 Gen 并续租。
// 返回获得的 ResumeLease（含最新 Gen）。

  func (m *MutexManager) Acquire(wdbatchID, holder string) (*ResumeLease, error) {
	  if m == nil || m.book == nil {
		  return nil, ErrUnavailable
	  }
	   wdbatchID = strings.TrimSpace(wdbatchID)
	   holder = strings.TrimSpace(holder)
	  if wdbatchID == "" {
		   return nil, ErrBatchID
	  }
	   if holder == "" {
		  return nil, ErrActor
	   }
	  now := m.clock()
	  cur,
		   err := m.book.GetByBatch(wdbatchID)
	   if err != nil && err != ErrLeaseNotFound {
		   return nil, err
	  }
	   if err == ErrLeaseNotFound || cur == nil {
		   // 首次创建
		  lease := &ResumeLease{
			  ID:        uuid.NewString(),
			  WDBatchID: wdbatchID,
			  Holder: holder,
			   Gen:    1,
			   AcquiredAt: now,
			  ExpiresAt:  now.Add(m.ttl),
		  }
		  if serr := m.book.Save(lease); serr != nil {
			  return nil, serr
		  }
		  return lease, nil
	  }
	   // 检查现有租约
	   if cur.Active(now) && cur.Holder != holder {
		  return nil, ErrLeaseConflict
	   }
	  // 可抢占（过期）或续租（同一 holder）：递进 Gen
	  cur.Gen++
	   cur.Holder = holder
	   cur.AcquiredAt = now
	   cur.ExpiresAt = now.Add(m.ttl)
	   cur.Released = false
	   cur.ReleasedAt = time.Time{}
	  if uerr := m.book.Update(cur); uerr != nil {
		  return nil, uerr
	  }
	  return cur, nil
   }

// Verify 校验调用者持有的世代号是否仍有效（防止 ErrStaleGen）。
// 调用者在执行任何续提/释放操作前应先 Verify：
// - 若 gen < 当前 Gen → ErrStaleGen
// - 若 gen > 当前 Gen → ErrStaleGen（不合理）
// - 若租约已过期 → ErrLeaseExpired
// - 若 holder 不匹配 → ErrNoLease

  func (m *MutexManager) Verify(wdbatchID, holder string, gen int64) error {
	   if m == nil || m.book == nil {
		   return ErrUnavailable
	   }
	   wdbatchID = strings.TrimSpace(wdbatchID)
	  holder = strings.TrimSpace(holder)
	  cur, err := m.book.GetByBatch(wdbatchID)
	   if err != nil {
		  if err == ErrLeaseNotFound {
			   return ErrNoLease
		   }
		  return err
	   }
	  now := m.clock()
	  if cur.Holder != holder {
		   return ErrNoLease
	  }
	  if cur.Released || !cur.Active(now) {
		  return ErrLeaseExpired
	   }
	   if cur.Gen != gen {
		  return ErrStaleGen
	   }
	   return nil
  }

// Release 主动释放租约。
// 只有当前 holder 可以释放（holder 不匹配则 ErrNoLease）。

   func (m *MutexManager) Release(wdbatchID, holder string, gen int64) error {
	  if m == nil || m.book == nil {
		   return ErrUnavailable
	   }
	  wdbatchID = strings.TrimSpace(wdbatchID)
	   holder = strings.TrimSpace(holder)
	  cur,
		   err := m.book.GetByBatch(wdbatchID)
	   if err != nil {
		   if err == ErrLeaseNotFound {
			   return ErrNoLease
		   }
		  return err
	   }
	  if cur.Holder != holder {
		   return ErrNoLease
	   }
	   if cur.Gen != gen {
		   return ErrStaleGen
	  }
	   now := m.clock()
	  cur.Released = true
	  cur.ReleasedAt = now
	  return m.book.Update(cur)
   }

// CurrentLease 查询指定批次当前租约（不做鉴权）。
// 若不存在返回 ErrLeaseNotFound。
   func (m *MutexManager) CurrentLease(wdbatchID string) (*ResumeLease, error) {
	   if m == nil || m.book == nil {
		  return nil, ErrUnavailable
	   }
	  return m.book.GetByBatch(strings.TrimSpace(wdbatchID))
  }

// ─── 内存 LeaseBook 实现 ──────────────────────────────────────────────────────
// MemLeaseBook 内存 LeaseBook，仅供使用。
type MemLeaseBook struct {
	byBatch map[string]*ResumeLease
}

// NewMemLeaseBook 构造内存 LeaseBook。
   func NewMemLeaseBook() *MemLeaseBook {
	  return &MemLeaseBook{byBatch: make(map[string]*ResumeLease)}
   }

  func (b *MemLeaseBook) GetByBatch(wdbatchID string) (*ResumeLease, error) {
	   if b == nil {
		   return nil, ErrUnavailable
	  }
	  l, ok := b.byBatch[wdbatchID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  cp := *l
	  return &cp, nil
  }

   func (b *MemLeaseBook) Save(lease *ResumeLease) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	   cp := *lease
	   b.byBatch[lease.WDBatchID] = &cp
	   return nil
   }

   func (b *MemLeaseBook) Update(lease *ResumeLease) error {
	   if b == nil {
		   return ErrUnavailable
	  }
	  _, ok := b.byBatch[lease.WDBatchID]
	  if !ok {
		  return ErrLeaseNotFound
	   }
	  cp := *lease
	   b.byBatch[lease.WDBatchID] = &cp
	  return nil
  }
