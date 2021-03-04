package booksnapgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftGuard 漂移守卫：pin 时记录初始状态，assemble 时。
type DriftGuard struct {
	LeaseID string
	UserID  string
	Asset   string
	SnapGen   int64
	BookSnaps []BookSnap
	StartedAt time.Time
	querier WaterlineQuerier
	clock func() time.Time
}

// NewDriftGuard 创建漂移守卫。

  func NewDriftGuard(lease BookSnapLease, querier WaterlineQuerier, clock func() time.Time) *DriftGuard {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &DriftGuard{
		LeaseID: lease.ID,
		UserID:    lease.UserID,
		Asset:     lease.Asset,
		SnapGen:   lease.SnapGen,
		BookSnaps: append([]BookSnap(nil), lease.BookSnaps...),
		StartedAt: clock(),
		querier:   querier,
		clock: clock,
	}
}

// Check 检测是否发生漂移，返回 (drifted, proof, error)。
   func (g *DriftGuard) Check(ctx context.Context) (bool, *BookSnapDriftProof, error) {
	   if g == nil {
		   return false, nil, ErrNilEngine
	   }
	  current, err := g.observeCurrent(ctx)
	   if err != nil {
		  return false, nil, err
	  }
	   snapMap := make(map[string]BookSnap, len(g.BookSnaps))
	   for _, bs := range g.BookSnaps {
		  snapMap[bs.Book] = bs
	   }
	  now := g.clock()
	  for _, obs := range current {
		  pinned, ok := snapMap[obs.Book]
		  if !ok {
			  continue
		  }
		  if obs.Waterline != pinned.Waterline {
			  proof := &BookSnapDriftProof{
				   ID: uuid.NewString(),
				  LeaseID: g.LeaseID,
				   UserID:  g.UserID,
				   Asset:   g.Asset,
				  SnapGen: g.SnapGen,
				   Kind: DriftKindWaterline,
				   Book: obs.Book,
				   OldWaterline: pinned.Waterline,
				   NewWaterline: obs.Waterline,
				  OldGen: pinned.ObserveGen,
				   NewGen: obs.ObserveGen,
				   Detail:    fmt.Sprintf("守卫检测：科目 %s 水线从 %s 漂移至 %s", obs.Book, pinned.Waterline, obs.Waterline),
				  CreatedAt: now,
			   }
			  return true, proof, nil
		   }
		   if obs.ObserveGen != pinned.ObserveGen {
			   proof := &BookSnapDriftProof{
				  ID:      uuid.NewString(),
				   LeaseID: g.LeaseID,
				   UserID:  g.UserID,
				   Asset:   g.Asset,
				   SnapGen: g.SnapGen,
				  Kind:   DriftKindGenMismatch,
				   Book:   obs.Book,
				  OldGen: pinned.ObserveGen,
				  NewGen: obs.ObserveGen,
				  Detail: fmt.Sprintf("守卫检测：科目 %s 世代从 %d 变为 %d", obs.Book, pinned.ObserveGen, obs.ObserveGen),
				  CreatedAt: now,
			  }
			  return true, proof, nil
		   }
		  if obs.Checksum != pinned.Checksum {
			   proof := &BookSnapDriftProof{
				   ID: uuid.NewString(),
				   LeaseID: g.LeaseID,
				   UserID:  g.UserID,
				  Asset:   g.Asset,
				  SnapGen: g.SnapGen,
				   Kind:    DriftKindCreditHop,
				   Book:   obs.Book,
				   Detail: fmt.Sprintf("守卫检测：科目 %s 校验和变化，疑似跨科目信用跳转", obs.Book),
				  CreatedAt: now,
			  }
			   return true, proof, nil
		   }
	  }
	   return false, nil, nil
  }

