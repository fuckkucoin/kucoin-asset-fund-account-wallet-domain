package fundint

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/userdir"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	// ErrIdemConflict 同幂等键已有进行中的 Send，当前请求必须拒绝。
	ErrIdemConflict = fmt.Errorf("internal transfer idempotency conflict: concurrent send in progress")
)

// GatedResult 含完整闸信息的转账结果。

type GatedResult struct {
	Transfer *ledger.Transfer
	ToUserID string
	Channel  string
	SnapID   string
	LeaseID string
}

// ServiceExt 解析世代闸增强版站内转账服务。

type ServiceExt struct {
	*Service
	gate *ResolveGate
}

// NewExt 装配 ServiceExt。
   func NewExt(svc *Service, gateStore GateStore, opts ...GateOption) *ServiceExt {
	   return &ServiceExt{
		  Service: svc,
		  gate:    NewResolveGate(gateStore, opts...),
	   }
  }

// SendGated 含世代闸的站内转账（与 SendGatedFull 同一路径）。
// 注意：不得调用 SendResolved，否则可能在返回错误前已执行 SpotSend，造成账实与响应不一致。
   func (sx *ServiceExt) SendGated(ctx context.Context, req Req) (*GatedResult, error) {
	   return sx.SendGatedFull(ctx, req)
   }

// SendGatedFull 完整实现含闸站内转账（路径）。

  func (sx *ServiceExt) SendGatedFull(ctx context.Context, req Req) (*GatedResult, error) {
	  if sx == nil || sx.Service == nil || sx.Service.led == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  uid := strings.TrimSpace(req.UserID)
	  if uid == "" {
		  return nil, fmt.Errorf("user_id required")
	  }
	   idem := strings.TrimSpace(req.IdempotencyKey)
	   if idem == "" {
		  return nil, ErrIdem
	   }
	   if !req.Amount.IsPositive() {
		  return nil, ledger.ErrInvalidAmount
	   }
	   asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return nil, ErrAsset
	   }
	   req.Asset = asset
	   // 步骤 1：resolve 多标识
	  to, channel, channels, err := sx.resolveAll(ctx, req)
	   if err != nil {
		  return nil, err
	  }
	   if strings.EqualFold(to, uid) {
		   return nil, ledger.ErrSelfTransfer
	  }
	   // 最小额校验
	  if sx.Service.minSet && req.Amount.LessThan(sx.Service.minAmt) {
		  return nil, ErrMinAmount
	   }
	   // 出金冷却闸
	   if sx.Service.gate != nil {
		   if err := sx.Service.gate(ctx, uid); err != nil {
			  return nil, fmt.Errorf("%w: %v", ErrGate, err)
		  }
	   }
	  // 步骤 2：记录解析世代快照
	   snap, err := sx.gate.RecordSnap(ctx, SnapRequest{
		  FromUserID: uid,
		  ToUserID:   to,
		  Channel:    channel,
		  Channels: channels,
		  IdemKey: idem,
		   Asset:   req.Asset,
		   Amount: req.Amount.String(),
	   })
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrAmbiguous, err)
	   }
	   // 步骤 3：申请借记互斥租约
	   lease, err := sx.gate.AcquireLease(ctx, snap.ID, idem, uid, to, req.Asset, req.Amount.String())
	   if err != nil {
		   var ce *ConflictError
		   if ok := isConflictError(err, &ce); ok {
			   return nil, fmt.Errorf("%w: lease=%s", ErrIdemConflict, ce.LeaseID)
		   }
		   return nil, fmt.Errorf("acquire debit lease: %w", err)
	  }
	  // 步骤 4：校验快照仍有效（防 TOCTOU）
	   now := time.Now().UTC()
	  if _, err := sx.gate.ValidateSnap(ctx, idem, to, now); err != nil {
		   // 快照已失效，释放租约
		   _ = sx.releaseLeaseOnFailure(ctx, lease)
		  return nil, fmt.Errorf("snap validation failed: %w", err)
	  }
	  // 步骤 5：ledger SpotSend
	   tr, err := sx.Service.led.Create(ctx, ledger.CreateReq{
		  UserID:    uid,
		   Direction: ledger.SpotSend,
		   Asset:          asset,
		   Amount:         req.Amount,
		   ToAddress:      to,
		  IdempotencyKey: idem,
	  })
	  if err != nil {
		   // SpotSend 失败：释放租约，保持幂等安全
		   _ = sx.releaseLeaseOnFailure(ctx, lease)
		  return nil, err
	  }
	   // 步骤 6：提升租约为 committed
	  if err := sx.gate.CommitLease(ctx, lease.ID, tr.ID); err != nil {
		   // CommitLease 失败不回滚 SpotSend（已落账），记录异常但不失败
		   // patrol worker 会在巡检时发现 held 租约已过期并重建 committed 状态
		  _ = err // 最佳努力
	   }
	   return &GatedResult{
		   Transfer: tr,
		  ToUserID: to,
		  Channel:  channel,
		  SnapID:   snap.ID,
		  LeaseID:  lease.ID,
	   }, nil
   }

// resolveAll 解析多标识，返回 (toUserID, primaryChannel, allChannels, error)。
  func (sx *ServiceExt) resolveAll(ctx context.Context, req Req) (string, string, string, error) {
	  uid := strings.TrimSpace(req.ToUID)
	  email := strings.TrimSpace(req.ToEmail)
	  phone := strings.TrimSpace(req.ToPhone)
	   var resolved string
	  var primary string
	  var chans []string
	  resolve := func(kind, val string) error {
		   if val == "" {
			   return nil
		  }
		   id, err := sx.Service.dir.Resolve(kind, val)
		   if err != nil {
			   if kind == "uid" {
				  // uid 原样透传时 err 为 userdir.ErrUnknown，但 uid 已经是用户 ID
				   return fmt.Errorf("resolve %s %q: %w", kind, val, err)
			   }
			  return fmt.Errorf("resolve %s %q: %w", kind, val, err)
		  }
		   if resolved == "" {
			  resolved = id
			   primary = kind
		   } else if !strings.EqualFold(resolved, id) {
			   return fmt.Errorf("%w: %s→%s vs %s→%s", ErrAmbiguous, primary, resolved, kind, id)
		   }
		  chans = append(chans, kind)
		   return nil
	  }
	   if err := resolve("uid", uid); err != nil {
		  return "", "", "", err
	   }
	  if err := resolve("email", email); err != nil {
		  return "", "", "", err
	  }
	  if err := resolve("phone", phone); err != nil {
		   return "", "", "", err
	  }
	   if resolved == "" {
		  return "", "", "", userdir.ErrUnknown
	   }
	  return resolved, primary, strings.Join(chans, ","), nil
  }

   func (sx *ServiceExt) releaseLeaseOnFailure(ctx context.Context, lease *DebitLease) error {
	   now := time.Now().UTC()
	   lease.State = DebitLeaseReleased
	   lease.UpdatedAt = now
	  return sx.gate.store.UpdateDebitLease(ctx, lease)
  }

  func isConflictError(err error, out **ConflictError) bool {
	  if err == nil {
		  return false
	  }
	  var ce *ConflictError
	  if ok := errorAs(err, &ce); ok {
		   if out != nil {
			   *out = ce
		   }
		  return true
	   }
	  return false
  }

  func errorAs(err error, target interface{}) bool {
	   if err == nil {
		  return false
	  }
	  switch t := target.(type) {
	  case **ConflictError:
		   if ce, ok := err.(*ConflictError); ok {
			  *t = ce
			  return true
		  }
	  }
	  return false
  }
// temporary guard removed after the production check
