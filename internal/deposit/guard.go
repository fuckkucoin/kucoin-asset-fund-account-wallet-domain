// 错网、错备注、对不上用户时先落回收单并停写。新的幂等键才计入 dep: 速度。
// 输出槽拒绝第二笔。成功贷记后，确认数未满标确认中，已满标已贷记。
// 冲正仍走账本既有 UnwindReorg，这里只补可用余额前后和读面相位。

// Author: ZaneCruz
// Date: 2021.01.08

package deposit

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// MountGuards 保证入金写路径上的巡检与熔断已装配。
  func MountGuards(s *Service) {
	   if s != nil {
		  s.ensureGuards()
	  }
  }

// ensureGuards 保证巡检、一次贷记槽、回退簿、熔断和回收台都在。

  func (s *Service) ensureGuards() {
	  if s == nil {
		  return
	  }
	  if s.slots == nil {
		   s.slots = NewSlots()
	  }
	   if s.patrol == nil {
		  s.patrol = NewPatrol()
	  }
	   if s.roll == nil {
		  s.roll = NewRollBook()
	   }
	  if s.circuit == nil {
		  s.circuit = NewCircuit()
	   }
	   if s.recover == nil {
		  s.recover = NewRecoverDesk()
	  }
	   if s.desk == nil {
		   s.desk = NewDesk()
	   }
	   s.patrol.BindDesk(s.desk)
	   s.patrol.BindSlots(s.slots)
   }

// PatrolBook 观测簿。
   func (s *Service) PatrolBook() *Patrol {
	  if s == nil {
		   return nil
	   }
	  s.ensureGuards()
	   return s.patrol
  }

// Circuit 入金写熔断。只认 dep: 键。

  func (s *Service) Circuit() *AbuseCircuit {
	  if s == nil {
		  return nil
	  }
	   s.ensureGuards()
	  return s.circuit
  }

// Recoveries 错备注 / 错网 / 无主回收台。
  func (s *Service) Recoveries() *RecoverDesk {
	  if s == nil {
		  return nil
	  }
	   s.ensureGuards()
	   return s.recover
   }

// Rolls 提前入账与冲正读面。
   func (s *Service) Rolls() *RollBook {
	  if s == nil {
		   return nil
	   }
	   s.ensureGuards()
	   return s.roll
  }

// BindRecon 差额要追加到对账工单。nil 则只落入金工单。

   func (s *Service) BindRecon(b *recon.Book) {
	  if s == nil {
		  return
	  }
	   s.recon = b
  }

// ApplyPatrol 跑三件套。窗口内开单并锁二次贷记，同时把字段追加到对账工单。
   func (s *Service) ApplyPatrol(in PatrolIn) (PatrolOut, error) {
	   if s == nil {
		  return PatrolOut{}, ErrPatrolInput
	   }
	  s.ensureGuards()
	   out, err := s.patrol.Apply(in)
	  if out.Kind != "" {
		  if rerr := s.emitRecon(out.Event); rerr != nil && err == nil {
			   err = rerr
		   }
		  out.Event.TicketID = s.lastRecon
	   }
	  if err != nil {
		   return out, err
	  }
	  if out.Blocked {
		   if cerr := s.patrol.TryCredit(in.Chain, in.TxHash, in.SecondTransfer, in.SecondAmount, in.OutputIndex); cerr != nil {
			   out.Blocked = true
		   }
	   }
	   return out, nil
  }

// TrySecondCredit 显式二次贷记。已锁定或已贷记必须失败。
   func (s *Service) TrySecondCredit(chain, hash, transferID, amount string, output int) error {
	   if s == nil {
		  return ErrSecondCredit
	  }
	   s.ensureGuards()
	   return s.patrol.TryCredit(chain, hash, transferID, amount, output)
  }

   func (s *Service) hitAbuse(user, chain, asset string) error {
	  if s == nil || s.circuit == nil {
		   return ErrDepositSpam
	   }
	  if strings.TrimSpace(user) == "" {
		  return ErrAbuseKey
	  }
	  return s.circuit.Admit(user, chain, asset)
  }

