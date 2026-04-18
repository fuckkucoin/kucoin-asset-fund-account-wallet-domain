// 功能：出金编排，幂等登记 → 核验闸门 → ledger.Create（冻→白名单/解密→KYT→签章→广播）。
// 作者：Talon
// 日期：2020.11.03

package withdraw

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/idemstore"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/vipfee"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Service struct {
	led *ledger.Service
	pipe *compliance.Pipeline
	cmds *command.Log
	book *Book
	bc       *Broadcaster
	bump     *Bumper
	nonces   *NonceBook
	vip      *vipfee.Service
	haltGate func(context.Context) error
	holds    *holdcat.Book
	idem idemstore.Store
	freezeLog   *FreezeEventWriter
	reg         *chainad.Registry // 抬价/广播 Registry 装配
	feeBumpSign FeeBumpSigner     // custody/HSM 抬价联签
	travelGate TravelGate
	hotPre   func(context.Context, string) error
	conserve *Conserve
	abuse    *Abuse
	fcGate   FundCircuitGate
	tickets *Desk
	idemIdx *idemIndex
	audits  *auditLog
	phases  *phaseBook
	release releaseGate
	reserveAdmit ReserveAdmit
	limit        LimitAdmitter
	limitHolds  map[string]string
	recoverHook func(context.Context,
		  string) error
}

  func New(led *ledger.Service) *Service {
	  return NewWith(led, nil)
   }

   func NewWith(led *ledger.Service, pipe *compliance.Pipeline) *Service {
	  if pipe == nil {
		   pipe = compliance.New(kyt.Static{Mode: kyt.Allow}, policy.NewRegistry(policy.DefaultV1()))
	  }
	   return &Service{
		   led: led, pipe: pipe, cmds: command.NewLog(), book: NewBook(), bc: NewBroadcaster(),
		  conserve: NewConserve(), abuse: NewAbuse(), tickets: NewDesk(),
		  idemIdx: newIdemIndex(),
		  audits:  newAuditLog(), phases: newPhaseBook(),
	  }
  }

// BindHotPrecheck 挂热库存只读闸。未绑定则不查热库存（账本 Create 仍有自己的库存检查）。
  func (s *Service) BindHotPrecheck(fn func(context.Context, string) error) *Service {
	  if s != nil {
		  s.hotPre = fn
	   }
	  return s
  }

   // Tickets 出金事故单台。

  func (s *Service) Tickets() *Desk {
	   if s == nil {
		  return nil
	   }
	   return s.tickets
   }

// TripAbuse 刷提跳闸。只记 wd: 键，不触碰在途冻结。

  func (s *Service) TripAbuse(key, reason string) (AbuseHit, error) {
	   if s == nil || s.abuse == nil {
		   return AbuseHit{}, ErrAbuseBlocked
	   }
	   return s.abuse.Trip(key, reason)
  }

// AbuseHits 当前刷提跳闸。
   func (s *Service) AbuseHits() []AbuseHit {
	   if s == nil || s.abuse == nil {
		  return nil
	   }
	  return s.abuse.Hits()
  }

// BindHalt 挂热钱包异常停提闸。未绑定则不检查（由上层 RequireWithdraw 覆盖公开面）。
  func (s *Service) BindHalt(fn func(context.Context) error) *Service {
	   if s != nil {
		   s.haltGate = fn
	   }
	   return s
   }

  // BindIdem 挂持久化幂等仓；未挂时仍用进程内 command.Log。

  func (s *Service) BindIdem(st idemstore.Store) *Service {
	  if s != nil {
		   s.idem = st
	   }
	  return s
  }

// BindFreezeLog 挂出金冻结跃迁审计。

  func (s *Service) BindFreezeLog(w *FreezeEventWriter) *Service {
	   if s != nil {
		  s.freezeLog = w
	   }
	   return s
   }

// BindHolds 挂冻结分类账。出金冻账后写入 pending_withdraw；取消时按 transfer id 释放。
   func (s *Service) BindHolds(h *holdcat.Book) *Service {
	  if s != nil {
		  s.holds = h
	  }
	   return s
   }

