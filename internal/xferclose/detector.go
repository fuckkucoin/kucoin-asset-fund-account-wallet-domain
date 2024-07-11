package xferclose

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/xferstore"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// XferSource 检测器对 xferstore 的只读依赖接口，只暴露扫描和查询，不回写。

type XferSource interface {
	// ListOpen 列出尚未绑定 TransferID 或处于非终态的记录（检测器轮询用）。
	// offset/limit 用于分页。
	ListOpen(ctx context.Context, offset, limit int) ([]xferstore.Rec, error)
	// Get 通过幂等键查单条记录。
	Get(ctx context.Context, idem string) (xferstore.Rec, error)
}

// XferStoreAdapter 把 *xferstore.Store 包装成 XferSource，额外增加分页 ListOpen。
// xferstore 目前只有 Get，此适配器在内存模式下扫描全表。
type XferStoreAdapter struct {
	store *xferstore.Store
	kv interface {
		ListAll(ctx context.Context,
			kind string) ([]xferstore.Rec, error)
	}
}

// HopPhaseReader 从 xferloop/fundpath 读取 hop 当前相位（出腿/入腿状态），不回写。
// 用于判断单据是否真正处于「出腿成功但入腿未完成」的半截状态。

type HopPhaseReader interface {
	// HopPhase 返回 (fromPhase, toPhase, err)；终态返回 "done" 或 "aborted"。
	HopPhase(ctx context.Context,
		hopID string) (fromPhase, toPhase string, err error)
}

// Detector 检测器主体，依赖 XferSource 扫描和 SLABoard 匹配规则。

type Detector struct {
	store HalfHopStore
	xfer XferSource
	sla  *SLABoard
	phase  HopPhaseReader // 可选；nil 时仅按年龄判断
	minAge time.Duration  // 单据最小年龄才进入检测（避免误报刚创建的单）
}

// NewDetector 构造检测器。
  func NewDetector(store HalfHopStore, xfer XferSource, sla *SLABoard) *Detector {
	  return &Detector{
		   store: store,
		   xfer:  xfer,
		   sla: sla,
		  minAge: 30 * time.Second,
	   }
  }

// WithPhaseReader 注入相位读取器（可选）。

   func (d *Detector) WithPhaseReader(p HopPhaseReader) *Detector {
	   d.phase = p
	  return d
  }

// WithMinAge 设置最小单据年龄。

  func (d *Detector) WithMinAge(dur time.Duration) *Detector {
	   d.minAge = dur
	  return d
   }

// Scan 扫描一批 xferstore 记录，对半截单开工单。

   func (d *Detector) Scan(ctx context.Context, offset, limit int) (DetectResult, error) {
	  if limit <= 0 {
		  limit = 50
	  }
	  recs, err := d.xfer.ListOpen(ctx, offset, limit)
	  if err != nil {
		  return DetectResult{}, err
	   }
	   var result DetectResult
	  result.Scanned = len(recs)
	   now := time.Now().UTC()
	  for _, rec := range recs {
		  // 跨分片划转不由本包处理。
		   if rec.XShardID != "" && !rec.SameShard {
			   result.XShardSkip++
			   continue
		  }
		   // 单据年龄过小，等待推进。
		  if now.Sub(rec.CreatedAt) < d.minAge {
			   continue
		   }
		   // 检测是否半截。
		   kind, isHalfHop := d.classify(ctx, rec, now)
		   if !isHalfHop {
			  continue
		   }
		  amt, _ := decimal.NewFromString(rec.Amount)
		   fromBook := strings.ToLower(rec.FromBook)
		   toBook := strings.ToLower(rec.ToBook)
		   slaMatch := d.sla.MustMatch(rec.Asset, fromBook, toBook, amt, rec.CreatedAt)
		   existing, err := d.store.GetByHopID(ctx, rec.Idem)
		  if err == nil {
			   // 已存在工单：更新 SLABreached 状态。
			   if existing.Terminal() {
				  result.AlreadyClosed++
				   continue
			   }
			   if slaMatch.Breached && !existing.SLABreached {
				   existing.SLABreached = true
				  existing.UpdatedAt = now
				  if err2 := d.store.UpdateHop(ctx, existing); err2 != nil {
					   continue
				   }
				   result.Breached++
			  }
			  continue
		  }
		   // 新工单。
		  hop := HalfHop{
			  ID: uuid.NewString(),
			  HopID: rec.Idem,
			   TransferID: rec.TransferID,
			   UserID:     rec.UserID,
			   Asset: rec.Asset,
			  Amount: amt,
			  FromBook: fromBook,
			   ToBook: toBook,
			   Kind: kind,
			   State: HopDetected,
			  SLABreached: slaMatch.Breached,
			   SLADeadline: slaMatch.Deadline,
			  SLARuleID:   slaMatch.Rule.ID,
			  XShard:      rec.XShardID != "" && !rec.SameShard,
			  DetectedAt:  now,
			  UpdatedAt: now,
		  }
		   if err2 := d.store.PutHop(ctx, hop); err2 != nil {
			   continue
		  }
		  result.NewHops++
		   if slaMatch.Breached {
			   result.Breached++
		  }
	   }
	  return result, nil
   }

