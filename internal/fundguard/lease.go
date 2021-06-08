package fundguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/guardlease"
	"account-assets.kucoin.net/funding/internal/validate"
)

// LeaseService 封装 guardlease.Service 的接口（方便替换）。
type LeaseService interface {
	IssueLease(ctx context.Context, req guardlease.IssueLeaseRequest) (guardlease.SnapLease, error)
	BindLease(ctx context.Context, req guardlease.BindLeaseRequest) (guardlease.SnapLease, error)
	ConsumeLease(ctx context.Context,
		leaseID string) error
	IsBlocked(userID, asset string) bool
	Halted() bool
}

// BindLeaseStore 将 guardlease.Service 注入到 fundguard.Service 中。
   func (s *Service) BindLeaseStore(ls LeaseService) *Service {
	   if s != nil {
		  s.leaser = ls
	   }
	   return s
   }

// GateWithLease 执行完整 Gate 流程并在成功后发放世代租约。
// 返回 Verdict（与原 Gate 兼容）以及新发放的 SnapLease。
// 若 guardlease 未注入，退化到原 Gate 行为（不影响守恒流程，仅丢失世代约束）。
  func (s *Service) GateWithLease(ctx context.Context, req Check) (Verdict, guardlease.SnapLease, error) {
	   verdict, err := s.Gate(ctx, req)
	  if err != nil {
		   return verdict, guardlease.SnapLease{}, err
	  }
	  if s.leaser == nil {
		   return verdict, guardlease.SnapLease{}, nil
	  }
	   if s.leaser.IsBlocked(req.UserID, req.Asset) {
		   return Verdict{Reason: ErrHalt.Error()},
			   guardlease.SnapLease{}, ErrHalt
	   }
	  actor, _ := validate.Actor(req.Actor)
	  // 计算本次快照哈希
	   snap, serr := s.Snapshot(ctx, req.UserID, req.Asset)
	  if serr != nil {
		   return Verdict{Reason: serr.Error()}, guardlease.SnapLease{}, serr
	   }
	  hash := bundleHash(snap)
	  lease, lerr := s.leaser.IssueLease(ctx, guardlease.IssueLeaseRequest{
		  UserID:       req.UserID,
		   Asset:        req.Asset,
		  SnapshotHash: hash,
		  ActorID: actor,
	  })
	  if lerr != nil {
		  return Verdict{Reason: lerr.Error()}, guardlease.SnapLease{}, lerr
	  }
	  return verdict, lease, nil
  }

// ConserveWithLease 在 Conserve 前核验世代租约，核验通过后再执行守恒。
// leaseID 和 snapHash 由调用方（通常为出金/入金协调者）从 GateWithLease 的返回值携带。
// 失败语义（fail-closed）：
// - 租约不存在 → ErrSnap；
// - 租约已超期 → ErrStaleSnap（需重新 Gate）；
// - 哈希不匹配 → ErrStaleSnap；
// - 有未清活账写 → ErrStaleSnap；
// - 守恒偏差 → ErrConserve（与原 Conserve 一致）。
   func (s *Service) ConserveWithLease(ctx context.Context, before, after Bundle, leaseID, snapHash string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   if s.leaser != nil {
		   if s.leaser.Halted() || s.leaser.IsBlocked(before.UserID, before.Asset) {
			   return ErrHalt
		   }
		  _, err := s.leaser.BindLease(ctx, guardlease.BindLeaseRequest{
			  LeaseID: leaseID,
			   UserID:  before.UserID,
			   Asset:   before.Asset,
			  SnapshotHash: snapHash,
		  })
		   if err != nil {
			   return mapLeaseErr(err)
		  }
	   }
	  return s.Conserve(before, after)
  }

// bundleHash 对 Bundle 计算 SHA-256 哈希（用于租约绑定凭证）。
// 行按 Book 字段升序排列保证确定性。

   func bundleHash(b Bundle) string {
	  type row struct {
		  Book string `json:"book"`
		   Asset     string `json:"asset"`
		   Available string `json:"available"`
		   Frozen string `json:"frozen"`
	   }
	   rows := make([]row, len(b.Rows))
	   for i, r := range b.Rows {
		  rows[i] = row{
			   Book: r.Book, Asset: r.Asset,
			  Available: r.Available, Frozen: r.Frozen,
		   }
	  }
	  sort.Slice(rows, func(i, j int) bool {
		   return rows[i].Book < rows[j].Book
	  })
	   type input struct {
		  UserID string `json:"user_id"`
		  Asset string `json:"asset"`
		  Total string `json:"total"`
		   Rows  []row  `json:"rows"`
		   At int64 `json:"at"`
	  }
	   inp := input{
		   UserID: b.UserID,
		   Asset:  b.Asset,
		  Total: b.Total,
		   Rows:  rows,
		  At: b.At.Truncate(time.Millisecond).UnixNano(),
	   }
	   data, _ := json.Marshal(inp)
	   h := sha256.Sum256(data)
	   return hex.EncodeToString(h[:])
   }

// BundleHash 导出代次，供调用方记录租约哈希时使用。
   func BundleHash(b Bundle) string {
	  return bundleHash(b)
  }

// mapLeaseErr 将 guardlease 错误映射到 fundguard 错误域。
  func mapLeaseErr(err error) error {
	  if err == nil {
		   return nil
	   }
	   s := err.Error()
	  switch {
	   case strings.Contains(s, "expired"):
		  return ErrStaleSnap
	  case strings.Contains(s, "stale snapshot"):
		   return ErrStaleSnap
	  case strings.Contains(s, "hash mismatch"):
		   return ErrStaleSnap
	  case strings.Contains(s, "not found"):
		  return ErrSnap
	   case strings.Contains(s, "halted"):
		  return ErrHalt
	   case strings.Contains(s, "write blocked"):
		   return ErrHalt
	   default:
		   return err
	  }
  }
