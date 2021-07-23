package hotpregate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

// DefaultLeaseTTL 默认租约有效期（5 分钟；预检到提交的最大期望窗口）。
const DefaultLeaseTTL = 5 * time.Minute

// windowLayout 时间窗口形态（分钟精度）。
const windowLayout = "200601021504"

// InventoryObserver 只读库存观察接口。
// GuardedObserver 实现此接口；中使用 mockObserver。

type InventoryObserver interface {
	Observe(ctx context.Context, asset string) (hotinv.Snapshot, error)
	ObserveAll(ctx context.Context) ([]hotinv.Snapshot,
		error)
}

// Service 热库存预检租约核心引擎。
type Service struct {
	store Store
	observer InventoryObserver
	book  *hotinv.Book
	nowFn func() time.Time
}

// New 构造 Service。observer 用于只读 Observe，不触发写操作。
  func New(store Store, book *hotinv.Book, observer InventoryObserver) *Service {
	  return &Service{
		  store:    store,
		   observer: observer,
		  book: book,
		  nowFn: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 覆盖时钟（仅用于）。

   func (svc *Service) WithClock(fn func() time.Time) *Service {
	  svc.nowFn = fn
	   return svc
  }

  // ── 输入校验 ──────────────────────────────────────────────────────────────────

  func validateAsset(asset string) (string, error) {
	   a := strings.ToUpper(strings.TrimSpace(asset))
	  if a == "" {
		   return "", ErrInvalidAsset
	  }
	   return a, nil
  }

   func validateWindow(w string) (string, error) {
	   w = strings.TrimSpace(w)
	   if w == "" || len(w) != 12 {
		  return "", ErrInvalidWindow
	   }
	  return w, nil
   }

  func newLeaseID() string { return "hpl_" + uuid.New().String() }
   func newProofID() string { return "idp_" + uuid.New().String() }

// ── IssueLease 预检租约签发 ───────────────────────────────────────────────────
// IssueLease 在 PreSubmit/ReadGate 只读 Observe 通过后签发预检租约。
// 幂等规则：同 asset + window 的 IdemKey 若存在未过期租约，直接返回已有租约（不新建）。
// 若已过期，重新 Observe 后签发新租约。
// 只读契约：Observe 由 GuardedObserver 包装，任何写操作触发 panic。
  func (svc *Service) IssueLease(ctx context.Context, req IssueRequest) (*HotPrecheckLease, error) {
	   asset,
		  err := validateAsset(req.Asset)
	   if err != nil {
		  return nil, err
	  }
	  now := svc.nowFn()
	   window := req.Window
	  if window == "" {
		  window = now.UTC().Format(windowLayout)
	   }
	   if _, err := validateWindow(window); err != nil {
		   return nil, err
	   }
	   idemKey := asset + ":" + window
	   // 幂等检查：若存在未过期租约，直接返回。
	  existing, err := svc.store.GetLeaseByIdem(ctx, idemKey)
	   if err == nil {
		   if !existing.IsExpired(now) {
			   return existing, nil
		   }
		  // 已过期，接着签发新租约。
	  } else if err != ErrLeaseNotFound {
		   return nil, fmt.Errorf("hotpregate issue: store error: %w", err)
	   }
	   // 只读 Observe。
	   snap, err := svc.observer.Observe(ctx, asset)
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrObserveFailed, err)
	   }
	   // 计算快照摘要。
	   band := req.Band
	  digest := ComputeDigest(snap, band, now)
	   // 获取 Cap 信息。
	   capVal := snap.HotMax
	   if capVal == "" {
		   capVal = "0"
	  }
	   // 确定 TTL。
	  ttl := DefaultLeaseTTL
	  if req.TTL > 0 {
		  ttl = time.Duration(req.TTL) * time.Second
	   }
	   lease := HotPrecheckLease{
		  LeaseID: newLeaseID(),
		   Asset: asset,
		   Window:       window,
		   Digest:       digest,
		  CapSnapshot:  capVal,
		   BandSnapshot: band,
		  State:    LeaseIssued,
		  IssuedAt: now,
		   ExpiresAt: now.Add(ttl),
		  IdemKey:   idemKey,
		  Actor:     strings.TrimSpace(req.Actor),
	  }
	  if err := svc.store.InsertLease(ctx, lease); err != nil {
		  if err == ErrIdemConflict {
			   // 并发：另一个协程刚插入。再查一次。
			   got, gerr := svc.store.GetLeaseByIdem(ctx, idemKey)
			   if gerr != nil {
				   return nil, fmt.Errorf("hotpregate issue idem conflict re-fetch: %w", gerr)
			  }
			   return got, nil
		  }
		   return nil, fmt.Errorf("hotpregate issue: insert: %w", err)
	   }
	   return &lease, nil
  }

