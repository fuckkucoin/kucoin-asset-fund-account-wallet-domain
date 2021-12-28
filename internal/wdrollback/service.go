// @FileName: service.go
// @Summary: 补偿腿申请要么整笔记下，要么余额保持原样。
// 回滚账服务：建账、闸门、分录在同一把锁里完成。
// 不调用出金 Cancel，不改链上广播行。
// 不变量：闸门拒绝时只追加未生效的审计分录，可用和冻结不变。
// @Author: Blaise
// @Created: 2021.12.26

package wdrollback

import (
	"strings"
	"sync"
	"time"
)

// Service 进程内回滚账。SQL 镜像由编排器另写，本服务以内存为资金真相。

type Service struct {
	mu sync.Mutex
	pos map[string]*Position
	holds map[string]*hold
	byKey  map[string]*Entry
	order  []string
	audits []Audit
	seq    uint64
}

// New 空账。权益锚点在第一次 SeedIfAbsent 时写下。
   func New() *Service {
	   return &Service{
		   pos: map[string]*Position{},
		  holds: map[string]*hold{},
		  byKey: map[string]*Entry{},
	  }
  }

// Apply 执行一条腿。
// 功能：幂等重入返回原分录；热态释放拒绝并记审计。
// 封账和分片标注不搬家。
// 不变量：动账后立刻守恒，失败则把内存位恢复到申请前。

  func (s *Service) Apply(req Request) (Result, error) {
	  if s == nil {
		  return Result{}, ErrUnavailable
	  }
	   if err := req.validate(); err != nil {
		  return Result{}, err
	   }
	   if req.Now.IsZero() {
		   req.Now = time.Now().UTC()
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if old, ok := s.byKey[req.IdempotencyKey]; ok {
		  if old.Fingerprint != req.fingerprint() {
			   return Result{}, ErrIdemConflict
		  }
		  return Result{Entry: old.copy(), Position: s.position(req.UserID, req.Asset), Replayed: true}, nil
	   }
	  if err := gate(req); err != nil {
		  e := s.appendDeny(req, err.Error())
		   return Result{Entry: e, Position: s.position(req.UserID, req.Asset)}, err
	   }
	  switch req.Kind {
	  case KindDeny:
		   e := s.appendDeny(req, req.Path)
		  return Result{Entry: e, Position: s.position(req.UserID, req.Asset)}, nil
	   case KindSeal:
		   p := s.pos[posKey(req.UserID, req.Asset)]
		  if p == nil {
			  return Result{}, ErrPosition
		  }
		   if err := Check(p.copy(), s.entries()); err != nil {
			  return Result{}, err
		  }
		  e := s.appendMarker(req, false)
		  return Result{Entry: e, Position: p.copy()}, nil
	  case KindShard:
		  p := s.pos[posKey(req.UserID,
			  req.Asset)]
		   if p == nil {
			  return Result{}, ErrPosition
		   }
		  if s.protected(req.UserID, req.Asset).Cmp(p.Frozen) > 0 {
			   return Result{},
				   ErrFilledProtected
		   }
		  e := s.appendMarker(req, false)
		   return Result{Entry: e, Position: p.copy()}, nil
	  }
	  p := s.pos[posKey(req.UserID, req.Asset)]
	   if p == nil {
		   return Result{}, ErrPosition
	  }
	   before := p.copy()
	   var err error
	   switch req.Kind {
	   case KindFreeze:
		   err = s.unfreeze(p, req.Amount)
	  case KindFee:
		   err = s.releaseFee(p, req.Amount)
	   case KindOccupancy:
		   err = s.releaseOccupancy(p, req.Amount)
	  default:
		  err = ErrKind
	  }
	  if err != nil {
		   *p = before
		   return Result{}, err
	   }
	   if err := Check(p.copy(), s.previewEntries(req)); err != nil {
		  *p = before
		  return Result{}, err
	  }
	   e := s.appendMove(req, before, p.copy())
	   return Result{Entry: e, Position: p.copy()}, nil
  }

// previewEntries 在落账前把本次释放算进轧差。尚未写入 byKey。
   func (s *Service) previewEntries(req Request) []Entry {
	   rows := s.entries()
	  rows = append(rows, Entry{
		   UserID: req.UserID, Asset: req.Asset, Kind: req.Kind,
		  Amount: req.Amount.String(),
		  Applied: true,
	   })
	   return rows
   }

   func (s *Service) appendMove(req Request, before, after Position) Entry {
	  e := s.baseEntry(req)
	   e.Applied = true
	   e.AvailableBefore = before.Available.String()
	  e.AvailableAfter = after.Available.String()
	  e.FrozenBefore = before.Frozen.String()
	  e.FrozenAfter = after.Frozen.String()
	  e.FeeBefore = before.FeeReserved.String()
	  e.FeeAfter = after.FeeReserved.String()
	  s.store(e)
	   return e.copy()
   }

  func (s *Service) appendMarker(req Request, applied bool) Entry {
	   e := s.baseEntry(req)
	   e.Applied = applied
	  p := s.position(req.UserID,
		   req.Asset)
	  e.AvailableBefore = p.Available.String()
	  e.AvailableAfter = p.Available.String()
	  e.FrozenBefore = p.Frozen.String()
	   e.FrozenAfter = p.Frozen.String()
	  e.FeeBefore = p.FeeReserved.String()
	  e.FeeAfter = p.FeeReserved.String()
	  s.store(e)
	   return e.copy()
   }

   func (s *Service) appendDeny(req Request, detail string) Entry {
	   key := req.IdempotencyKey
	   if req.Kind != KindDeny {
		  key = req.IdempotencyKey + ":deny"
	  }
	  if old, ok := s.byKey[key]; ok {
		  return old.copy()
	  }
	   now := req.Now
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   e := Entry{
		   ID: s.next("rbe"), PlanID: req.PlanID, LegID: req.LegID, FailureID: req.FailureID,
		   TransferID: req.TransferID, UserID: req.UserID, Asset: req.Asset,
		   Kind: KindDeny, Amount: "0", IdempotencyKey: key,
		  Fingerprint: req.fingerprint() + "|deny", ReasonCode: req.ReasonCode,
		   Path: req.Path, Applied: false, CreatedAt: now.UTC(),
	   }
	   p := s.position(req.UserID, req.Asset)
	   e.AvailableBefore = p.Available.String()
	  e.AvailableAfter = p.Available.String()
	  e.FrozenBefore = p.Frozen.String()
	   e.FrozenAfter = p.Frozen.String()
	  e.FeeBefore = p.FeeReserved.String()
	  e.FeeAfter = p.FeeReserved.String()
	   s.store(e)
	   s.audits = append(s.audits, Audit{
		  ID: e.ID, PlanID: req.PlanID, FailureID: req.FailureID, TransferID: req.TransferID,
		  UserID: req.UserID, Asset: req.Asset, Path: req.Path, TxHash: req.TxHash,
		   ReasonCode: req.ReasonCode, Detail: detail, At: now.UTC(),
	   })
	  return e.copy()
  }

  func (s *Service) baseEntry(req Request) Entry {
	  return Entry{
		  ID: s.next("rbe"), PlanID: req.PlanID, LegID: req.LegID, FailureID: req.FailureID,
		   TransferID: req.TransferID, UserID: req.UserID, Asset: req.Asset,
		   Kind: req.Kind, Amount: req.Amount.String(), IdempotencyKey: req.IdempotencyKey,
		   Fingerprint: req.fingerprint(), ReasonCode: req.ReasonCode, Path: req.Path,
		   CreatedAt: req.Now.UTC(),
	   }
   }

  func (s *Service) store(e Entry) {
	  cp := e
	   s.byKey[e.IdempotencyKey] = &cp
	  s.order = append(s.order, e.IdempotencyKey)
   }

   func (s *Service) next(prefix string) string {
	  s.seq++
	  return prefix + "-" + itoa(s.seq)
  }

  func itoa(n uint64) string {
	   if n == 0 {
		   return "0"
	   }
	  var b [20]byte
	  i := len(b)
	  for n > 0 {
		  i--
		  b[i] = byte('0' + n%10)
		   n /= 10
	   }
	  return string(b[i:])
  }

  func (s *Service) position(user, asset string) Position {
	  p := s.pos[posKey(user, asset)]
	  if p == nil {
		   return Position{UserID: user, Asset: asset}
	   }
	  return p.copy()
  }

  func (s *Service) entries() []Entry {
	  out := make([]Entry, 0, len(s.order))
	   for _, k := range s.order {
		   if e := s.byKey[k]; e != nil {
			  out = append(out, e.copy())
		   }
	  }
	   return out
   }

// Position 读取当前资金位。没有建账时返回 ErrPosition。
   func (s *Service) Position(user, asset string) (Position, error) {
	  if s == nil {
		   return Position{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  p := s.pos[posKey(strings.TrimSpace(user), strings.TrimSpace(asset))]
	   if p == nil {
		   return Position{}, ErrPosition
	  }
	  return p.copy(), nil
  }

// Assert 对外守恒口。夹具用它可用、冻结和分录。

   func (s *Service) Assert(user, asset string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  p := s.pos[posKey(strings.TrimSpace(user), strings.TrimSpace(asset))]
	  if p == nil {
		  return ErrPosition
	  }
	  return Check(p.copy(),
		  s.entries())
  }

// Entries 按用户、资产、出金单过滤。空串表示不过滤。
   func (s *Service) Entries(user, asset, transfer string) []Entry {
	   if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  user = strings.TrimSpace(user)
	   asset = strings.TrimSpace(asset)
	   transfer = strings.TrimSpace(transfer)
	  var out []Entry
	  for _, e := range s.entries() {
		  if user != "" && e.UserID != user {
			   continue
		   }
		   if asset != "" && e.Asset != asset {
			   continue
		   }
		  if transfer != "" && e.TransferID != transfer {
			  continue
		  }
		  out = append(out, e)
	  }
	   return out
   }

// Audits 返回自动解冻拒绝审计，顺序与写入一致。

   func (s *Service) Audits() []Audit {
	  if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   return append([]Audit(nil),
		   s.audits...)
   }

// RestoreEntry 从镜像装回分录。同一幂等键已存在则跳过，不改余额。

   func (s *Service) RestoreEntry(e Entry) {
	   if s == nil || e.IdempotencyKey == "" {
		   return
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.byKey[e.IdempotencyKey]; ok {
		  return
	   }
	  s.store(e)
   }

// HoldCount 保护笔数，供编排器核对部分成交是否登记。
  func (s *Service) HoldCount() int {
	   if s == nil {
		  return 0
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return len(s.holds)
  }
