package kcfund

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"github.com/shopspring/decimal"
)

const kindHopSaga = "aa_hop_saga"

const (
	hopPhaseInit  = "init"
	hopPhaseQuota = "quota_reserved"
	hopPhaseLedgerOut = "ledger_out"
	hopPhaseOverlayIn  = "overlay_in"
	hopPhaseHopped     = "hopped"
	hopPhaseOverlayOut = "overlay_out"
	hopPhaseLedgerIn   = "ledger_in"
	hopPhaseDone = "done"
	hopPhaseAborting = "aborting"
	hopPhaseAborted = "aborted"
)

var (
	ErrHopCrash      = errors.New("hop saga crash")
	ErrHopCompensate = errors.New("hop saga compensate incomplete")
)

// HopFundsReq 母子 hop 的账本+overlay 逐步提交。
type HopFundsReq struct {
	UserID string
	Kind   string
	ClientOid string
	Currency string
	Amount   decimal.Decimal
	AmountRaw   string
	FromOwner   string
	ToOwner     string
	QuotaParent string // 母户额度主体；空则跳过额度租赁
	Fault       string // 单测注入：after_ledger_out / after_overlay_in / after_hop / after_aborting / abort_ledger_out / undo_fail
}

type hopSaga struct {
	ClientOid string `json:"client_oid"`
	Kind      string `json:"kind"`
	UserID string `json:"user_id"`
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
	AmountRaw string `json:"amount_raw"`
	FromOwner string `json:"from_owner"`
	ToOwner string `json:"to_owner"`
	QuotaParent string `json:"quota_parent,omitempty"`
	QuotaLease  string `json:"quota_lease,omitempty"`
	Phase       string `json:"phase"`
	CompPhase string `json:"comp_phase,omitempty"` // aborting 时记下失败相位，禁止恢复时默认 hopped 误冲 overlay
	Attempt   int    `json:"attempt"`
	Fault string `json:"fault,omitempty"`
}

   func hopStepKey(clientOid, step string, n int) string {
	  if n <= 1 {
		   return clientOid + ":" + step
	  }
	  return fmt.Sprintf("%s:%s:%d", clientOid, step, n)
  }

// hopLiveAttempt 已冲正的跳次跳过，未冲正则续跑同一幂等键。
  func hopLiveAttempt(ctx context.Context, led *ledger.Service, clientOid, step, undoStep string) int {
	  for n := 1; n < 32; n++ {
		  tr, err := led.ByIdempotency(ctx, hopStepKey(clientOid, step, n))
		  if err != nil || tr == nil {
			   return n
		   }
		   undo, uerr := led.ByIdempotency(ctx, hopStepKey(clientOid, undoStep, n))
		   if uerr == nil && undo != nil && undo.State == ledger.StateCredited {
			   continue
		  }
		   return n
	   }
	   return 32
   }

   func compensateLedger(ctx context.Context, led *ledger.Service, uid, asset string, amt decimal.Decimal, undoKey string) error {
	  if led == nil {
		  return nil
	   }
	  _, err := led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.SubCredit, Asset: asset, Amount: amt,
		  IdempotencyKey: undoKey,
	   })
	   return err
   }

   func loadHopSaga(ctx context.Context, kv persist.KV, oid string) (hopSaga, persist.Fence, error) {
	  if kv == nil {
		   return hopSaga{}, persist.Fence{}, persist.ErrNotFound
	  }
	   row, err := kv.Get(ctx, kindHopSaga, oid)
	   if err != nil {
		  return hopSaga{}, persist.Fence{}, err
	  }
	  var s hopSaga
	  if err := json.Unmarshal(row.Body, &s); err != nil {
		  return hopSaga{}, persist.Fence{}, err
	   }
	   return s, row.Fence, nil
  }

  func saveHopSaga(ctx context.Context, kv persist.KV, s hopSaga, exp persist.Fence) (persist.Fence, error) {
	   if kv == nil {
		   return exp, nil
	  }
	   body, err := json.Marshal(s)
	  if err != nil {
		  return exp, err
	   }
	  if err := kv.Put(ctx, persist.Row{Kind: kindHopSaga, Key: s.ClientOid, Body: body}, exp); err != nil {
		   return exp, err
	   }
	   row, err := kv.Get(ctx, kindHopSaga, s.ClientOid)
	   if err != nil {
		  return exp, err
	  }
	   return row.Fence, nil
  }

  func maybeCrash(fault, at string) error {
	   if strings.TrimSpace(fault) == at {
		   return fmt.Errorf("%w: %s", ErrHopCrash, at)
	  }
	  return nil
  }