// CheckCrossBookHop 专项检查跨科目信用跳转。

  func (g *DriftGuard) CheckCrossBookHop(ctx context.Context, sourceBook, targetBook string) (bool, *BookSnapDriftProof, error) {
	   if g == nil {
		  return false, nil, ErrNilEngine
	   }
	   current, err := g.observeCurrent(ctx)
	  if err != nil {
		   return false, nil, err
	  }
	   snapMap := make(map[string]BookSnap, len(g.BookSnaps))
	  for _, bs := range g.BookSnaps {
		  snapMap[bs.Book] = bs
	  }
	   now := g.clock()
	   sourceSnap, sourceOK := snapMap[sourceBook]
	   targetSnap,
		  targetOK := snapMap[targetBook]
	  if !sourceOK || !targetOK {
		   return false, nil, nil
	   }
	  var sourceObs,
		  targetObs WaterlineObservation
	   for _, obs := range current {
		  if obs.Book == sourceBook {
			  sourceObs = obs
		   }
		   if obs.Book == targetBook {
			  targetObs = obs
		  }
	  }
	   sourceDrifted := sourceObs.Waterline != sourceSnap.Waterline || sourceObs.Checksum != sourceSnap.Checksum
	   targetDrifted := targetObs.Waterline != targetSnap.Waterline || targetObs.Checksum != targetSnap.Checksum
	   if sourceDrifted && targetDrifted {
		  proof := &BookSnapDriftProof{
			  ID:      uuid.NewString(),
			   LeaseID: g.LeaseID,
			  UserID:  g.UserID,
			  Asset:   g.Asset,
			  SnapGen:      g.SnapGen,
			  Kind:         DriftKindCreditHop,
			   Book:         sourceBook + "→" + targetBook,
			  OldWaterline: sourceSnap.Waterline,
			   NewWaterline: sourceObs.Waterline,
			  Detail: fmt.Sprintf("跨科目信用跳转：%s→%s，源水线 %s→%s，目标水线 %s→%s", sourceBook, targetBook, sourceSnap.Waterline, sourceObs.Waterline, targetSnap.Waterline, targetObs.Waterline),
			  CreatedAt: now,
		   }
		  return true, proof, nil
	  }
	   if sourceDrifted || targetDrifted {
		  driftBook := sourceBook
		  if targetDrifted {
			   driftBook = targetBook
		  }
		   proof := &BookSnapDriftProof{
			   ID:      uuid.NewString(),
			  LeaseID: g.LeaseID,
			   UserID:  g.UserID,
			   Asset:   g.Asset,
			   SnapGen: g.SnapGen,
			  Kind:    DriftKindWaterline,
			  Book:    driftBook,
			  Detail: fmt.Sprintf("单侧科目 %s 漂移，疑似跨科目操作中间态", driftBook),
			  CreatedAt: now,
		  }
		  return true, proof, nil
	   }
	   return false, nil, nil
  }

  func (g *DriftGuard) observeCurrent(ctx context.Context) ([]WaterlineObservation, error) {
	  if g.querier != nil {
		   return g.querier.ObserveWaterlines(ctx, g.UserID, g.Asset)
	   }
	   out := make([]WaterlineObservation,
		   0, len(g.BookSnaps))
	  for _, bs := range g.BookSnaps {
		   out = append(out, WaterlineObservation{
			   Book:      bs.Book,
			  Waterline: bs.Waterline,
			  ObserveGen: bs.ObserveGen,
			   Checksum:   bs.Checksum,
		   })
	  }
	   return out, nil
  }

// DriftGuardChecksum 守卫状态指纹。

   func DriftGuardChecksum(guard *DriftGuard) string {
	   h := sha256.New()
	   h.Write([]byte(guard.LeaseID))
	  h.Write([]byte{0})
	  h.Write([]byte(guard.UserID))
	   h.Write([]byte{0})
	  h.Write([]byte(guard.Asset))
	   h.Write([]byte{0})
	   for _, bs := range guard.BookSnaps {
		   h.Write([]byte(bs.Book))
		   h.Write([]byte{0})
		   h.Write([]byte(bs.Waterline))
		  h.Write([]byte{0})
		  h.Write([]byte(bs.Checksum))
		  h.Write([]byte{0})
	  }
	   return hex.EncodeToString(h.Sum(nil))
   }

// BookPairKey 科目识。
   func BookPairKey(a, b string) string {
	   a = strings.ToLower(strings.TrimSpace(a))
	   b = strings.ToLower(strings.TrimSpace(b))
	  if a > b {
		   a, b = b, a
	   }
	   return a + "|" + b
   }

// IsHopPair 判断是否为典型跨科目跳转对。
   func IsHopPair(source, target string) bool {
	   pairs := map[string]bool{
		   "funding|trading": true,
		   "funding|margin":  true,
		   "funding|futures": true,
		   "earn|funding": true,
		   "margin|trading":  true,
		  "futures|trading": true,
	   }
	   return pairs[BookPairKey(source, target)]
   }
