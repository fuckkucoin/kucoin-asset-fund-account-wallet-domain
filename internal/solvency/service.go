package solvency

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/bus"
	"github.com/google/uuid"
)

// AuditAppender 发布时向审计仓追加一条事件。实现不得改分区结构。

type AuditAppender interface {
	Append(kind, actor, body string) (string, error)
}

type Options struct {
	Books  Books
	Store  Store
	Secret string
	Bus    bus.Bus
	Audit  AuditAppender
	Clock  func() time.Time
	AllowOverride bool
}

type Pipeline struct {
	mu sync.Mutex
	books  Books
	store  Store
	secret string
	bus   bus.Bus
	audit AuditAppender
	clock func() time.Time
	allowOverride bool
	overrideOn bool
	override   []ReserveLeg
	signals []DriftSignal
}

   func New(opt Options) *Pipeline {
	  st := opt.Store
	   if st == nil {
		   st = NewMem()
	  }
	  return &Pipeline{
		  books: opt.Books,
		  store: st, secret: strings.TrimSpace(opt.Secret),
		  bus: opt.Bus, audit: opt.Audit, clock: opt.Clock, allowOverride: opt.AllowOverride,
	   }
   }

  func (p *Pipeline) now() time.Time {
	   t := time.Now()
	   if p != nil && p.clock != nil {
		   t = p.clock()
	  }
	   if t.IsZero() {
		  t = time.Now()
	   }
	   return utc(t)
   }

   func (p *Pipeline) SetClock(fn func() time.Time) {
	  if p == nil || fn == nil {
		   return
	   }
	  p.mu.Lock()
	   p.clock = fn
	  p.mu.Unlock()
  }

  func (p *Pipeline) Store() Store {
	  if p == nil {
		  return nil
	  }
	  return p.store
   }

// Begin 开一张 building 证明单。此时没有根，发布会被仓储拒绝。
   func (p *Pipeline) Begin(ctx context.Context, sch Schedule) (Attestation, error) {
	  if p == nil || p.store == nil {
		   return Attestation{}, ErrUnavailable
	   }
	  if !sch.Enabled || sch.Window <= 0 || sch.Window > maxWindow {
		   return Attestation{}, ErrWindow
	  }
	   now := p.now()
	  att := Attestation{
		  ID: uuid.NewString(), ScheduleID: sch.ID, Status: StatusBuilding,
		   Class: ClassInternal,
		  Solvency: true, Deprecated: false,
		  WindowStart: now, WindowEnd: now.Add(sch.Window),
		   FenceToken: uuid.NewString(), Scope: joinScope(sch.Assets),
		   Disclaimer: DisclaimerInternal,
		  CreatedAt:  now, UpdatedAt: now,
	   }
	   if err := p.store.InsertAttestation(ctx, att); err != nil {
		   return Attestation{}, err
	   }
	  noteCollect(att.Status)
	   return att, nil
   }