// gateMemoAndNetwork 在进账本之前停住不能静默贷记的观察。
// 已有未批准回收单、错网、备注链漏填、备注对不上人、用户为空，都先落单再返回。
// 备注能唯一归到用户时，把用户写回请求，后面的贷记仍走原观察线程。
  func (s *Service) gateMemoAndNetwork(req *ObserveReq, seen wallet.Observation) error {
	   if s == nil || req == nil {
		   return ErrRecoveryRequired
	   }
	   s.ensureGuards()
	  if s.recover.BlocksObserve(req.Chain, req.TxHash, req.OutputIndex) != nil {
		  return ErrRecoveryRequired
	   }
	  if WrongNetwork(req.Chain, req.Asset, req.Network) {
		  s.landRecovery(*req, KindWrongNetEvent, "network "+req.Network+" chain "+req.Chain)
		  return ErrWrongNetwork
	  }
	   if req.Memo == "" && req.Tag == "" && req.UTXO == "" && wallet.RequiresFamMemo(req.Chain) {
		  s.openDesk(req.UserID, req.Chain, req.Asset, req.TxHash, TicketMemoMiss, "fam memo/tag missing")
		   s.landRecovery(*req, KindWrongMemoEvent, "fam memo/tag missing")
		  return errFamMemo(req.Chain)
	   }
	  if req.Memo != "" || req.Tag != "" || req.UTXO != "" {
		   got, err := s.Attributor().Resolve(req.Chain, req.Memo, req.Tag, req.UTXO, "")
		   if err != nil {
			   s.landRecovery(*req, KindWrongMemoEvent, err.Error())
			   return err
		  }
		  if req.UserID != "" && got.UserID != req.UserID {
			   s.landRecovery(*req, KindWrongMemoEvent, "attribution mismatch")
			   return errAttribMismatch
		  }
		  req.UserID = got.UserID
	   }
	  if strings.TrimSpace(req.UserID) == "" {
		  s.landRecovery(*req, KindUnattribEvent, "unattributed deposit")
		   return ErrUnattributed
	  }
	   return nil
  }