// ── BindLease 提交前绑定 ──────────────────────────────────────────────────────
// BindLease 将 issued→bound；表示提交方已取走租约，将使用此次 Observe 快照作为基准。
// 提交决策消费（ConsumeOrReject）前须先调用 BindLease，以便复核时区分"已取走"与"未取走"。

   func (svc *Service) BindLease(ctx context.Context, req BindRequest) (*HotPrecheckLease, error) {
	  if strings.TrimSpace(req.LeaseID) == "" {
		   return nil, ErrInvalidLeaseID
	  }
	  now := svc.nowFn()
	   lease, err := svc.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return nil, err
	  }
	   if lease.IsExpired(now) {
		  return nil, ErrLeaseExpired
	  }
	  switch lease.State {
	   case LeaseIssued:
	  // 正常路径
	   case LeaseBound:
		  return lease, nil // 幂等
	  case LeaseConsumed:
		   return nil, ErrLeaseConsumed
	  case LeaseDrifted:
		   return nil, ErrLeaseDrifted
	   case LeaseExpired:
		  return nil, ErrLeaseExpired
	   }
	  if err := svc.store.UpdateLeaseState(ctx, req.LeaseID, LeaseIssued, LeaseBound, now,
		   fmt.Sprintf("bound by %s", req.Actor)); err != nil {
		   return nil, fmt.Errorf("hotpregate bind: %w", err)
	   }
	  lease.State = LeaseBound
	   t := now
	   lease.BoundAt = &t
	  return lease, nil
  }

