package ledseal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/snap"
)

// ReplayChecker 快照回放一致性校验器。
type ReplayChecker struct {
	svc *Service
	clock func() time.Time
}

// NewReplayChecker 构造回放校验器。
   func NewReplayChecker(svc *Service) *ReplayChecker {
	  return &ReplayChecker{
		   svc: svc,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。
  func (rc *ReplayChecker) WithClock(c func() time.Time) *ReplayChecker {
	   if rc != nil && c != nil {
		   rc.clock = c
	   }
	   return rc
  }

  // CheckSlice 对给定切片抽样回放一致性校验：
   // 1. 加载切片对应的快照；
   // 2. 对快照 Positions 按确定性顺序哈希（作为"快照指纹"）；
  // 3. 与快照自身 Checksum 核对（利用 snap.Snapshot.VerifyChecksum）；
   // 4. 返回 ReplayCheck 结果。
  // 注意：此处"Apply 指纹"即取快照 Checksum，因为 snap 包已在 CaptureFrom 时
  // 对 Positions 做了确定性 SHA-256。只要 VerifyChecksum 通过，说明快照未被篡改。

   func (rc *ReplayChecker) CheckSlice(ctx context.Context, sliceID string) (ReplayCheck, error) {
	  if rc == nil || rc.svc == nil {
		  return ReplayCheck{}, ErrUnavailable
	  }
	   sl, err := rc.svc.store.GetSlice(ctx, sliceID)
	  if err != nil {
		  return ReplayCheck{}, fmt.Errorf("load slice: %w", err)
	  }
	  // 加载快照。
	   snp, err := rc.svc.snapSvc.Get(ctx, sl.SnapID)
	   if err != nil {
		  return ReplayCheck{}, fmt.Errorf("load snap: %w", err)
	  }
	   // 计算进程内指纹（确定性，不依赖外部服务）。
	   localFP := computeFingerprint(snp)
	  // 用 snap 包的 VerifyChecksum 验内部一致性。
	  verifyErr := snp.VerifyChecksum()
	   match := verifyErr == nil && localFP == snp.Checksum
	   res := ReplayCheck{
		   SliceID:         sliceID,
		   SnapID:          sl.SnapID,
		   FingerprintFrom: localFP,
		   FingerprintSnap: snp.Checksum,
		  Match:           match,
		   CheckedAt: rc.clock(),
	  }
	  if !match {
		  return res, fmt.Errorf("%w: slice=%s snap=%s local=%s stored=%s verifyErr=%v",
			   ErrReplay, sliceID, sl.SnapID, localFP, snp.Checksum, verifyErr)
	   }
	  return res, nil
   }

// CheckBatch 对差分批次的 from/to 两个切片都做回放校验。
// 两者均通过才视为批次回放一致。
   func (rc *ReplayChecker) CheckBatch(ctx context.Context, batchID string) ([]ReplayCheck, error) {
	   if rc == nil || rc.svc == nil {
		   return nil, ErrUnavailable
	  }
	  batch, err := rc.svc.store.GetBatch(ctx, batchID)
	   if err != nil {
		   return nil, err
	   }
	  results := make([]ReplayCheck,
		   0, 2)
	   for _, slID := range []string{batch.FromSliceID, batch.ToSliceID} {
		   res, err := rc.CheckSlice(ctx, slID)
		   if err != nil {
			   return results, err
		   }
		   results = append(results, res)
	   }
	  return results, nil
  }

// computeFingerprint 对快照的 Positions 做确定性哈希。
// 与 snap.checksumOf 算法一致：排序后逐行哈希 book|asset|avail|frozen。

   func computeFingerprint(snp snap.Snapshot) string {
	  positions := make([]snap.Position,
		  len(snp.Positions))
	   copy(positions, snp.Positions)
	   sort.Slice(positions, func(i, j int) bool {
		   ki := strings.ToUpper(positions[i].Book) + "|" + strings.ToUpper(positions[i].Asset)
		  kj := strings.ToUpper(positions[j].Book) + "|" + strings.ToUpper(positions[j].Asset)
		  return ki < kj
	   })
	   h := sha256.New()
	  h.Write([]byte(strings.TrimSpace(snp.UserID)))
	  h.Write([]byte{0})
	   for _, p := range positions {
		  h.Write([]byte(strings.ToUpper(strings.TrimSpace(p.Book)) + "|" + strings.ToUpper(strings.TrimSpace(p.Asset))))
		   h.Write([]byte{0})
		  h.Write([]byte(p.Available))
		  h.Write([]byte{0})
		   h.Write([]byte(p.Frozen))
		  h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil))
  }