// Seal 把 building 收成 sealed。叶与当前账面或围栏不一致时保持 building，不写封存摘要。
// 窗口已过则拒绝封存，避免用过期草稿补一张根。
  func (p *Pipeline) Seal(ctx context.Context, id string) (Attestation, error) {
	  if p == nil || p.store == nil {
		  return Attestation{}, ErrUnavailable
	  }
	   att, err := p.store.GetAttestation(ctx, id)
	   if err != nil {
		   return Attestation{}, err
	   }
	   if att.Status != StatusBuilding {
		  if att.Status == StatusSealed || att.Status == StatusPublished {
			   return att, ErrImmutable
		  }
		   return att, ErrTransition
	   }
	   now := p.now()
	   if now.After(att.WindowEnd) {
		  return att, ErrWindow
	   }
	   if strings.TrimSpace(p.secret) == "" {
		   return att, ErrSecret
	  }
	   if att.Root == "" || att.Coverage == "" {
		   return att, ErrNotCompared
	  }
	   if att.Coverage == CoverageUnread {
		  return att, ErrReserveUnread
	   }
	   if att.SealDigest != "" {
		  return att, ErrPartial
	  }
	  leaves, err := p.store.ListLeaves(ctx, id)
	   if err != nil {
		   return att, err
	  }
	   if len(leaves) != att.LeafCount {
		   return att, ErrLeafMismatch
	  }
	   live, epoch, err := p.readLeaves(ctx, att, now)
	   if err != nil {
		  return att, err
	   }
	  if epoch != att.FenceEpoch {
		  return att, ErrFenceMoved
	   }
	   if BookDigest(live) != att.BookDigest {
		   return att, ErrLeafMismatch
	  }
	   tr, err := BuildTree(leaves)
	   if err != nil {
		  return att, err
	  }
	  if tr.Root != att.Root {
		  return att, ErrRoot
	  }
	  snaps, err := p.store.ListReserve(ctx, id)
	  if err != nil {
		   return att, err
	  }
	   if err := coverAssets(leaves, snaps); err != nil {
		   return att, err
	   }
	   att.Class, att.Disclaimer = classOf(att)
	   seal := SealDigest(att)
	  sig := Sign(p.secret, SignCanon(att, seal))
	  next, err := p.store.CasAttestation(ctx, id, StatusBuilding, AttestPatch{
		   Status: StatusSealed, Signature: sig, Signer: SignerID, SealDigest: seal,
		  Class: att.Class, Disclaimer: att.Disclaimer, SealedAt: &now, UpdatedAt: now,
	   })
	  if err != nil {
		  return att, err
	  }
	   noteCollect(next.Status)
	  if err := p.bump(ctx, next, now); err != nil {
		   return next, err
	   }
	  return next, nil
   }

// Publish 只接受 sealed。building 上即使已经写下草稿根也返回 ErrPartial，状态保持不变。
// 内控档（短缺或裁剪范围）必须由运维显式确认后才能发布，且类别不会被改成可发布档。

  func (p *Pipeline) Publish(ctx context.Context, id string, acknowledge bool) (Attestation, error) {
	   if p == nil || p.store == nil {
		  return Attestation{},
			  ErrUnavailable
	   }
	  att, err := p.store.GetAttestation(ctx, id)
	   if err != nil {
		  return Attestation{}, err
	   }
	   switch att.Status {
	   case StatusBuilding:
		  noteRefuse("building")
		   if att.Root != "" && att.SealDigest == "" {
			   return att, ErrPartial
		  }
		  return att, ErrNotSealed
	   case StatusPublished:
		   noteRefuse("immutable")
		   return att, ErrImmutable
	   case StatusSealed:
	  default:
		  noteRefuse(string(att.Status))
		  return att, ErrNotSealed
	   }
	  if err := p.recheck(ctx, att); err != nil {
		  noteRefuse("recheck")
		  return att, err
	   }
	   if att.Class != ClassPublishable && !acknowledge {
		   noteRefuse("deficit")
		  return att, ErrDeficitOpen
	   }
	  if att.Class == ClassPublishable && att.Coverage != CoverageCovered {
		  noteRefuse("coverage")
		   return att, ErrDeficitOpen
	   }
	   now := p.now()
	   next, err := p.store.CasAttestation(ctx, id, StatusSealed, AttestPatch{
		  Status: StatusPublished, PublishedAt: &now, UpdatedAt: now,
	   })
	   if err != nil {
		   noteRefuse("cas")
		   return att, err
	   }
	  noteCollect(next.Status)
	  if p.audit != nil {
		   ref, aerr := p.audit.Append("solvency.publish", SignerID, auditBody(next))
		  if aerr != nil {
			   return next, fmt.Errorf("%w: %v", ErrAudit, aerr)
		  }
		   if err := p.store.NoteAudit(ctx, next.ID, ref); err != nil {
			  return next, err
		   }
		  next, _ = p.store.GetAttestation(ctx, id)
	   }
	  return next, nil
   }

  func auditBody(a Attestation) string {
	  return strings.Join([]string{a.ID, a.Root, string(a.Status), a.Class, a.Coverage, a.SealDigest}, "|")
   }

