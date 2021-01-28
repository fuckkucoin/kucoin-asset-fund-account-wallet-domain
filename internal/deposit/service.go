// 充值，先在事务外 Inspect，再按观察金额建单。

// Author: Cason
// Date: 2021.01.08

package deposit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/google/uuid"
)

type Observer interface {
	Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error)
}

type Service struct {
	led *ledger.Service
	obs  Observer
	cmds *command.Log
	book *Book
	match *Matcher
	attr  *Attributor
	delay *DelayBook
	pol *chainad.Policies
	haltGate     func(context.Context) error
	kytScreen    kyt.Screener
	travel       *travelvasp.Service
	originVASP   string
	peerVASP     string
	travelThresh *travelvasp.ThresholdBook
	desk *Desk
	patrol  *Patrol
	slots   *Slots
	roll    *RollBook
	circuit *AbuseCircuit
	fcGate FundCircuitGate
	recover   *RecoverDesk
	recon     *recon.Book
	lastRecon string
	attribOn  atomic.Bool
	attribEng AttributionGate
	attribOnce sync.Once
	dustGuard  DustGuardChecker // 可选；由 dustguard 注入，CheckObserve 挡污染贷记
	occ         OccupancyNote
	recoverHook func(context.Context, string) error
	creditedSide *CreditedSide
}

   func New(led *ledger.Service, obs Observer) *Service {
	  s := &Service{led: led,
		   obs: obs, cmds: command.NewLog(), book: NewBook(), match: NewMatcher(nil)}
	  s.ensureGuards()
	  return s
  }

   func (s *Service) WithPolicies(p *chainad.Policies) *Service {
	  if s != nil {
		   s.pol = p
	  }
	  return s
  }

// BindHalt 挂热钱包异常全局闸。事故停充停提时 Observe/Confirm 与出金共用同一 KV 标志。

  func (s *Service) BindHalt(fn func(context.Context) error) *Service {
	   if s != nil {
		  s.haltGate = fn
	   }
	   return s
   }

   // BindDesk 挂入金异常台；Observe 前 BlocksCreditScoped，漂移时 OpenException(asset)。

  func (s *Service) BindDesk(d *Desk) *Service {
	  if s != nil {
		   s.desk = d
		   if s.patrol != nil {
			   s.patrol.BindDesk(d)
		   }
	   }
	   return s
  }

  func (s *Service) Desk() *Desk {
	   if s == nil {
		   return nil
	   }
	   return s.desk
   }

  func (s *Service) checkHalt(ctx context.Context) error {
	  if s != nil && s.haltGate != nil {
		  return s.haltGate(ctx)
	   }
	  return nil
   }

// BindDustGuard 挂载粉尘防护钩子；Observe 前检查地址是否污染。
// 需在 dust-guard worker 启动时调用；不设置时 Observe 不做防护检查。
   func (s *Service) BindDustGuard(g DustGuardChecker) {
	  if s != nil {
		   s.dustGuard = g
	   }
  }

type ObserveReq struct {
	UserID string
	Chain  string
	TxHash string
	Asset          string
	IdempotencyKey string
	WaitConfirm    bool
	Memo string
	Tag string
	UTXO string
	Tier string
	Network     string
	OutputIndex int
}