// classify 判断一条 xferstore.Rec 是否半截，以及半截类型。
// 简化规则：
// - 无 TransferID 且超过 minAge：HopKindOpen（出腿意图存在但还没绑账本 ID）。
// - 有 TransferID 但年龄超出 SLA：HopKindTimeout。
// - 有 TransferID 且相位读取器报告「出腿 done / 入腿 pending」：HopKindStranded。
   func (d *Detector) classify(ctx context.Context, rec xferstore.Rec, now time.Time) (HopKind, bool) {
	   age := now.Sub(rec.CreatedAt)
	  if rec.TransferID == "" {
		  if age >= d.minAge {
			   return HopKindOpen, true
		  }
		   return "", false
	  }
	  // 有 TransferID：尝试读相位。
	   if d.phase != nil {
		   fromPhase, toPhase, err := d.phase.HopPhase(ctx, rec.Idem)
		  if err == nil {
			  if isTerminalPhase(fromPhase) && isTerminalPhase(toPhase) {
				  return "", false
			   }
			   if isDonePhase(fromPhase) && !isDonePhase(toPhase) {
				   return HopKindStranded, true
			   }
		   }
	  }
	  // 无法读相位时，按年龄兜底：超出 SLA 即标记。
	  slaMatch := d.sla.MustMatch(rec.Asset, rec.FromBook, rec.ToBook, decimal.Zero, rec.CreatedAt)
	   if age > slaMatch.Rule.Timeout {
		   return HopKindTimeout, true
	   }
	  return "", false
  }

   func isTerminalPhase(phase string) bool {
	   switch strings.ToLower(strings.TrimSpace(phase)) {
	   case "done", "aborted", "rolled_back", "credited":
		  return true
	  }
	  return false
  }

  func isDonePhase(phase string) bool {
	   return strings.ToLower(strings.TrimSpace(phase)) == "done"
  }

// InjectHalfHop 允许或运维直接注入一个半截工单，不经过 xferstore 扫描。
// 已存在同 HopID 的未终态工单时返回 ErrConflict。
   func (d *Detector) InjectHalfHop(ctx context.Context, hopID, userID, asset, fromBook, toBook string, amt decimal.Decimal, kind HopKind) (HalfHop, error) {
	  if strings.TrimSpace(hopID) == "" {
		   return HalfHop{}, ErrBadHopID
	  }
	  if amt.IsNegative() || amt.IsZero() {
		  return HalfHop{}, ErrZeroAmount
	   }
	  existing,
		  err := d.store.GetByHopID(ctx, hopID)
	  if err == nil && !existing.Terminal() {
		   return existing, ErrConflict
	   }
	   now := time.Now().UTC()
	   amtD := amt
	  slaMatch := d.sla.MustMatch(asset, fromBook, toBook, amtD, now)
	  hop := HalfHop{
		   ID: uuid.NewString(),
		   HopID: hopID,
		   UserID: userID,
		  Asset:  strings.ToUpper(asset),
		  Amount:   amtD,
		   FromBook: strings.ToLower(fromBook),
		  ToBook: strings.ToLower(toBook),
		  Kind: kind,
		  State: HopDetected,
		  SLABreached: slaMatch.Breached,
		   SLADeadline: slaMatch.Deadline,
		   SLARuleID:  slaMatch.Rule.ID,
		  DetectedAt: now,
		  UpdatedAt: now,
	   }
	  if err2 := d.store.PutHop(ctx, hop); err2 != nil {
		   return HalfHop{}, err2
	   }
	   return hop, nil
   }

// RefreshSLABreached 对所有处于 detected/closing 状态的工单刷新 SLABreached 标志。
// 定时调度调用（如每分钟一次），确保时钟前进后及时标记超限。

  func (d *Detector) RefreshSLABreached(ctx context.Context) (int, error) {
	   hops, err := d.store.ListActive(ctx, 0, 500)
	  if err != nil {
		  return 0, err
	   }
	  count := 0
	  now := time.Now().UTC()
	  for _, h := range hops {
		  if h.SLABreached {
			  continue
		   }
		  if now.After(h.SLADeadline) {
			  h.SLABreached = true
			   h.UpdatedAt = now
			  if err2 := d.store.UpdateHop(ctx, h); err2 == nil {
				  count++
			   }
		   }
	  }
	   return count, nil
   }