// Revoke 作废已封存或已发布的证明。building 只能废弃，不能作废后假装发布过。

  func (p *Pipeline) Revoke(ctx context.Context, id, actor, reason string) (Attestation, error) {
	  if err := validActor(actor); err != nil {
		  return Attestation{}, err
	  }
	  if strings.TrimSpace(reason) == "" {
		   return Attestation{},
			  ErrTransition
	  }
	   att, err := p.store.GetAttestation(ctx, id)
	   if err != nil {
		  return Attestation{}, err
	   }
	   if !AllowAttest(att.Status, StatusRevoked) {
		   return att, ErrTransition
	  }
	   now := p.now()
	  return p.store.CasAttestation(ctx, id, att.Status, AttestPatch{
		  Status: StatusRevoked, Reason: strings.TrimSpace(reason), RevokedAt: &now, UpdatedAt: now,
	   })
   }

// Abandon 丢弃超时或被中断的 building。不生成封存摘要，也不能再发布。

   func (p *Pipeline) Abandon(ctx context.Context, id, reason string) (Attestation, error) {
	   now := p.now()
	   return p.store.CasAttestation(ctx, id, StatusBuilding, AttestPatch{
		  Status: StatusAbandoned, Reason: strings.TrimSpace(reason), UpdatedAt: now,
	   })
   }

// Prove 对已封存或已发布的根出具包含路径。路径来自已落叶，不读取调用之后的账面。
   func (p *Pipeline) Prove(ctx context.Context, id, userID, book, asset string) (Proof, Attestation, error) {
	   att, err := p.store.GetAttestation(ctx, id)
	   if err != nil {
		  return Proof{}, Attestation{}, err
	  }
	  if !ReadableProof(att.Status) {
		  return Proof{}, att, ErrNotSealed
	  }
	  book, err = NormBook(book)
	   if err != nil {
		   return Proof{}, att, err
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  userID = strings.TrimSpace(userID)
	   leaves, err := p.store.ListLeaves(ctx, id)
	  if err != nil {
		   return Proof{}, att, err
	   }
	  tr, err := BuildTree(leaves)
	  if err != nil {
		  return Proof{}, att, err
	   }
	  if tr.Root != att.Root {
		  return Proof{}, att, ErrRoot
	   }
	  pr, err := tr.Prove(LiabilityLeaf{UserID: userID, Book: book, Asset: asset})
	  if err != nil {
		  return Proof{}, att, err
	  }
	   if err := Verify(att.Root, pr); err != nil {
		   return Proof{}, att, err
	   }
	  return pr, att, nil
   }

// VerifyProof 只读校验。不改证明单。
  func VerifyProof(root string, pr Proof) error { return Verify(root, pr) }

   func (p *Pipeline) Get(ctx context.Context, id string) (Attestation, error) {
	  if p == nil || p.store == nil {
		   return Attestation{}, ErrUnavailable
	  }
	   return p.store.GetAttestation(ctx, id)
   }

   func (p *Pipeline) List(ctx context.Context, scheduleID string, limit int) ([]Attestation, error) {
	  if p == nil || p.store == nil {
		  return nil, ErrUnavailable
	   }
	   return p.store.ListAttestations(ctx, scheduleID, limit)
  }

  func (p *Pipeline) recheck(ctx context.Context, att Attestation) error {
	   if strings.TrimSpace(p.secret) == "" {
		  return ErrSecret
	   }
	   leaves, err := p.store.ListLeaves(ctx, att.ID)
	  if err != nil {
		  return err
	  }
	  if len(leaves) != att.LeafCount {
		  return ErrLeafMismatch
	   }
	   tr, err := BuildTree(leaves)
	  if err != nil {
		  return err
	  }
	  if tr.Root != att.Root {
		   return ErrRoot
	  }
	   if BookDigest(leaves) != att.BookDigest {
		   return ErrLeafMismatch
	  }
	   return VerifySignature(p.secret, att)
  }