// Observe 认领一笔链上入金。
// 入参：UserID、Chain、TxHash、Asset、IdempotencyKey。金额不从请求读。
// 出参：Transfer；观察资产对不上返回 ErrObservationMismatch；零额拒。
// 顺序：Inspect（账本事务外）→ 用 observed amount 做幂等 payload → ledger.Create。
// 同一 (chain, tx_hash, output_index) 第二次进 ledger 是 ErrDuplicateHash。成功 key 回放原单。
  func (s *Service) Observe(ctx context.Context, req ObserveReq) (*ledger.Transfer, error) {
	  if s == nil || s.led == nil {
		   return nil, fmt.Errorf("deposit ledger unavailable")
	   }
	  if err := s.checkHalt(ctx); err != nil {
		   return nil, err
	   }
	   req.Chain = strings.ToUpper(strings.TrimSpace(req.Chain))
	  req.TxHash = strings.TrimSpace(req.TxHash)
	  req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	  if req.TxHash == "" {
		  return nil, fmt.Errorf("tx_hash required")
	  }
	   if req.Chain == "" || req.Asset == "" {
		   return nil, fmt.Errorf("chain and asset required")
	  }
	   if s.obs == nil {
		   return nil, fmt.Errorf("chain observer unavailable")
	   }
	  seen, err := s.obs.Inspect(ctx, req.Chain, req.TxHash)
	  if err != nil {
		   return nil, err
	   }
	   if seen.Reorged {
		  s.openDesk(req.UserID, req.Chain, req.Asset, req.TxHash, TicketReorgHold, "observation reorged")
		   return nil, chainad.ErrReorged
	  }
	  if s.pol != nil {
		  if err := s.pol.Check(req.Chain, req.Asset, seen.Confirmations); err != nil {
			  return nil, err
		  }
	  }
	  if seen.Amount.IsZero() {
		   return nil, fmt.Errorf("observed amount is zero")
	  }
	   if seen.Asset != "" && seen.Asset != req.Asset {
		  s.openDesk(req.UserID, req.Chain, req.Asset, req.TxHash, TicketHashDrift, "observed asset "+seen.Asset)
		   return nil, fmt.Errorf("%w: observed %s", ledger.ErrObservationMismatch, seen.Asset)
	  }
	   if err := s.Attributor().DustReject(seen.Amount.String()); err != nil {
		  return nil, err
	   }
	   // 请求未带 memo/tag 时回落链观察 FamMEMO 备注（XLM/ATOM/EOS/XRP/TON）。
	   // 观察层能给出时写入归因；漏填仍不入账。
	   // 不得用观察伪造备注；空观察不得猜用户。
	   if req.Memo == "" {
		  req.Memo = strings.TrimSpace(seen.Memo)
	  }
	  if req.Tag == "" {
		  req.Tag = strings.TrimSpace(seen.Tag)
	   }
	   if seen.OutputIndex != 0 && req.OutputIndex == 0 {
		  req.OutputIndex = seen.OutputIndex
	   }
	  if err := s.gateMemoAndNetwork(&req, seen); err != nil {
		   return nil, err
	  }
	  // 资金域统一熔断闸：只读判定，跳闸时拒绝新的观察写，不动已入账单据与在途冻结。
	  if err := s.fundCircuit(ctx, req.UserID, req.Chain, req.Asset); err != nil {
		   return nil, err
	  }
	   if req.Tier != "" {
		  if _, err := s.Delays().HoldCredit(req.Chain, req.TxHash, seen.Confirmations, ParseTier(req.Tier)); err != nil {
			  return nil, err
		   }
	  }
	   fromAddr := strings.TrimSpace(seen.FromAddress)
	   if err := s.screenInboundKYT(ctx, req.UserID, req.Chain, req.Asset, seen.Amount.String(), req.TxHash, fromAddr); err != nil {
		  return nil, err
	   }
	   if err := s.screenInboundTravel(req.UserID, req.Asset, seen.Amount.String(), req.TxHash, fromAddr); err != nil {
		   return nil, err
	   }
	  if s.desk != nil {
		   if err := s.desk.BlocksCreditScoped(req.UserID, req.Chain, req.Asset); err != nil {
			  return nil, err
		  }
	  }
	   // 粉尘防护钩：污染地址不得静默贷记；开防护单并停写。
	   // 钩子默认关闭（dustGuard==nil）；挂载后 fail-closed，错误即停写。
	  if s.dustGuard != nil {
		   toAddr := strings.TrimSpace(seen.ToAddress)
		  if err := s.dustGuard.CheckObserve(ctx, req.Chain, toAddr, req.TxHash,
			   req.UserID, seen.Amount.String(), req.Asset, fromAddr); err != nil {
			   return nil, err
		  }
	   }
	   // 归因钩子默认关闭。关闭时这里直接返回，不改备注闸、巡检和 dep: 熔断。
	  if err := s.gateAttribution(ctx, req, seen); err != nil {
		   return nil, err
	   }
	   payload := command.HashPayload("deposit", req.UserID, req.Chain, req.TxHash, req.Asset, seen.Amount.String(), fmt.Sprintf("%d", req.OutputIndex))
	  rec, existed, err := s.cmds.Begin(req.IdempotencyKey, "deposit", payload)
	   if err != nil {
		  return nil, err
	  }
	  if existed && rec.Status == command.StatusSucceeded && rec.ResultID != "" {
		   return s.led.Get(ctx, rec.ResultID)
	  }
	  s.ensureGuards()
	  if err := s.hitAbuse(req.UserID, req.Chain, req.Asset); err != nil {
		   s.cmds.Fail(req.IdempotencyKey, err)
		  return nil, err
	   }
	  if err := s.slots.Allow(req.Chain, req.TxHash, req.IdempotencyKey, req.OutputIndex); err != nil {
		  s.noteDup(req)
		  s.cmds.Fail(req.IdempotencyKey, err)
		  return nil, err
	   }
	   run := Run{ID: uuid.NewString(), Chain: req.Chain, TxHash: req.TxHash, OutputIndex: req.OutputIndex, UserID: req.UserID, Asset: req.Asset, FromAddress: fromAddr, Step: StepObserved, Amount: seen.Amount.String()}
	   if err := s.book.PutErr(run); err != nil {
		  s.cmds.Fail(req.IdempotencyKey, err)
		  return nil, err
	  }
	  tr, err := s.led.Create(ctx, ledger.CreateReq{
		   UserID:    req.UserID,
		  Direction: ledger.WalletDeposit,
		  Asset:     req.Asset, Amount: seen.Amount, Chain: req.Chain,
		  TxHash: req.TxHash, IdempotencyKey: req.IdempotencyKey,
		   WaitConfirm: req.WaitConfirm,
		  OutputIndex: req.OutputIndex,
		  FromAddress: fromAddr,
		   Memo:        req.Memo, Tag: req.Tag, TagName: seen.TagName,
	  })
	   if tr != nil {
		   run.TransferID = tr.ID
		   switch tr.State {
		  case ledger.StateCredited:
			   run.Step = StepCredited
		  case ledger.StateHold:
			  run.Step = StepHeld
		  default:
			  run.Step = StepAccepted
		  }
		  if err != nil {
			   run.Err = err.Error()
			  s.cmds.Fail(req.IdempotencyKey, err)
		  } else {
			   s.cmds.Succeed(req.IdempotencyKey,
				  tr.ID)
		   }
		   if perr := s.book.PutErr(run); perr != nil && err == nil {
			   err = perr
		  }
	   } else if err != nil {
		  s.cmds.Fail(req.IdempotencyKey,
			   err)
		  if errors.Is(err, ledger.ErrDuplicateHash) {
			  s.noteDup(req)
		  }
	  }
	  if err == nil && tr != nil {
		  s.afterCredit(ctx, req, tr, seen.Confirmations)
	   }
	  if err == nil && tr != nil && tr.State == ledger.StateCredited {
		  ingest.Try(ctx, ingest.CreditFact(tr.UserID, tr.Asset, tr.Amount.String(), tr.ID, "deposit-desk", time.Now().UTC()))
	  }
	  if err == nil && tr != nil {
		  s.rememberObserved(ctx, tr.UserID, tr.Chain, tr.Asset, tr.Amount.String(), tr.TxHash, tr.IdempotencyKey, tr.ID, fromAddr, seen.Confirmations)
	  }
	   return tr, err
   }

  func (s *Service) Runs() *Book { return s.book }

