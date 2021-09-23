package partial

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/freeze"
	"account-assets.kucoin.net/funding/internal/margin"
)

var (
	ErrUnavailable    = fmt.Errorf("partial release unavailable")
	ErrEpisode        = fmt.Errorf("liquidation episode required")
	ErrFill           = fmt.Errorf("fill rejected")
	ErrAssetMismatch  = fmt.Errorf("fill asset must match lock asset")
	ErrRollbackLocked = fmt.Errorf("production config forbids rollback path")
	ErrMismatch = fmt.Errorf("freeze and ledger release mismatch")
	ErrClosed = fmt.Errorf("partial release fail-closed")
)

// Service 部分成交释冻服务。

type Service struct {
	cfg   Config
	store Store
	freeze freeze.AmountAPI
	ledger ReleasePoster
}

  func New(cfg Config, store Store, fr freeze.AmountAPI, led ReleasePoster) (*Service, error) {
	  if err := cfg.Validate(); err != nil {
		  return nil, err
	   }
	   if store == nil {
		  return nil, ErrUnavailable
	  }
	  return &Service{cfg: cfg,
		  store: store, freeze: fr, ledger: led}, nil
   }

// Open 在维持违约锁仓完成后打开释冻会话。
// 做什么：记下初始冻结额。：已存在且金额相同则幂等成功。不变量：remaining=initial，released=0。
  func (s *Service) Open(ctx context.Context, accountID, asset, initial string) (Episode, error) {
	  if s == nil {
		   return Episode{}, ErrUnavailable
	  }
	   accountID = normalizeID(accountID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if accountID == "" || asset == "" {
		   return Episode{}, ErrEpisode
	   }
	  if _, err := margin.MustPositiveAmount(initial, "initial"); err != nil {
		  return Episode{}, err
	  }
	   if err := margin.AssertConserve(initial, initial, "0"); err != nil {
		   return Episode{}, err
	   }
	  exist, ok, err := s.store.GetEpisode(ctx, accountID)
	   if err != nil {
		  return Episode{}, err
	   }
	  if ok {
		   if exist.Asset != asset || exist.Initial != initial {
			   return exist, fmt.Errorf("%w: episode already opened", ErrClosed)
		   }
		   return exist, nil
	   }
	  if s.freeze != nil {
		  if err := s.freeze.Hold(accountID, asset, initial, freeze.ReasonMaintenanceBreach); err != nil {
			   return Episode{}, err
		  }
	  }
	   ep := Episode{
		   AccountID: accountID,
		   Asset:     asset,
		  Initial: initial,
		  Remaining: initial,
		  Released: "0",
		  OpenedAtUnix: nowUnix(),
	   }
	  if err := s.store.PutEpisode(ctx, ep); err != nil {
		  return Episode{}, err
	   }
	   return ep, nil
  }

// ApplyFill 处理一笔部分成交。
// 做什么：按成交额释冻并写账本，中断后可重入补腿。
// 未知会话、资产不一致、金额非法失败封闭。
// 不变量：remaining+released=initial；同 fill 不二次计数。
   func (s *Service) ApplyFill(ctx context.Context, fill Fill) (ApplyResult, error) {
	  if s == nil {
		  return ApplyResult{}, ErrUnavailable
	  }
	   fill.ID = normalizeID(fill.ID)
	   fill.AccountID = normalizeID(fill.AccountID)
	   fill.Asset = strings.ToUpper(strings.TrimSpace(fill.Asset))
	   if fill.ID == "" || fill.AccountID == "" {
		   return ApplyResult{}, ErrFill
	   }
	  if _, err := margin.MustPositiveAmount(fill.Amount, "fill"); err != nil {
		  return ApplyResult{}, err
	   }
	   if prev, ok, err := s.store.GetTicket(ctx, fill.ID); err != nil {
		  return ApplyResult{}, err
	   } else if ok && prev.Status == StatusComplete {
		  ep, _, _ := s.store.GetEpisode(ctx, fill.AccountID)
		  return ApplyResult{Ticket: prev, Episode: ep, Idempotent: true}, nil
	   } else if ok && prev.Status != StatusRejected {
		   return s.resumeTicket(ctx, prev)
	   }
	  ep, ok, err := s.store.GetEpisode(ctx, fill.AccountID)
	  if err != nil {
		  return ApplyResult{}, err
	   }
	   if !ok {
		   return ApplyResult{}, ErrEpisode
	   }
	   if fill.Asset != ep.Asset {
		   return ApplyResult{}, ErrAssetMismatch
	   }
	  rel, err := margin.ReleaseForFill(fill.Amount, ep.Remaining)
	  if err != nil {
		  return ApplyResult{}, err
	  }
	   t := Ticket{
		  FillID: fill.ID,
		   AccountID: fill.AccountID,
		   Asset: fill.Asset,
		   FillAmount:  fill.Amount,
		   ReleaseAmt:  rel,
		  Status:      StatusPending,
		   UpdatedUnix: nowUnix(),
	   }
	   if err := s.store.PutTicket(ctx, t); err != nil {
		   return ApplyResult{}, err
	   }
	   return s.advance(ctx, ep, t)
  }

// Reenter 扫描未完成票据并补腿。重启后调用，禁止超释或漏释。
  func (s *Service) Reenter(ctx context.Context, accountID string) ([]ApplyResult, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  pending, err := s.store.ListPending(ctx, normalizeID(accountID))
	  if err != nil {
		  return nil, err
	  }
	  var out []ApplyResult
	  for _, t := range pending {
		  res, err := s.resumeTicket(ctx, t)
		   if err != nil {
			   return out, err
		  }
		  out = append(out, res)
	   }
	   return out, nil
   }

   func (s *Service) resumeTicket(ctx context.Context, t Ticket) (ApplyResult, error) {
	  ep, ok, err := s.store.GetEpisode(ctx, t.AccountID)
	   if err != nil {
		  return ApplyResult{}, err
	  }
	  if !ok {
		   return ApplyResult{}, ErrEpisode
	   }
	  return s.advance(ctx, ep, t)
   }

  func (s *Service) advance(ctx context.Context, ep Episode, t Ticket) (ApplyResult, error) {
	  idem := ReleaseIdemKey(t.FillID)
	  if !t.JournalDone {
		  if s.ledger != nil {
			   if err := s.ledger.PostRelease(ctx, t.AccountID, t.Asset, t.ReleaseAmt, idem); err != nil {
				  return ApplyResult{Ticket: t, Episode: ep}, err
			   }
		  }
		   t.JournalDone = true
		   t.Status = StatusJournalPosted
		   t.UpdatedUnix = nowUnix()
		   if err := s.store.PutTicket(ctx, t); err != nil {
			  return ApplyResult{}, err
		  }
	   }
	   if !t.FreezeDone {
		  if s.freeze != nil {
			   if err := s.freeze.Release(t.AccountID, t.Asset, t.ReleaseAmt, idem); err != nil {
				   return ApplyResult{Ticket: t, Episode: ep}, err
			   }
		   }
		   t.FreezeDone = true
		   t.Status = StatusFreezeReleased
		   t.UpdatedUnix = nowUnix()
		  if err := s.store.PutTicket(ctx, t); err != nil {
			   return ApplyResult{}, err
		   }
	  }
	   if err := s.lockLegs(ctx, ep, t); err != nil {
		  t.Status = StatusRejected
		   t.Reason = err.Error()
		  _ = s.store.PutTicket(ctx, t)
		  return ApplyResult{Ticket: t, Episode: ep}, err
	  }
	   t.Status = StatusComplete
	  t.UpdatedUnix = nowUnix()
	   if err := s.store.PutTicket(ctx, t); err != nil {
		  return ApplyResult{}, err
	   }
	   ep,
		  err := s.recomputeEpisode(ctx, ep)
	  if err != nil {
		   t.Status = StatusRejected
		   t.Reason = err.Error()
		   _ = s.store.PutTicket(ctx, t)
		   return ApplyResult{Ticket: t, Episode: ep}, err
	  }
	  return ApplyResult{Ticket: t, Episode: ep}, nil
  }

  func (s *Service) recomputeEpisode(ctx context.Context, ep Episode) (Episode, error) {
	   tickets,
		   err := s.store.ListTickets(ctx, ep.AccountID)
	  if err != nil {
		  return ep, err
	  }
	  rel := "0"
	  for _, tk := range tickets {
		  if !tk.JournalDone || !tk.FreezeDone {
			  continue
		  }
		  rel, err = addAmt(rel, tk.ReleaseAmt)
		  if err != nil {
			  return ep, err
		   }
	   }
	   left, err := subAmt(ep.Initial, rel)
	   if err != nil {
		  return ep, err
	   }
	   if err := mustConserve(ep.Initial, left, rel); err != nil {
		   return ep, fmt.Errorf("%w: %v", ErrClosed, err)
	  }
	   ep.Remaining = left
	   ep.Released = rel
	   if err := s.store.PutEpisode(ctx, ep); err != nil {
		   return ep, err
	   }
	   return ep, nil
  }

   func (s *Service) lockLegs(ctx context.Context, ep Episode, t Ticket) error {
	   if s.freeze == nil || s.ledger == nil {
		   return nil
	  }
	   // 已完成双腿的释出（含本笔）应一致冻结占用。
	  rel := t.ReleaseAmt
	  tickets, err := s.store.ListTickets(ctx, t.AccountID)
	   if err != nil {
		   return err
	   }
	   sum := "0"
	  for _, tk := range tickets {
		   if tk.FillID == t.FillID {
			  continue
		  }
		   if tk.JournalDone && tk.FreezeDone {
			   sum, err = addAmt(sum, tk.ReleaseAmt)
			  if err != nil {
				   return err
			   }
		  }
	  }
	  sum, err = addAmt(sum, rel)
	  if err != nil {
		   return err
	   }
	   wantFrozen, err := subAmt(ep.Initial, sum)
	  if err != nil {
		   return err
	  }
	  fr, err := parseDecAmt(s.freeze.FrozenAmount(t.AccountID, t.Asset), "frozen")
	  if err != nil {
		  return err
	   }
	  want, err := parseDecAmt(wantFrozen, "want")
	   if err != nil {
		   return err
	   }
	  if !fr.Equal(want) {
		  return fmt.Errorf("%w: frozen %s want %s", ErrMismatch, fr, want)
	   }
	  if !t.JournalDone || !t.FreezeDone {
		   return fmt.Errorf("%w: legs incomplete", ErrMismatch)
	  }
	   return nil
  }

  func applyConserve(ep *Episode, release string) error {
	   left, err := subAmt(ep.Remaining, release)
	  if err != nil {
		  return err
	  }
	   rel, err := addAmt(ep.Released, release)
	  if err != nil {
		  return err
	  }
	  if err := mustConserve(ep.Initial, left, rel); err != nil {
		  return fmt.Errorf("%w: %v", ErrClosed, err)
	  }
	  ep.Remaining = left
	  ep.Released = rel
	  return nil
   }

// AttemptRollback 回退已释冻结。配置锁死，不得走此路径。
  func (s *Service) AttemptRollback(ctx context.Context, accountID, amount string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   if err := s.cfg.Validate(); err != nil {
		  return err
	   }
	  if s.cfg.RollbackForbidden() {
		  return ErrRollbackLocked
	  }
	   return fmt.Errorf("%w: rollback path not implemented outside locked config", ErrClosed)
   }

// EpisodeOf 读取会话。

   func (s *Service) EpisodeOf(ctx context.Context, accountID string) (Episode, bool, error) {
	   if s == nil || s.store == nil {
		  return Episode{}, false, ErrUnavailable
	   }
	  return s.store.GetEpisode(ctx, normalizeID(accountID))
   }
