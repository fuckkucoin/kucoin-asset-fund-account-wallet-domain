package acctpol

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Registry 策略代次注册中心（封装 Store 的业务操作）。
type Registry struct {
	store HopStore
	clock func() time.Time
}

// NewRegistry 创建策略代次注册中心。
  func NewRegistry(store HopStore) *Registry {
	  return &Registry{store: store, clock: time.Now}
   }

// WithClock 替换时钟（供注入）。

   func (r *Registry) WithClock(fn func() time.Time) *Registry {
	   r.clock = fn
	   return r
  }

  // ─── 发布请求 / 回复 ──────────────────────────────────────────────────────
  // PublishRequest 发布新策略代次的请求参数。
   type PublishRequest struct {
	  UserID string // 空 = 全局代次
	   Asset  string // 空 = 全资产
	   AllowBizKinds []HopBizKind // 允许的 HopBizKind（空=全允许）
	  DenyBizKinds []HopBizKind // 拒绝的 HopBizKind（先于 allow）
	  RequireToken bool         // 是否强制要求 HopToken
	   MaxSingleAmt string // 单笔上限
	  DailyLimit  string // 日限额
	   PublishedBy string // 操作人
   }

   // PublishReply 发布结果。
  type PublishReply struct {
	   VersionID     string `json:"version_id"`
	   Gen           int64  `json:"gen"`
	  SupersededGen int64  `json:"superseded_gen,omitempty"` // 被取代的旧 Gen（若有）
  }

  // Publish 发布新全局策略代次：
   // 1. 分配新 Gen；
   // 2. 将旧 active 全局代次标记为 superseded；
   // 3. 插入新 active 代次。
   // 注意：Publish 后 Gate 的下一次 Check 会读到新 Gen；
  // 已签发但尚未消费的 HopToken 持有旧 Gen，复验时会因 ErrGenMismatch 被拒绝。
  func (r *Registry) Publish(ctx context.Context, req PublishRequest) (*PublishReply, error) {
	  if req.PublishedBy == "" {
		  return nil, fmt.Errorf("acctpol publish: published_by required")
	   }
	   gen, err := r.store.NextGen(ctx)
	   if err != nil {
		  return nil, fmt.Errorf("acctpol publish: next gen: %w", err)
	   }
	  now := r.clock()
	  reply := &PublishReply{Gen: gen}
	  // 将旧全局 active 代次 Supersede（若存在）
	   old, err := r.store.GetActiveGlobalVersion(ctx, req.Asset)
	  if err == nil && old != nil {
		  if transErr := old.Transition(PublishStateSuperseded); transErr == nil {
			  old.State = PublishStateSuperseded
			  old.SupersededAt = &now
			   old.UpdatedAt = now
			   if updateErr := r.store.UpdateVersion(ctx, old); updateErr != nil {
				  return nil, fmt.Errorf("acctpol publish: supersede old version (gen=%d): %w", old.Gen, updateErr)
			  }
			   reply.SupersededGen = old.Gen
		  }
	   }
	  // 创建新 active 代次
	   newVer := &HopPolicyVersion{
		   ID: uuid.NewString(),
		   UserID: req.UserID,
		  Asset: req.Asset,
		  Gen:   gen,
		  State: PublishStateActive,
		   AllowBizKinds: req.AllowBizKinds,
		   DenyBizKinds: req.DenyBizKinds,
		   RequireToken: req.RequireToken,
		  MaxSingleAmt: req.MaxSingleAmt,
		   DailyLimit:   req.DailyLimit,
		   PublishedBy: req.PublishedBy,
		   PublishedAt: &now,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   newVer.DocHash = computeDocHash(newVer)
	   if err := r.store.InsertVersion(ctx, newVer); err != nil {
		   return nil, fmt.Errorf("acctpol publish: insert version: %w", err)
	   }
	  reply.VersionID = newVer.ID
	   return reply, nil
   }

// PinRequest 钉版请求参数。
type PinRequest struct {
	VersionID string // 要钉的代次 ID（必须是 active 状态）
	UserID   string // 钉版用户（个人覆盖；空=全局）
	PinnedBy string // 操作人
}

// PinReply 钉版结果。
type PinReply struct {
	VersionID string `json:"version_id"`
	Gen     int64 `json:"gen"`      // 新分配的世代号（Unpin 后与 Pin 时一致）
	PrevGen int64 `json:"prev_gen"` // 钉版前的世代号
}

// Pin 将指定代次升为 pinned：
// 1. 分配新 Gen（Pin 动作本身也是世代事件）；
// 2. 将旧 pinned 代次（若存在）回退为 active；
// 3. 将目标代次标记为 pinned。

  func (r *Registry) Pin(ctx context.Context, req PinRequest) (*PinReply, error) {
	  if req.VersionID == "" {
		  return nil, fmt.Errorf("acctpol pin: version_id required")
	   }
	   if req.PinnedBy == "" {
		   return nil, fmt.Errorf("acctpol pin: pinned_by required")
	   }
	  ver, err := r.store.GetVersionByID(ctx, req.VersionID)
	   if err != nil {
		   return nil, fmt.Errorf("acctpol pin: get version: %w", err)
	   }
	   if err := ver.Transition(PublishStatePinned); err != nil {
		  return nil, err
	   }
	  // 释放旧 pinned（若有），不分配新 Gen，只改状态
	   uid := req.UserID
	   if uid == "" {
		  uid = ver.UserID
	  }
	   if uid != "" {
		  oldPinned, pinErr := r.store.GetPinnedVersionByUser(ctx, uid, ver.Asset)
		  if pinErr == nil && oldPinned != nil && oldPinned.ID != ver.ID {
			  if transErr := oldPinned.Transition(PublishStateActive); transErr == nil {
				   now := r.clock()
				  oldPinned.State = PublishStateActive
				   oldPinned.PinnedBy = ""
				   oldPinned.PinnedAt = nil
				  oldPinned.UpdatedAt = now
				  _ = r.store.UpdateVersion(ctx, oldPinned)
			   }
		   }
	  }
	  // 分配新世代
	   gen, err := r.store.NextGen(ctx)
	  if err != nil {
		   return nil, fmt.Errorf("acctpol pin: next gen: %w", err)
	  }
	   now := r.clock()
	  prevGen := ver.Gen
	  ver.Gen = gen
	  ver.State = PublishStatePinned
	  ver.UserID = uid
	  ver.PinnedBy = req.PinnedBy
	   ver.PinnedAt = &now
	   ver.UpdatedAt = now
	  ver.DocHash = computeDocHash(ver)
	   if err := r.store.UpdateVersion(ctx, ver); err != nil {
		  return nil, fmt.Errorf("acctpol pin: update version: %w", err)
	   }
	  return &PinReply{VersionID: ver.ID, Gen: gen, PrevGen: prevGen}, nil
  }

// UnpinRequest 卸钉请求参数。

type UnpinRequest struct {
	UserID string // 卸钉用户（空=全局）
	Asset string // 币种（空=全资产）
	UnpinnedBy string
}

// Unpin 卸钉：将当前 pinned 代次降回 active，不改 Gen（世代不回滚）。
   func (r *Registry) Unpin(ctx context.Context, req UnpinRequest) error {
	  ver,
		   err := r.store.GetPinnedVersionByUser(ctx, req.UserID, req.Asset)
	   if err != nil {
		  return fmt.Errorf("acctpol unpin: %w", err)
	   }
	   if err := ver.Transition(PublishStateActive); err != nil {
		  return err
	   }
	  now := r.clock()
	   ver.State = PublishStateActive
	  ver.PinnedBy = ""
	   ver.PinnedAt = nil
	  ver.UpdatedAt = now
	  return r.store.UpdateVersion(ctx, ver)
  }

// ─── 工具函数 ─────────────────────────────────────────────────────────────
// computeDocHash 计算策略代次的内容指纹（防篡改）。
// hash = SHA-256(userID ‖ ":" ‖ asset ‖ ":" ‖ allowList ‖ ":" ‖ denyList ‖ ":" ‖ Gen)
   func computeDocHash(v *HopPolicyVersion) string {
	   allows := make([]string, len(v.AllowBizKinds))
	  for i, k := range v.AllowBizKinds {
		  allows[i] = string(k)
	   }
	   denies := make([]string, len(v.DenyBizKinds))
	  for i, k := range v.DenyBizKinds {
		   denies[i] = string(k)
	   }
	  raw := fmt.Sprintf("%s:%s:%s:%s:%d",
		  v.UserID, v.Asset,
		   strings.Join(allows, ","),
		  strings.Join(denies, ","),
		   v.Gen,
	  )
	   h := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", h)
   }