// Confirm 达确认深度后从 system.bridge 入钱包。深度不够返回 pending，Recover 再扫。
// 入账前再过 desk BlocksCreditScoped（Match 漂移单须先结案）。
// 先读单再闸，避免无主单时空闸。
// 未挂 desk 时跳过（开发装配）； Attach 必挂。

   func (s *Service) Confirm(ctx context.Context, id string) error {
	  if s == nil || s.led == nil {
		   return fmt.Errorf("deposit ledger unavailable")
	   }
	  if err := s.checkHalt(ctx); err != nil {
		  return err
	   }
	   tr, err := s.led.Get(ctx, id)
	   if err != nil {
		  return err
	   }
	   if tr != nil {
		  s.ensureGuards()
		   if err := s.hitAbuse(tr.UserID, tr.Chain, tr.Asset); err != nil {
			   return err
		  }
		   if err := s.slots.BlockedConfirm(tr.Chain, tr.TxHash, tr.OutputIndex); err != nil {
			  return err
		  }
		  if s.desk != nil {
			  if err := s.desk.BlocksCreditScoped(tr.UserID, tr.Chain, tr.Asset); err != nil {
				  return err
			   }
		   }
	  }
	   if err := s.led.ConfirmAndCredit(ctx, id); err != nil {
		  return err
	  }
	  if tr != nil && tr.State != ledger.StateReorged {
		   if face, ok := s.roll.Face(tr.Chain, tr.TxHash, tr.OutputIndex); ok && face.Confirming {
			  _, _ = s.roll.MarkFinal(tr.Chain, tr.TxHash, tr.ID, tr.OutputIndex)
		  }
	  }
	   if credited, gerr := s.led.Get(ctx, id); gerr == nil && credited != nil && credited.State == ledger.StateCredited {
		   ingest.Try(ctx, ingest.CreditFact(credited.UserID, credited.Asset, credited.Amount.String(), credited.ID, "deposit-desk", time.Now().UTC()))
		   need := DepthFor(credited.Chain)
		   s.emitCredited(ctx, credited, need)
	   }
	  s.rememberConfirmed(ctx, id)
	  return nil
  }

// Unwind 把已 credited 的入金按重组扣回。必须在调用方把链观察做完之后再进账本锁。
   func (s *Service) Unwind(ctx context.Context, id string) error {
	  if s == nil || s.led == nil {
		  return fmt.Errorf("deposit ledger unavailable")
	  }
	   var before string
	  if tr, err := s.led.Get(ctx, id); err == nil && tr != nil {
		   before = walletAvail(ctx, s.led, tr.UserID, tr.Asset)
	  }
	  if err := s.led.UnwindReorg(ctx, id); err != nil {
		   return err
	   }
	  s.explainUnwind(ctx, id, before)
	  s.rememberUnwound(ctx, id)
	   return nil
  }

   func (s *Service) Get(ctx context.Context, id string) (*ledger.Transfer, error) {
	   if s == nil || s.led == nil {
		   return nil, fmt.Errorf("deposit ledger unavailable")
	  }
	   return s.led.Get(ctx, id)
   }

// openDesk Observe 漂移落单，写入 Asset 供 BlocksCreditScoped。
// actor 不得为 system；开单失败不影响主错误返回。
// 空 user 跳过（无法归因）；不得用开单掩盖观察失败。

   func (s *Service) openDesk(userID, chain, asset, hash string, kind TicketKind, note string) {
	  if s == nil || s.desk == nil {
		  return
	  }
	   if strings.TrimSpace(userID) == "" {
		  return
	   }
	   _, _ = s.desk.OpenException("deposit-observe", userID, chain, asset, hash, string(kind), note)
   }