// HopFunds 母子划转 saga。账本与 overlay 不同库，不能同事务；写先行相位到 KV，
// 集群进程崩溃后 RecoverHop / 同 clientOid 重试必须能冲正或续跑，禁止永久半边。
// 整段持用户锁，避免并发 Create 各补一笔。
  func HopFunds(ctx context.Context, led *ledger.Service, svc *subacct.Service, req HopFundsReq) (subacct.HopRec, error) {
	  if led == nil || svc == nil {
		  return subacct.HopRec{}, subacct.ErrUnavailable
	   }
	   if err := svc.Hydrate(ctx); err != nil {
		   return subacct.HopRec{}, err
	   }
	  uid := strings.TrimSpace(req.UserID)
	  oid := strings.TrimSpace(req.ClientOid)
	  cur := strings.ToUpper(strings.TrimSpace(req.Currency))
	   raw := strings.TrimSpace(req.AmountRaw)
	   if raw == "" {
		  raw = req.Amount.String()
	  }
	   if uid == "" || oid == "" || cur == "" || !req.Amount.IsPositive() {
		  return subacct.HopRec{}, ledger.ErrMissingIdempotency
	  }
	  from, to := strings.TrimSpace(req.FromOwner), strings.TrimSpace(req.ToOwner)
	   if from == "" || to == "" || from == to {
		  return subacct.HopRec{}, subacct.ErrSelf
	  }
	  ctx, lease, err := led.LockUser(ctx, uid)
	  if err != nil {
		   return subacct.HopRec{}, err
	  }
	  if lease != nil {
		  defer lease.Unlock()
	   }
	   kv := svc.KV()
	   saga, fence, err := loadHopSaga(ctx, kv, oid)
	   if err != nil && !errors.Is(err, persist.ErrNotFound) {
		  return subacct.HopRec{}, err
	   }
	   if errors.Is(err, persist.ErrNotFound) || saga.ClientOid == "" {
		   saga = hopSaga{
			   ClientOid: oid,
			   Kind: req.Kind, UserID: uid, Currency: cur,
			   Amount: req.Amount.String(), AmountRaw: raw, FromOwner: from, ToOwner: to,
			   QuotaParent: strings.TrimSpace(req.QuotaParent),
			  Phase:       hopPhaseInit, Attempt: 1, Fault: req.Fault,
		   }
		   fence, err = saveHopSaga(ctx, kv, saga, persist.Fence{})
		  if err != nil {
			  return subacct.HopRec{}, err
		   }
	   } else {
		   if req.Fault != "" {
			  saga.Fault = req.Fault
		  }
		  if saga.Phase == hopPhaseDone {
			   return svc.GetHopByIdem(oid)
		  }
		  if saga.Phase == hopPhaseAborted {
			  saga.Attempt++
			  saga.Phase = hopPhaseInit
			   saga.Fault = req.Fault
			  fence,
				   err = saveHopSaga(ctx, kv, saga, fence)
			  if err != nil {
				  return subacct.HopRec{}, err
			   }
		   }
	  }
	  hop, err := runHopSaga(ctx, led, svc, saga, fence, req.Amount)
	   if err == nil && hop.ID != "" {
		   at := hop.CreatedAt
		   if at.IsZero() {
			   at = time.Now().UTC()
		  }
		   ingest.Try(ctx, ingest.HopFact(hop.FromUser, hop.ToUser, hop.Asset, hop.Amount, hop.ID, true, at))
		  ingest.Try(ctx, ingest.HopFact(hop.ToUser, hop.FromUser, hop.Asset, hop.Amount, hop.ID, false, at))
	   }
	  return hop, err
   }