type Request struct {
	UserID string
	Asset  string
	Amount decimal.Decimal
	Chain  string
	ToAddress string
	Memo      string // FamMEMO 出金备注；落 memo_hint
	Tag       string // 与 Memo 同义别名；落 tag_hint
	TagName   string // memo / destination_tag / comment
	IdempotencyKey string
	WaitConfirm bool
	TicketID  string // 放行票据；大额没有票据则失败关闭
	DestClass string
	Risk      int
}

// Request 发起一笔出金。
// 链上出金申请；FamMEMO 备注落 transfers.memo_hint/tag_hint/tag_name_hint 供历史回显。
// Amount 须过 money.Parse；备注链须 ValidFamMemoTag（空备注 fail-closed）。
// 幂等 payload 含 memo，同 key 不同备注冲突；KYT deny 不进 Create。
   func (s *Service) Request(ctx context.Context, req Request) (*ledger.Transfer, error) {
	  if s == nil {
		   return nil, fmt.Errorf("withdraw ledger unavailable")
	   }
	  if err := s.book.writable(); err != nil {
		  return nil, err
	  }
	  if s.led == nil {
		   return nil, fmt.Errorf("withdraw ledger unavailable")
	  }
	   if s.haltGate != nil {
		  if err := s.haltGate(ctx); err != nil {
			   return nil, err
		   }
	  }
	  req.Chain = strings.ToUpper(strings.TrimSpace(req.Chain))
	  req.ToAddress = strings.TrimSpace(req.ToAddress)
	  req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	  // 资金域统一熔断闸：只读判定，跳闸时拒绝新申请，在途筛查/广播单不受影响。
	   if err := s.fundCircuit(ctx, req.UserID, req.Chain, req.Asset); err != nil {
		  return nil, err
	   }
	  req.Memo = strings.TrimSpace(req.Memo)
	   req.Tag = strings.TrimSpace(req.Tag)
	   req.TagName = strings.TrimSpace(req.TagName)
	   if req.ToAddress == "" || req.Chain == "" {
		  return nil, ledger.ErrMissingAddress
	  }
	  if _, err := money.Parse(req.Amount.String()); err != nil {
		   return nil, err
	  }
	  // 统一 memo/tag；缺省按链推导 tagName；校验 FamMEMO 形态。
	  memo := req.Memo
	  if memo == "" {
		  memo = req.Tag
	   }
	   tag := req.Tag
	   if tag == "" {
		   tag = memo
	  }
	   tagName := req.TagName
	   if tagName == "" {
		  tagName = wallet.FamMemoTagName(req.Chain)
	   }
	   if err := wallet.ValidFamMemoTag(req.Chain, memo); err != nil {
		  return nil, fmt.Errorf("%w: %v", ledger.ErrAddressFormat, err)
	   }
	  req.Memo, req.Tag, req.TagName = memo, tag, tagName
	   if err := s.admitReserve(ctx, PhaseSign, req.Asset, req.Amount.String(), ""); err != nil {
		  return nil, err
	   }
	  var limitHold string
	  var limitReused bool
	   if s.limit != nil {
		   verdict, lerr := s.limit.Admit(ctx, LimitAdmit{
			  UserID: req.UserID, Asset: req.Asset, Amount: req.Amount.String(),
			   Chain: req.Chain, Dest: req.ToAddress, Idem: req.IdempotencyKey,
		  })
		   if lerr != nil {
			  return nil, lerr
		   }
		  limitHold = verdict.HoldID
		   limitReused = verdict.Reused
	   }
	   payload := command.HashPayload("withdraw", req.UserID, req.Asset, req.Amount.String(), req.Chain, req.ToAddress, req.Memo, req.TagName)
	   fp := idemstore.Fingerprint(req.UserID, req.Asset, req.Amount.String(), req.Chain, req.ToAddress, req.Memo, req.TagName)
	  replay, err := s.prepareIdem(ctx, req, fp, payload)
	   if err != nil || replay != nil {
		  if err != nil && limitHold != "" && !limitReused && s.limit != nil {
			  _ = s.limit.ReleaseAdmit(ctx, limitHold, "withdraw precheck failed")
		   }
		   return replay, err
	   }
	  gate, gerr := s.pipe.GateWithdraw(ctx, compliance.GateReq{
		   UserID: req.UserID, Asset: req.Asset, Amount: req.Amount.String(),
		   Chain: req.Chain, ToAddress: req.ToAddress, Allowlisted: true,
	   })
	   if gerr != nil && !errors.Is(gerr, compliance.ErrReview) {
		  s.idemFail(ctx, req.IdempotencyKey, gerr)
		   if limitHold != "" && !limitReused && s.limit != nil {
			   _ = s.limit.ReleaseAdmit(ctx, limitHold, "compliance refused")
		   }
		  if errors.Is(gerr, compliance.ErrDenied) {
			  return nil, fmt.Errorf("%w: %s", ledger.ErrRiskRejected, gate.Reason)
		   }
		   if errors.Is(gerr, compliance.ErrUnavailable) {
			  return nil, fmt.Errorf("%w: %v", ledger.ErrKYTUnavailable, gerr)
		   }
		   return nil, gerr
	  }
	   if s.limit != nil && gate.Decision != "" && limitHold != "" {
		  _, _ = s.limit.Admit(ctx, LimitAdmit{
			  UserID: req.UserID, Asset: req.Asset, Amount: req.Amount.String(),
			  Chain: req.Chain, Dest: req.ToAddress, Idem: req.IdempotencyKey, KYTHint: string(gate.Decision),
		   })
	   }
	   if prior, ok := s.book.ResumeByIdem(req.IdempotencyKey); ok && prior.DebitOnce && prior.TransferID != "" {
		   return s.led.Get(ctx, prior.TransferID)
	  }
	   run := Run{
		   ID: uuid.NewString(),
		  UserID: req.UserID, Step: StepRequested, PolicyVer: gate.PolicyVer,
		   Asset: req.Asset, Amount: req.Amount.String(), IdemKey: req.IdempotencyKey,
		  Idem: req.IdempotencyKey, Chain: req.Chain,
	   }
	   if err := s.book.PutErr(run); err != nil {
		   s.idemFail(ctx, req.IdempotencyKey, err)
		  s.cmds.Fail(req.IdempotencyKey,
			   err)
		   if limitHold != "" && !limitReused && s.limit != nil {
			  _ = s.limit.ReleaseAdmit(ctx, limitHold, "withdraw book persist failed")
		   }
		  return nil, err
	  }
	   // 放行闸在冻账之前。没有票据或已授权额度不足则失败关闭，不得进入 ledger.Create。
	   // 闸失败不改冻结态，也不为链上 live hash 自动解冻。
	   var relGrant ReleaseGrant
	  relHeld := false
	  if s.release != nil {
		   g, rerr := s.release.BeforeFreeze(ctx, ReleaseAsk{
			   UserID: req.UserID, Asset: req.Asset, Amount: req.Amount.String(),
			  Chain: req.Chain, Dest: req.ToAddress, Idem: req.IdempotencyKey,
			  TicketID: req.TicketID, Risk: req.Risk, DestClass: req.DestClass,
		  })
		   if rerr != nil {
			   s.idemFail(ctx, req.IdempotencyKey, rerr)
			  return nil, rerr
		  }
		  relGrant = g
		  relHeld = true
	   }
	  tr, err := s.led.Create(ctx, ledger.CreateReq{
		   UserID: req.UserID, Direction: ledger.WalletWithdraw,
		   Asset: req.Asset, Amount: req.Amount, Chain: req.Chain,
		  ToAddress:      req.ToAddress,
		   IdempotencyKey: req.IdempotencyKey,
		  WaitConfirm: req.WaitConfirm,
		   Memo: req.Memo, Tag: req.Tag, TagName: req.TagName,
	   })
	   if relHeld {
		   if tr != nil {
			  _ = s.release.CommitFreeze(ctx, relGrant)
		  } else {
			   _ = s.release.AbortFreeze(ctx, relGrant)
		   }
	   }
	   if tr != nil {
		   run.TransferID = tr.ID
		   run.KYT = tr.KYTDecision
		   if err == nil || errors.Is(err, ledger.ErrKYTReview) {
			  s.rememberFreeze(req.IdempotencyKey, fp, tr.ID, req.UserID, req.Asset, req.Amount.String())
			  s.noteHotIntent(tr.ID, req.UserID, req.Asset, req.Amount.String(), req.Chain)
		   }
		  if s.holds != nil && tr.ID != "" {
			  _, _ = s.holds.PlacePendingWithdraw(ctx, req.UserID, req.Asset, req.Amount.String(), tr.ID)
		   }
		   if limitHold != "" && tr.ID != "" {
			  s.rememberLimit(tr.ID, limitHold)
		   }
		   _ = s.AdvanceFreeze(ctx, &run, FreezeHeld, "withdraw.Request", "funds held")
		   switch {
		   case errors.Is(err, ledger.ErrKYTReview):
			  run.Step = StepReview
			  run.DebitOnce = true
			  _ = s.AdvanceFreeze(ctx, &run, FreezeScreening, "withdraw.Request", "kyt review")
			  _, _ = s.pipe.OpenReview(tr.ID, req.UserID, "withdraw_review", gate.PolicyVer)
			   s.idemSucceed(ctx,
				   req.IdempotencyKey, tr.ID)
		  case err != nil:
			   run.Step = StepFailed
			  run.Err = err.Error()
			   _ = s.AdvanceFreeze(ctx, &run, FreezeFailed, "withdraw.Request", err.Error())
			  _ = s.book.FailSeal(run, err.Error())
			  s.idemFail(ctx, req.IdempotencyKey, err)
			  s.cmds.Fail(req.IdempotencyKey, err)
			   if limitHold != "" && s.limit != nil {
				  _ = s.limit.ReleaseAdmit(ctx, limitHold, "withdraw create failed")
			  }
			  return tr, err
		   case tr.State == ledger.StateCredited:
			   run.Step = StepConfirmed
			  run.DebitOnce = true
			  _ = s.MarkFreezePath(ctx, &run, []FreezeState{FreezeScreening, FreezeApproved, FreezeSigning, FreezeBroadcast, FreezeConfirming, FreezeSucceeded}, "withdraw.Request", "sync credited")
			   s.dropHotIntent(tr.ID)
			  s.idemSucceed(ctx, req.IdempotencyKey, tr.ID)
			   s.cmds.Succeed(req.IdempotencyKey, tr.ID)
			  _ = s.pipe.NoteUsage(req.UserID, req.Asset, req.ToAddress, req.Amount.String())
		   case tr.State == ledger.StateChainSubmitted || tr.State == ledger.StateConfirmed:
			  run.Step = StepSubmitted
			   run.DebitOnce = true
			  path := []FreezeState{FreezeScreening, FreezeApproved, FreezeSigning, FreezeBroadcast}
			  if tr.State == ledger.StateConfirmed {
				   path = append(path, FreezeConfirming)
			  }
			  _ = s.MarkFreezePath(ctx, &run, path, "withdraw.Request", "sync submitted")
			   s.idemSucceed(ctx, req.IdempotencyKey, tr.ID)
			  s.cmds.Succeed(req.IdempotencyKey, tr.ID)
			   _ = s.pipe.NoteUsage(req.UserID, req.Asset, req.ToAddress, req.Amount.String())
		   default:
			   run.Step = StepFrozen
			   run.DebitOnce = true
			   s.idemSucceed(ctx, req.IdempotencyKey, tr.ID)
			   s.cmds.Succeed(req.IdempotencyKey, tr.ID)
			   _ = s.pipe.NoteUsage(req.UserID, req.Asset, req.ToAddress, req.Amount.String())
		  }
		   if perr := s.book.PutErr(run); perr != nil {
			  return tr, perr
		   }
	   } else if err != nil {
		  s.idemFail(ctx, req.IdempotencyKey, err)
		  if limitHold != "" && s.limit != nil {
			   _ = s.limit.ReleaseAdmit(ctx, limitHold, "withdraw create failed")
		  }
	  }
	  return tr, err
  }

   func (s *Service) Cases() *cases.Service { return s.pipe.Cases() }

  func (s *Service) Pipeline() *compliance.Pipeline { return s.pipe }

   func (s *Service) Runs() *Book { return s.book }

// Confirm 把已提交的出金再走 ConfirmAndCredit。链未达深度返回 pending，留给 Recover 重试。

  func (s *Service) Confirm(ctx context.Context, id string) error {
	  if s == nil || s.led == nil {
		  return fmt.Errorf("withdraw ledger unavailable")
	  }
	  return s.led.ConfirmAndCredit(ctx, id)
  }

  func (s *Service) Get(ctx context.Context, id string) (*ledger.Transfer, error) {
	   if s == nil || s.led == nil {
		  return nil, fmt.Errorf("withdraw ledger unavailable")
	   }
	  return s.led.Get(ctx, id)
   }