// ── RecheckLease 提交复核（核心封堵路径）────────────────────────────────────
// RecheckLease 提交/归集决策消费前，重新 Observe 并签发时摘要。
// 若摘要匹配（无漂移）：将 bound→consumed，返回 RecheckResult{Accepted: true}。
// 若摘要不匹配（有漂移）：
// 1. 签发 InvDriftProof 并写库；
// 2. 将租约关联到 DriftProofID 并置 drifted 状态；
// 3. 返回 RecheckResult{Accepted: false, DriftProof: ...}。
// 提交方必须检查 RecheckResult.Accepted；若 false 必须拒绝本次提交/归集决策。
   func (svc *Service) RecheckLease(ctx context.Context, req RecheckRequest) (RecheckResult, error) {
	   if strings.TrimSpace(req.LeaseID) == "" {
		   return RecheckResult{},
			   ErrInvalidLeaseID
	  }
	   now := svc.nowFn()
	   lease, err := svc.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return RecheckResult{}, err
	  }
	  if lease.IsExpired(now) {
		  return RecheckResult{}, ErrLeaseExpired
	   }
	  switch lease.State {
	   case LeaseBound, LeaseIssued:
		  // 正常路径：issued 状态也允许复核（未明确 Bind 时）
	  case LeaseConsumed:
		  return RecheckResult{}, ErrLeaseConsumed
	   case LeaseDrifted:
		  return RecheckResult{}, ErrLeaseDrifted
	  case LeaseExpired:
		   return RecheckResult{}, ErrLeaseExpired
	   }
	   // 重新只读 Observe。
	  snap,
		   err := svc.observer.Observe(ctx, lease.Asset)
	  if err != nil {
		   return RecheckResult{}, fmt.Errorf("%w: %v", ErrObserveFailed, err)
	   }
	  recheckDigest := ComputeDigest(snap,
		   lease.BandSnapshot, now)
	   // 摘要。
	   if DigestMatch(lease.Digest, recheckDigest) {
		   // 无漂移：消费租约。
		   nextState := LeaseConsumed
		   if err := svc.store.UpdateLeaseState(ctx, lease.LeaseID, lease.State, nextState, now,
			   fmt.Sprintf("consumed by %s after recheck: no drift", req.Actor)); err != nil {
			  return RecheckResult{}, fmt.Errorf("hotpregate recheck consume: %w", err)
		  }
		  lease.State = nextState
		  t := now
		   lease.ConsumedAt = &t
		  return RecheckResult{Accepted: true, Lease: lease}, nil
	   }
	   // 漂移：签发 InvDriftProof。
	   severity := ClassifyDrift(lease.Digest, recheckDigest, lease.BandSnapshot)
	  reason := buildDriftReason(lease.Digest, recheckDigest, severity)
	  proof := InvDriftProof{
		  ProofID: newProofID(),
		   LeaseID: lease.LeaseID,
		  Asset:   lease.Asset,
		   Window: lease.Window,
		   SnapshotDigestAtIssue:   lease.Digest,
		   SnapshotDigestAtRecheck: recheckDigest,
		   HotAvailDelta: HotAvailDelta(lease.Digest, recheckDigest),
		  WarmAvailDelta: WarmAvailDelta(lease.Digest,
			   recheckDigest),
		  DigestMismatch: true,
		  Severity:       severity,
		  Reason:    reason,
		  CreatedAt: now,
	   }
	   if err := svc.store.InsertProof(ctx, proof); err != nil {
		  return RecheckResult{}, fmt.Errorf("hotpregate recheck drift proof insert: %w", err)
	   }
	  if err := svc.store.SetLeaseDriftProof(ctx, lease.LeaseID, proof.ProofID, now); err != nil {
		  return RecheckResult{}, fmt.Errorf("hotpregate recheck set drift proof: %w", err)
	   }
	   lease.State = LeaseDrifted
	  lease.DriftProofID = proof.ProofID
	  t := now
	   lease.ConsumedAt = &t
	   return RecheckResult{
		  Accepted: false,
		   Lease:    lease,
		  DriftProof: &proof,
	  }, ErrDriftDetected
  }

// ── 查询方法 ──────────────────────────────────────────────────────────────────
// GetLease 按 LeaseID 查询租约。

   func (svc *Service) GetLease(ctx context.Context, leaseID string) (*HotPrecheckLease, error) {
	   if strings.TrimSpace(leaseID) == "" {
		   return nil, ErrInvalidLeaseID
	  }
	  return svc.store.GetLease(ctx, leaseID)
   }

// GetProof 按 ProofID 查询漂移证明。
   func (svc *Service) GetProof(ctx context.Context, proofID string) (*InvDriftProof, error) {
	   if strings.TrimSpace(proofID) == "" {
		  return nil, ErrProofNotFound
	  }
	  return svc.store.GetProof(ctx, proofID)
   }

// ListLeases 列出租约（分页）。

   func (svc *Service) ListLeases(ctx context.Context, f LeaseListFilter) ([]HotPrecheckLease, error) {
	  if f.Limit <= 0 {
		  f.Limit = 50
	   }
	   return svc.store.ListLeases(ctx, f)
  }

// ListProofs 列出漂移证明（分页）。
   func (svc *Service) ListProofs(ctx context.Context, f DriftProofListFilter) ([]InvDriftProof, error) {
	  if f.Limit <= 0 {
		  f.Limit = 50
	  }
	  return svc.store.ListProofs(ctx, f)
   }

// ── 内部辅助 ──────────────────────────────────────────────────────────────────

  func buildDriftReason(issue, recheck InvSnapshotDigest, sev DriftSeverity) string {
	  hotDelta := HotAvailDelta(issue, recheck)
	   warmDelta := WarmAvailDelta(issue, recheck)
	   return fmt.Sprintf("severity=%s hot_delta=%s warm_delta=%s issue_hex=%s recheck_hex=%s",
		   sev,
		  hotDelta, warmDelta, issue.Hex[:12], recheck.Hex[:12])
   }