// RecoverHop 崩溃后续跑：读 KV 相位，冲正未完成跳或续做剩余跳。

   func RecoverHop(ctx context.Context, led *ledger.Service, svc *subacct.Service, clientOid string) (subacct.HopRec, error) {
	   if svc == nil {
		  return subacct.HopRec{}, subacct.ErrUnavailable
	  }
	  if err := svc.Hydrate(ctx); err != nil {
		   return subacct.HopRec{}, err
	   }
	   saga, _, err := loadHopSaga(ctx, svc.KV(), strings.TrimSpace(clientOid))
	   if err != nil {
		  return subacct.HopRec{}, err
	  }
	  amt, err := ledger.ParseAmount(saga.Amount)
	  if err != nil {
		   return subacct.HopRec{}, err
	   }
	  return HopFunds(ctx, led, svc, HopFundsReq{
		  UserID: saga.UserID, Kind: saga.Kind, ClientOid: saga.ClientOid,
		  Currency: saga.Currency, Amount: amt, AmountRaw: saga.AmountRaw,
		  FromOwner: saga.FromOwner, ToOwner: saga.ToOwner,
	  })
  }

  func runHopSaga(ctx context.Context, led *ledger.Service, svc *subacct.Service, saga hopSaga, fence persist.Fence, amt decimal.Decimal) (subacct.HopRec, error) {
	   kv := svc.KV()
	  advance := func(phase string) error {
		   saga.Phase = phase
		  var err error
		  fence, err = saveHopSaga(ctx, kv, saga, fence)
		   return err
	   }
	  abort := func(failPhase string, cause error) (subacct.HopRec, error) {
		  saga.CompPhase = failPhase
		   if err := advance(hopPhaseAborting); err != nil {
			  return subacct.HopRec{}, err
		  }
		  if err := maybeCrash(saga.Fault, "after_aborting"); err != nil {
			  return subacct.HopRec{}, err
		  }
		   if cerr := compensateHop(ctx, led, svc, saga, amt, saga.CompPhase); cerr != nil {
			  return subacct.HopRec{}, fmt.Errorf("%w: %v", ErrHopCompensate, cerr)
		   }
		  if err := advance(hopPhaseAborted); err != nil {
			   return subacct.HopRec{}, err
		  }
		   if cause == nil {
			  return subacct.HopRec{}, ErrHopCompensate
		  }
		   return subacct.HopRec{}, cause
	   }
	   if saga.Phase == hopPhaseAborting {
		   phase := saga.CompPhase
		   if phase == "" {
			   phase = inferCompPhase(ctx, led, svc, saga)
		   }
		  if err := compensateHop(ctx, led, svc, saga, amt, phase); err != nil {
			  return subacct.HopRec{}, err
		  }
		  if err := advance(hopPhaseAborted); err != nil {
			   return subacct.HopRec{}, err
		   }
		   return subacct.HopRec{}, ErrHopCompensate
	  }
	  switch saga.Kind {
	  case "PARENT_TO_SUB":
		  n := saga.Attempt
		   if n <= 0 {
			  n = hopLiveAttempt(ctx, led, saga.ClientOid, "ledger-out", "ledger-out-undo")
		   }
		  if saga.Phase == hopPhaseInit {
			  if qp := strings.TrimSpace(saga.QuotaParent); qp != "" {
				   leaseID, err := svc.BeginQuotaLease(ctx, qp, saga.Currency, saga.AmountRaw)
				   if err != nil {
					   return subacct.HopRec{}, err
				   }
				   saga.QuotaLease = leaseID
				   if err := advance(hopPhaseQuota); err != nil {
					  svc.ReleaseQuotaLease(ctx, leaseID, qp, saga.Currency, saga.AmountRaw)
					  return subacct.HopRec{}, err
				   }
			   } else if err := advance(hopPhaseQuota); err != nil {
				  return subacct.HopRec{}, err
			   }
		  }
		  if saga.Phase == hopPhaseQuota {
			   if _, err := led.Create(ctx, ledger.CreateReq{
				   UserID: saga.UserID, Direction: ledger.SubDebit, Asset: saga.Currency, Amount: amt,
				   IdempotencyKey: hopStepKey(saga.ClientOid, "ledger-out", n),
			  }); err != nil {
				   return abort(hopPhaseQuota, err)
			  }
			   if err := advance(hopPhaseLedgerOut); err != nil {
				   return subacct.HopRec{}, err
			  }
			  if err := maybeCrash(saga.Fault, "after_ledger_out"); err != nil {
				   return subacct.HopRec{}, err
			  }
		  }
		   if saga.Phase == hopPhaseLedgerOut {
			   if saga.Fault == "after_aborting" || saga.Fault == "abort_ledger_out" {
				  return abort(hopPhaseLedgerOut, ErrHopCompensate)
			  }
			  if _, err := svc.Credit(subacct.PostReq{OwnerID: saga.FromOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid + ":ov-in"}); err != nil {
				  return abort(hopPhaseLedgerOut, err)
			  }
			   if err := advance(hopPhaseOverlayIn); err != nil {
				  return subacct.HopRec{}, err
			   }
			   if err := maybeCrash(saga.Fault, "after_overlay_in"); err != nil {
				   return subacct.HopRec{}, err
			   }
		  }
		   if saga.Phase == hopPhaseOverlayIn {
			  hop, err := svc.Hop(ctx, subacct.HopReq{
				  FromOwner: saga.FromOwner, ToOwner: saga.ToOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid,
			  })
			  if err != nil {
				  return abort(hopPhaseOverlayIn,
					  err)
			  }
			  if err := advance(hopPhaseHopped); err != nil {
				   return subacct.HopRec{}, err
			  }
			   if err := maybeCrash(saga.Fault, "after_hop"); err != nil {
				   return subacct.HopRec{}, err
			  }
			  if err := advance(hopPhaseDone); err != nil {
				   return subacct.HopRec{}, err
			  }
			  _ = svc.CommitQuotaLease(ctx, saga.QuotaLease)
			   return hop, nil
		   }
		  if saga.Phase == hopPhaseHopped || saga.Phase == hopPhaseDone {
			  if err := advance(hopPhaseDone); err != nil {
				  return subacct.HopRec{}, err
			   }
			   _ = svc.CommitQuotaLease(ctx, saga.QuotaLease)
			   return svc.GetHopByIdem(saga.ClientOid)
		   }
	  case "SUB_TO_PARENT":
		  if saga.Phase == hopPhaseInit {
			  if qp := strings.TrimSpace(saga.QuotaParent); qp != "" {
				   leaseID, err := svc.BeginQuotaLease(ctx, qp, saga.Currency, saga.AmountRaw)
				   if err != nil {
					  return subacct.HopRec{}, err
				   }
				   saga.QuotaLease = leaseID
				  if err := advance(hopPhaseQuota); err != nil {
					  svc.ReleaseQuotaLease(ctx, leaseID, qp, saga.Currency, saga.AmountRaw)
					   return subacct.HopRec{}, err
				   }
			  } else if err := advance(hopPhaseQuota); err != nil {
				  return subacct.HopRec{}, err
			  }
		   }
		  if saga.Phase == hopPhaseQuota {
			  if _, err := svc.Hop(ctx, subacct.HopReq{
				  FromOwner: saga.FromOwner, ToOwner: saga.ToOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid,
			  }); err != nil {
				  return abort(hopPhaseQuota,
					   err)
			  }
			  if err := advance(hopPhaseHopped); err != nil {
				  return subacct.HopRec{}, err
			   }
			  if err := maybeCrash(saga.Fault, "after_hop"); err != nil {
				  return subacct.HopRec{}, err
			  }
		  }
		   if saga.Phase == hopPhaseHopped {
			  if _, err := svc.Debit(subacct.PostReq{OwnerID: saga.ToOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid + ":ov-out"}); err != nil {
				   return abort(hopPhaseHopped, err)
			   }
			  if err := advance(hopPhaseOverlayOut); err != nil {
				  return subacct.HopRec{}, err
			   }
		   }
		  if saga.Phase == hopPhaseOverlayOut {
			  n := saga.Attempt
			  if n <= 0 {
				  n = hopLiveAttempt(ctx, led, saga.ClientOid, "ledger-in", "ledger-in-undo")
			  }
			  if _, err := led.Create(ctx, ledger.CreateReq{
				   UserID: saga.UserID, Direction: ledger.SubCredit, Asset: saga.Currency, Amount: amt,
				   IdempotencyKey: hopStepKey(saga.ClientOid, "ledger-in", n),
			   }); err != nil {
				   return abort(hopPhaseOverlayOut, err)
			   }
			  if err := advance(hopPhaseDone); err != nil {
				  return subacct.HopRec{}, err
			  }
			  _ = svc.CommitQuotaLease(ctx, saga.QuotaLease)
			   return svc.GetHopByIdem(saga.ClientOid)
		  }
		  if saga.Phase == hopPhaseDone {
			  _ = svc.CommitQuotaLease(ctx, saga.QuotaLease)
			   return svc.GetHopByIdem(saga.ClientOid)
		   }
	   default:
		   return subacct.HopRec{}, ledger.ErrUnknownDirection
	   }
	  return subacct.HopRec{}, fmt.Errorf("%w: phase %s", ErrHopCompensate, saga.Phase)
   }

  func compensateHop(ctx context.Context, led *ledger.Service, svc *subacct.Service, saga hopSaga, amt decimal.Decimal, phase string) error {
	  n := saga.Attempt
	  if n <= 0 {
		  n = 1
	  }
	  if phase == hopPhaseAborting {
		   if saga.CompPhase != "" {
			   phase = saga.CompPhase
		   } else {
			   phase = inferCompPhase(ctx, led, svc, saga)
		   }
	  }
	   var first error
	   note := func(err error) {
		   if err != nil && first == nil {
			  first = err
		  }
	  }
	  // 额度租赁：凡进入过 quota_reserved 及之后相位失败均释放
	   if saga.QuotaLease != "" && phase != hopPhaseInit {
		  svc.ReleaseQuotaLease(ctx, saga.QuotaLease, saga.QuotaParent, saga.Currency, saga.AmountRaw)
	  }
	  switch saga.Kind {
	  case "PARENT_TO_SUB":
		   if phase == hopPhaseHopped {
			   note(svc.ReverseHop(ctx, saga.ClientOid))
		  }
		   if phase == hopPhaseOverlayIn || phase == hopPhaseHopped {
			  _, e := svc.Debit(subacct.PostReq{OwnerID: saga.FromOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid + ":ov-undo"})
			   note(e)
			   svc.ForgetPost("credit", subacct.PostReq{OwnerID: saga.FromOwner, IdempotencyKey: saga.ClientOid + ":ov-in"})
		   }
		   if phase == hopPhaseLedgerOut || phase == hopPhaseOverlayIn || phase == hopPhaseHopped {
			  note(compensateLedger(ctx, led, saga.UserID, saga.Currency, amt, hopStepKey(saga.ClientOid, "ledger-out-undo", n)))
		  }
	  case "SUB_TO_PARENT":
		   if phase == hopPhaseOverlayOut {
			  _, e := svc.Credit(subacct.PostReq{OwnerID: saga.ToOwner, Asset: saga.Currency, Amount: saga.AmountRaw, IdempotencyKey: saga.ClientOid + ":ov-out-undo"})
			  note(e)
			   svc.ForgetPost("debit", subacct.PostReq{OwnerID: saga.ToOwner, IdempotencyKey: saga.ClientOid + ":ov-out"})
		   }
		   if phase == hopPhaseHopped || phase == hopPhaseOverlayOut {
			   note(svc.ReverseHop(ctx, saga.ClientOid))
		   }
	   }
	  if saga.Fault == "undo_fail" {
		   return fmt.Errorf("%w: injected", ErrHopCompensate)
	   }
	   return first
  }