var errAttribMismatch = errSentinel("deposit attribution mismatch")

  func errFamMemo(chain string) error {
	   return errSentinel("deposit " + wallet.FamMemoTagName(chain) + " required for " + chain)
  }

   func (s *Service) landRecovery(req ObserveReq, kind, note string) {
	   if s == nil {
		   return
	  }
	  s.ensureGuards()
	  rec, err := s.recover.Open("deposit-observe", Recovery{
		  Kind: kind, UserID: req.UserID, Chain: req.Chain, Network: req.Network,
		  Asset: req.Asset, TxHash: req.TxHash, OutputIndex: req.OutputIndex,
		   Memo: req.Memo, Tag: req.Tag, Note: note,
	  })
	  if err != nil {
		   return
	   }
	  if strings.TrimSpace(req.UserID) != "" && req.UserID != "unassigned" {
		   deskKind := TicketWrongMemo
		   switch kind {
		  case KindWrongNetEvent:
			   deskKind = TicketWrongNet
		  case KindUnattribEvent:
			  deskKind = TicketUnattrib
		  }
		   s.openDesk(req.UserID, req.Chain, req.Asset, req.TxHash, deskKind, note)
	  }
	   ev := DepositDiffEvent{
		   Kind: kind, Chain: req.Chain, TxHash: req.TxHash, OutputIndex: req.OutputIndex,
		  Asset: req.Asset, UserID: req.UserID, DepositTicket: rec.ID,
		  Phase: PhaseRecovery, Blocked: true, Reason: note, ObserveState: "observed",
		   CreditState: "none",
	  }
	  s.emitRecon(ev)
   }

  func (s *Service) afterCredit(ctx context.Context, req ObserveReq, tr *ledger.Transfer, confirms int) {
	  if s == nil || tr == nil {
		   return
	  }
	   s.ensureGuards()
	  _ = s.slots.Commit(OutputSlot{
		   Chain: req.Chain, TxHash: req.TxHash, OutputIndex: req.OutputIndex,
		   UserID: req.UserID, Asset: req.Asset, Amount: tr.Amount.String(),
		   TransferID: tr.ID, Idem: req.IdempotencyKey, State: SlotCredited,
	   })
	  s.patrol.NoteBalanced(PatrolIn{
		   UserID: req.UserID, Chain: req.Chain, TxHash: req.TxHash, Asset: req.Asset,
		   OutputIndex: req.OutputIndex, Observed: tr.Amount.String(), Credited: tr.Amount.String(),
		   Confirmed: confirms >= DepthFor(req.Chain), Confirms: confirms, CreditState: string(tr.State),
		  TransferID: tr.ID,
	   })
	  need := DepthFor(req.Chain)
	   if tr.State == ledger.StateCredited && confirms < need {
		   before := walletAvail(ctx, s.led, tr.UserID, tr.Asset)
		  _, _ = s.roll.MarkEarly(RollbackNote{
			  Chain: req.Chain, TxHash: req.TxHash, OutputIndex: req.OutputIndex,
			  Asset: tr.Asset, UserID: tr.UserID, TransferID: tr.ID,
			   Amount: tr.Amount.String(), AvailBefore: before, Reason: "early credit",
		   })
		  s.emitCredited(ctx, tr, confirms)
		  return
	  }
	  if tr.State == ledger.StateCredited {
		  _, _ = s.roll.MarkFinal(req.Chain, req.TxHash, tr.ID, req.OutputIndex)
		   s.emitCredited(ctx, tr, confirms)
	   }
   }

   func (s *Service) noteDup(req ObserveReq) {
	   if s == nil {
		   return
	   }
	  s.ensureGuards()
	   out, err := s.patrol.Apply(PatrolIn{
		   UserID: req.UserID, Chain: req.Chain, TxHash: req.TxHash, Asset: req.Asset,
		   OutputIndex: req.OutputIndex, SecondAttempt: true, Observed: "0",
	   })
	   if err == nil || errors.Is(err, ErrOutsideWindow) || out.Kind != "" {
		   s.emitRecon(out.Event)
	  }
   }

  func (s *Service) explainUnwind(ctx context.Context, id, beforeSnap string) {
	   if s == nil || s.led == nil {
		  return
	   }
	   s.ensureGuards()
	  tr, err := s.led.Get(ctx, id)
	   if err != nil || tr == nil {
		  return
	   }
	   before := ""
	  if face, ok := s.roll.Face(tr.Chain, tr.TxHash, tr.OutputIndex); ok {
		   before = face.AvailBefore
	  }
	  if before == "" {
		  before = beforeSnap
	  }
	   if before == "" {
		  before = walletAvail(ctx, s.led, tr.UserID, tr.Asset)
	  }
	   after := walletAvail(ctx, s.led, tr.UserID, tr.Asset)
	  s.slots.MarkReversed(tr.Chain, tr.TxHash, "reorg", tr.OutputIndex)
	   face, ferr := s.roll.Rollback(RollbackNote{
		  Chain: tr.Chain, TxHash: tr.TxHash, OutputIndex: tr.OutputIndex,
		   Asset: tr.Asset, UserID: tr.UserID, TransferID: tr.ID,
		  Amount: tr.Amount.String(), AvailBefore: before, AvailAfter: after,
		  ReverseEntry: "reorg-unwind:" + tr.ID, Reason: "确认回退",
	   })
	  if ferr != nil && !errors.Is(ferr, ErrSecondCredit) {
		   return
	   }
	  if strings.TrimSpace(tr.UserID) != "" {
		  s.openDesk(tr.UserID, tr.Chain, tr.Asset, tr.TxHash, TicketEarlyBack, face.Explain)
	   }
	   s.emitRecon(DepositDiffEvent{
		   Kind: KindEarlyRollback, Chain: tr.Chain, TxHash: tr.TxHash, OutputIndex: tr.OutputIndex,
		   Asset: tr.Asset, UserID: tr.UserID, ObservedAmount: tr.Amount.String(),
		   CreditedAmount: tr.Amount.String(), ObserveState: "reorged", CreditState: "reversed",
		   SecondCredit: true, Blocked: true, Phase: PhaseReversed,
		   ReverseEntry: face.ReverseEntry, AvailBefore: face.AvailBefore, AvailAfter: face.AvailAfter,
		   Reason: face.Explain,
	  })
  }

  func walletAvail(ctx context.Context, led *ledger.Service, user, asset string) string {
	  if led == nil || strings.TrimSpace(user) == "" {
		  return "0"
	   }
	   books, err := led.Accounts(ctx, user)
	   if err != nil {
		  return "0"
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   for _, b := range books {
		  if !strings.EqualFold(b.Asset, asset) {
			  continue
		  }
		  if b.Subject == "wallet" || strings.EqualFold(b.Book, "wallet") {
			  if b.Available == "" {
				   return "0"
			   }
			  return b.Available
		  }
	   }
	   return "0"
  }

// NoteEarly 登记提前入账。必须对上已贷记的账本单据，不得凭请求体锁输出。

  func (s *Service) NoteEarly(ctx context.Context, note RollbackNote) (Face, error) {
	   if s == nil || s.led == nil {
		   return Face{}, ErrFaceForge
	  }
	   s.ensureGuards()
	   tr, err := s.proofTransfer(ctx, note)
	  if err != nil {
		  return Face{}, err
	   }
	  if tr.State != ledger.StateCredited {
		   return Face{}, ErrFaceForge
	  }
	   note.Asset = tr.Asset
	   note.UserID = tr.UserID
	  note.Amount = tr.Amount.String()
	  note.TransferID = tr.ID
	   if err := s.slots.Allow(note.Chain, note.TxHash, tr.IdempotencyKey, note.OutputIndex); err != nil {
		  return Face{}, err
	   }
	   face, err := s.roll.MarkEarly(note)
	   if err != nil {
		   return face, err
	  }
	   if err := s.slots.Commit(OutputSlot{
		   Chain: note.Chain, TxHash: note.TxHash, OutputIndex: note.OutputIndex,
		  UserID: note.UserID, Asset: note.Asset, Amount: note.Amount,
		   TransferID: note.TransferID, Idem: tr.IdempotencyKey, State: SlotCredited,
	  }); err != nil {
		  return face, err
	   }
	  return face, nil
  }

// ApplyRollback 只在账本已经冲正后补读面。未冲正不得标成已冲正，也不接受请求体里的余额。

  func (s *Service) ApplyRollback(ctx context.Context, note RollbackNote) (Face, error) {
	   if s == nil || s.led == nil {
		   return Face{}, ErrFaceForge
	   }
	  s.ensureGuards()
	  tr, err := s.proofTransfer(ctx, note)
	  if err != nil {
		   return Face{}, err
	  }
	   if tr.State != ledger.StateReorged {
		  return Face{}, ErrFaceForge
	   }
	  before := ""
	  if cur, ok := s.roll.Face(tr.Chain, tr.TxHash, tr.OutputIndex); ok {
		   before = cur.AvailBefore
	  }
	  if before == "" {
		  return Face{}, ErrFaceForge
	  }
	   after, err := walletAvailErr(ctx, s.led, tr.UserID, tr.Asset)
	   if err != nil {
		  return Face{}, err
	   }
	  note.Chain = tr.Chain
	  note.TxHash = tr.TxHash
	   note.OutputIndex = tr.OutputIndex
	  note.Asset = tr.Asset
	  note.UserID = tr.UserID
	  note.Amount = tr.Amount.String()
	   note.TransferID = tr.ID
	  note.AvailBefore = before
	  note.AvailAfter = after
	  note.ReverseEntry = "reorg-unwind:" + tr.ID
	  if note.Reason == "" {
		  note.Reason = "确认回退"
	  }
	  face, err := s.roll.Rollback(note)
	  if err != nil {
		  return face, err
	   }
	  s.slots.MarkReversed(tr.Chain, tr.TxHash, note.Reason, tr.OutputIndex)
	  if strings.TrimSpace(tr.UserID) != "" {
		  s.openDesk(tr.UserID, tr.Chain, tr.Asset, tr.TxHash, TicketEarlyBack, face.Explain)
	   }
	  if rerr := s.emitRecon(DepositDiffEvent{
		   Kind: KindEarlyRollback, Chain: tr.Chain, TxHash: tr.TxHash, OutputIndex: tr.OutputIndex,
		   Asset: tr.Asset, UserID: tr.UserID, ObservedAmount: tr.Amount.String(), CreditedAmount: tr.Amount.String(),
		  ObserveState: "reorged", CreditState: "reversed", SecondCredit: true, Blocked: true,
		   Phase: PhaseReversed, ReverseEntry: face.ReverseEntry,
		   AvailBefore: face.AvailBefore, AvailAfter: face.AvailAfter, Reason: face.Explain,
	  }); rerr != nil {
		   return face, rerr
	   }
	  return face, nil
   }

   func (s *Service) proofTransfer(ctx context.Context, note RollbackNote) (*ledger.Transfer, error) {
	  if s == nil || s.led == nil || strings.TrimSpace(note.TransferID) == "" {
		   return nil, ErrFaceForge
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  tr,
		  err := s.led.Get(ctx, strings.TrimSpace(note.TransferID))
	  if err != nil || tr == nil {
		   return nil, ErrFaceForge
	  }
	   if CanonicalChain(tr.Chain) != CanonicalChain(note.Chain) || strings.TrimSpace(tr.TxHash) != strings.TrimSpace(note.TxHash) || tr.OutputIndex != note.OutputIndex {
		   return nil, ErrPatrolInput
	  }
	   return tr, nil
   }

   func walletAvailErr(ctx context.Context, led *ledger.Service, user, asset string) (string, error) {
	   if led == nil || strings.TrimSpace(user) == "" {
		  return "", ErrFaceForge
	  }
	   books, err := led.Accounts(ctx, user)
	  if err != nil {
		  return "", err
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  for _, b := range books {
		   if !strings.EqualFold(b.Asset, asset) {
			  continue
		  }
		  if b.Subject == "wallet" || strings.EqualFold(b.Book, "wallet") {
			   if b.Available == "" {
				  return "0", nil
			  }
			  return b.Available, nil
		  }
	  }
	   return "0", nil
   }

// ReadFace 只读相位。确认中与已冲正分开返回。
   func (s *Service) ReadFace(chain, hash string, output int) (Face, error) {
	   if s == nil {
		   return Face{}, ErrFaceMiss
	  }
	  s.ensureGuards()
	   face, ok := s.roll.Face(chain, hash, output)
	   if !ok {
		  return Face{}, ErrFaceMiss
	  }
	  return face, nil
  }