// inferCompPhase 旧 aborting 记录无 CompPhase：有未冲正 hop 才按 hopped，否则只冲账本，禁止误扣 overlay。

   func inferCompPhase(ctx context.Context, led *ledger.Service, svc *subacct.Service, saga hopSaga) string {
	  if hop, err := svc.GetHopByIdem(saga.ClientOid); err == nil && hop.ID != "" && !hop.Reversed {
		  return hopPhaseHopped
	   }
	  if saga.Kind == "SUB_TO_PARENT" {
		  return hopPhaseHopped
	   }
	  n := saga.Attempt
	   if n <= 0 {
		  n = 1
	  }
	   if led != nil {
		   tr, err := led.ByIdempotency(ctx, hopStepKey(saga.ClientOid, "ledger-out", n))
		  if err == nil && tr != nil && tr.State == ledger.StateCredited {
			  return hopPhaseLedgerOut
		   }
	  }
	  return hopPhaseInit
  }

type HopRecoverReport struct {
	ClientOid string `json:"client_oid"`
	Phase     string `json:"phase"`
	Err   string `json:"error,omitempty"`
	HopID string `json:"hop_id,omitempty"`
}

// RecoverOpenHops 扫描未完成 hop saga，崩溃后由 recon-worker 扫回，禁止只靠人工点 RecoverHop。

   func RecoverOpenHops(ctx context.Context, led *ledger.Service, svc *subacct.Service, limit int) ([]HopRecoverReport, error) {
	  if svc == nil {
		   return nil, subacct.ErrUnavailable
	  }
	   if err := svc.Hydrate(ctx); err != nil {
		  return nil, err
	   }
	  kv := svc.KV()
	  if kv == nil {
		  return nil, persist.ErrNotFound
	  }
	   if limit <= 0 {
		  limit = 100
	  }
	   out := make([]HopRecoverReport, 0)
	   after := ""
	   for {
		  page, err := persist.PageKind(ctx, kv, kindHopSaga, persist.Cursor{After: after, Limit: limit})
		   if err != nil {
			   return out, err
		  }
		  for _, row := range page.Rows {
			   var saga hopSaga
			  if err := json.Unmarshal(row.Body, &saga); err != nil {
				   out = append(out, HopRecoverReport{ClientOid: row.Key, Err: err.Error()})
				  metrics.HopRecover.WithLabelValues("error").Inc()
				   continue
			  }
			   if saga.Phase == hopPhaseDone || saga.Phase == hopPhaseAborted {
				   metrics.HopRecover.WithLabelValues("skip").Inc()
				   continue
			   }
			  rep := HopRecoverReport{ClientOid: saga.ClientOid, Phase: saga.Phase}
			  hop, err := RecoverHop(ctx, led, svc, saga.ClientOid)
			  if err != nil {
				  rep.Err = err.Error()
				  metrics.HopRecover.WithLabelValues("error").Inc()
			  } else {
				   rep.HopID = hop.ID
				  metrics.HopRecover.WithLabelValues("ok").Inc()
			   }
			   out = append(out, rep)
		  }
		  if page.Done {
			   break
		   }
		   after = page.Next
		   if after == "" {
			   break
		  }
	   }
	  return out, nil
   }

// BindFundsRecon 挂 worker 周期：扫残留补账、未完成 hop saga、过期额度租赁、过期闪兑限价冻结、孤儿 holdcat。recon-worker 与资金 HTTP 进程共用。
   func BindFundsRecon(rt *app.Runtime) {
	   if rt == nil {
		  return
	  }
	   if rt.TravelFXRefresh == nil {
		   rt.TravelFXRefresh = func(ctx context.Context) {
			  book, _ := rt.TravelThresh.(*travelvasp.ThresholdBook)
			   if book == nil {
				   return
			   }
			  n, stale := book.RefreshTravelRatesTick(ctx)
			  if n > 0 || stale > 0 {
				   rt.Log.Info().Int("refreshed", n).Int("stale", stale).Msg("travel fx refresh")
			   }
		   }
	   }
	  var tickets *recon.Book
	   if rt.Durable.Recon != nil {
		  tickets = rt.Durable.Recon
	   }
	  loop := xferloop.Ensure(rt.Ledger, rt.KV, tickets, func(ctx context.Context, oid string) error {
		  if rt.Ledger == nil {
			   return ledger.ErrNotFound
		  }
		   svc := subacct.NewWithKV(rt.KV)
		   if rt.SQL != nil && !rt.Mem {
			   svc.BindSQL(rt.SQL)
		   }
		  _,
			   err := RecoverHop(ctx, rt.Ledger, svc, oid)
		   return err
	  })
	   if rt.FundsRecon != nil {
		   return
	   }
	   if rt.Withdraw == nil && rt.Ledger != nil {
		  withdraw.Attach(rt, nil)
	  }
	  rt.QuotaLeaseExpire = func(ctx context.Context) {
		  svc := subacct.NewWithKV(rt.KV)
		   if rt.SQL != nil && !rt.Mem {
			  svc.BindSQL(rt.SQL)
		   }
		  if n, err := svc.ExpireStaleQuotaLeases(ctx, 100); err != nil {
			  rt.Log.Warn().Err(err).Msg("quota lease expire")
		  } else if n > 0 {
			  rt.Log.Info().Int("expired", n).Msg("quota leases expired")
		  }
	   }
	   rt.FundsRecon = func(ctx context.Context) {
		   // 热钱包异常熔断：只读/加严，清扫不得自动解除。
		   if rt.FundSurf != nil {
			   gate := rt.FundSurf.HaltGate()
			   if f, err := gate.Snapshot(ctx); err != nil {
				   rt.Log.Warn().Err(err).Msg("withdraw halt snapshot")
			   } else if f.On {
				  rt.Log.Warn().Str("actor", f.Actor).Str("reason", f.Reason).Msg("withdraw halt active")
			   }
			   if rt.Cfg != nil && rt.Store != nil {
				   inv := hotinv.FromHotMax(rt.Cfg.Transfer.HotMax)
				  if _, err := inv.Alerts(ctx, rt.Store); err != nil {
					  if eng := gate.Engage(ctx, "bind-funds-recon", "hot inventory unreadable: "+err.Error()); eng != nil {
						   rt.Log.Warn().Err(eng).Msg("withdraw halt engage")
					   }
				  } else if err := inv.RequireAnomalyFree(ctx, rt.Store); err != nil {
					  if eng := gate.Engage(ctx, "bind-funds-recon", "hot inventory anomaly: "+err.Error()); eng != nil {
						  rt.Log.Warn().Err(eng).Msg("withdraw halt engage")
					   }
				   }
			  }
		   }
		  if rt.Ledger != nil {
			   if _, err := rt.Ledger.SweepStrandedCovers(ctx, 200); err != nil {
				  rt.Log.Warn().Err(err).Msg("cover sweep")
			   }
		  }
		  svc := subacct.NewWithKV(rt.KV)
		  if rt.SQL != nil && !rt.Mem {
			   svc.BindSQL(rt.SQL)
		  }
		   if _, err := RecoverOpenHops(ctx, rt.Ledger, svc, 200); err != nil {
			  rt.Log.Warn().Err(err).Msg("hop recover")
		   }
		   if rt.Convert != nil {
			  if _, err := rt.Convert.SweepExpiredLimits(ctx, 200); err != nil {
				  rt.Log.Warn().Err(err).Msg("convert limit sweep")
			   }
		   }
		   if _, err := SweepOrphanHoldcats(ctx, rt, 200); err != nil {
			  rt.Log.Warn().Err(err).Msg("orphan holdcat sweep")
		   }
		  if rt.Withdraw != nil {
			  if wd := withdraw.FromRuntime(rt); wd != nil {
				  if _, err := wd.SweepStuck(time.Now().UTC(), 30*time.Minute, 200); err != nil {
					   rt.Log.Warn().Err(err).Msg("withdraw stuck sweep")
				   }
			   }
		  }
		   if loop != nil {
			   if _, err := loop.Tick(ctx); err != nil {
				   rt.Log.Warn().Err(err).Msg("xfer loop")
			   }
		   }
		  if rt.Ledger != nil && rt.Store != nil {
			  hot := hotinv.New(nil)
			   if rt.Cfg != nil {
				  hot = hotinv.FromHotMax(rt.Cfg.Transfer.HotMax)
			  }
			   if _, err := recon.RunTriparty(ctx, rt.Ledger, rt.Store, tickets, "triparty", hot, nil); err != nil {
				  rt.Log.Warn().Err(err).Msg("triparty recon")
			  }
		  }
	  }
  }
