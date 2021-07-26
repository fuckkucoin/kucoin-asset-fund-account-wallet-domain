package hotreserve

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

   func normAsset(asset string) (string, error) {
	  a, err := validate.Asset(asset)
	   if err != nil {
		   return "", ErrAsset
	  }
	  return a, nil
  }

   func clip(s string, n int) string {
	   s = strings.TrimSpace(s)
	  if len(s) > n {
		  return s[:n]
	   }
	  return s
   }

  func fingerprint(req CreateReq) string {
	  return strings.Join([]string{
		   req.Asset,
		  req.Amount, req.UserID, req.IntentID, req.MovementID, req.ApprovalTicketID,
	   }, "|")
   }

// Create 开立预留。热运营可用按保守下界立即下降。
// 做什么：占用一笔尚未冻结的出金意图。
// 无新鲜快照、一致差异、或下界不够时失败即关。
// 不变量：成功后状态为 open，分录只有一笔 open，冻结跟踪为 0。
   func (s *Service) Create(ctx context.Context, req CreateReq) (Reservation, error) {
	   if s == nil {
		  return Reservation{},
			  ErrUnavailable
	   }
	   as, err := normAsset(req.Asset)
	   if err != nil {
		  return Reservation{}, err
	   }
	  amt, err := parsePos(req.Amount)
	  if err != nil {
		   return Reservation{}, err
	  }
	   req.Asset = as
	   req.Amount = amt.String()
	   req.UserID = clip(req.UserID, 64)
	  req.IntentID = clip(req.IntentID,
		  128)
	  req.MovementID = clip(req.MovementID, 128)
	  req.ApprovalTicketID = clip(req.ApprovalTicketID, 128)
	   req.Note = clip(req.Note, 255)
	   if req.IdempotencyKey != "" {
		  idem, ierr := validate.Idem(req.IdempotencyKey)
		   if ierr != nil {
			   return Reservation{}, ErrIdem
		  }
		  req.IdempotencyKey = idem
	   }
	   if _, err := s.readSurface(ctx, as); err != nil {
		  s.noteGate("reserve", as, req.Amount, req.MovementID, "deny", "unread")
		  return Reservation{}, err
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if err := s.guardLocked(); err != nil {
		  return Reservation{}, err
	   }
	  fp := fingerprint(req)
	   if req.IdempotencyKey != "" {
		  if id, ok := s.byIdem[req.IdempotencyKey]; ok {
			   cur := s.reserves[id]
			   if cur == nil || cur.Fingerprint != fp {
				   return Reservation{}, ErrIdem
			   }
			   return cur.copy(), nil
		   }
	  }
	   if err := s.dupLocked(req); err != nil {
		  return Reservation{}, err
	   }
	   if err := s.admitLocked("reserve", as, amt, req.MovementID, false); err != nil {
		  return Reservation{}, err
	  }
	  now := s.now()
	   row := &Reservation{
		  ID: uuid.NewString(), Idem: req.IdempotencyKey, Fingerprint: fp,
		   Asset: as, Amount: req.Amount, State: StateOpen,
		   UserID: req.UserID, IntentID: req.IntentID, MovementID: req.MovementID,
		   ApprovalTicketID: req.ApprovalTicketID, Note: req.Note,
		   ExpiresAt: now.Add(s.pol.ReserveTTL), CreatedAt: now, UpdatedAt: now,
	  }
	   j, err := s.buildJournal(row, KindOpen, SubOpen, SubHot, req.Note, now)
	  if err != nil {
		  return Reservation{}, err
	  }
	  if err := s.tx(ctx, func(ctx context.Context) error {
		  if err := s.persistReservation(ctx, row); err != nil {
			   return err
		   }
		  return s.persistJournal(ctx, j)
	  }); err != nil {
		  return Reservation{}, err
	  }
	  s.reserves[row.ID] = row
	  s.rememberJournal(j)
	  if row.Idem != "" {
		   s.byIdem[row.Idem] = row.ID
	  }
	  s.noteGateLocked("reserve", as, req.Amount, req.MovementID, "allow", "")
	  return row.copy(), nil
  }

   func (s *Service) dupLocked(req CreateReq) error {
	   for _, r := range s.reserves {
		  if r.Asset != req.Asset {
			  continue
		  }
		   if r.State != StateOpen && r.State != StateEarmarked {
			   continue
		  }
		  if req.IntentID != "" && r.IntentID == req.IntentID {
			   return ErrDuplicate
		  }
		  if req.MovementID != "" && r.MovementID == req.MovementID {
			  return ErrDuplicate
		   }
	   }
	   return nil
  }

// Earmark 把 open 转为已圈存，并记下冻结跟踪。不调用账本 Apply。
// 不变量：圈存后预留仍计占用，冻结跟踪等于本金，不另加一笔预留。
  func (s *Service) Earmark(ctx context.Context, req EarmarkReq) (Reservation, error) {
	  if s == nil {
		  return Reservation{}, ErrUnavailable
	  }
	  req.ID = strings.TrimSpace(req.ID)
	   req.MovementID = clip(req.MovementID, 128)
	  req.FreezeRef = clip(req.FreezeRef, 128)
	   req.Note = clip(req.Note, 255)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  row := s.reserves[req.ID]
	  if row == nil {
		   return Reservation{}, ErrNotFound
	  }
	  if row.State == StateEarmarked {
		  if req.MovementID != "" && row.MovementID != "" && row.MovementID != req.MovementID {
			   return Reservation{}, ErrState
		   }
		   return row.copy(), nil
	  }
	  if row.State != StateOpen {
		  return Reservation{}, ErrState
	  }
	   now := s.now()
	  next := *row
	   next.State = StateEarmarked
	  next.EarmarkedAt = now
	   next.UpdatedAt = now
	   next.FreezeRef = req.FreezeRef
	  if req.MovementID != "" {
		  next.MovementID = req.MovementID
	   }
	  if req.Note != "" {
		   next.Note = req.Note
	  }
	   j1, err := s.buildJournal(&next, KindEarmark, SubEarmarked, SubOpen, req.Note, now)
	   if err != nil {
		   return Reservation{}, err
	   }
	   j1.Seq = len(s.journals[row.ID]) + 1
	   j2, err := s.buildJournal(&next, KindEarmarkFreeze, SubFreeze, SubMemoFrz, req.FreezeRef, now)
	  if err != nil {
		  return Reservation{}, err
	  }
	  j2.Seq = j1.Seq + 1
	  j2.EventID = eventOf(&next,
		   KindEarmarkFreeze, j2.Seq)
	   if err := s.tx(ctx, func(ctx context.Context) error {
		  if err := s.persistReservation(ctx, &next); err != nil {
			   return err
		  }
		   if err := s.persistJournal(ctx, j1); err != nil {
			   return err
		   }
		  if err := s.persistJournal(ctx, j2); err != nil {
			  return err
		  }
		  return s.persistTrack(ctx, &Track{
			   ReservationID: next.ID, Kind: TrackFreeze, Asset: next.Asset,
			  Amount: next.Amount, Ref: next.FreezeRef, UpdatedAt: now,
		   })
	  }); err != nil {
		  return Reservation{}, err
	  }
	  *row = next
	   s.rememberJournal(j1)
	  s.rememberJournal(j2)
	  s.tracks[trackKey(row.ID, TrackFreeze)] = &Track{
		  ReservationID: row.ID, Kind: TrackFreeze, Asset: row.Asset,
		  Amount: row.Amount, Ref: row.FreezeRef, UpdatedAt: now,
	   }
	  return row.copy(), nil
   }

// Consume 兑付。预留离开占用合计，冻结跟踪清掉，金额只留在广播跟踪。
// 没有广播引用不得兑付。已兑付且引用相同则原样返回。
// 不变量：兑付后 earmarked 与 freeze 不再同时持有这笔金额。

  func (s *Service) Consume(ctx context.Context, req ConsumeReq) (Reservation, error) {
	  if s == nil {
		  return Reservation{}, ErrUnavailable
	   }
	  req.ID = strings.TrimSpace(req.ID)
	   req.BroadcastRef = clip(req.BroadcastRef, 128)
	   req.Note = clip(req.Note, 255)
	   if req.BroadcastRef == "" {
		  return Reservation{}, ErrAmount
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  row := s.reserves[req.ID]
	  if row == nil {
		  return Reservation{}, ErrNotFound
	  }
	  if row.State == StateConsumed {
		  if row.BroadcastRef != req.BroadcastRef {
			   return Reservation{}, ErrIdem
		   }
		  return row.copy(), nil
	   }
	   if row.State != StateEarmarked {
		  return Reservation{}, ErrState
	  }
	   now := s.now()
	  next := *row
	   next.State = StateConsumed
	  next.ConsumedAt = now
	   next.UpdatedAt = now
	   next.BroadcastRef = req.BroadcastRef
	  if req.Note != "" {
		  next.Note = req.Note
	   }
	   j1, err := s.buildJournal(&next, KindConsume, SubBroadcast, SubEarmarked, req.Note, now)
	  if err != nil {
		   return Reservation{}, err
	   }
	   j1.Seq = len(s.journals[row.ID]) + 1
	  j2, err := s.buildJournal(&next, KindConsumeFreeze, SubMemoFrz, SubFreeze, req.BroadcastRef, now)
	  if err != nil {
		   return Reservation{}, err
	  }
	   j2.Seq = j1.Seq + 1
	  j2.EventID = eventOf(&next, KindConsumeFreeze, j2.Seq)
	  if err := s.tx(ctx, func(ctx context.Context) error {
		  if err := s.persistReservation(ctx, &next); err != nil {
			   return err
		   }
		   if err := s.persistJournal(ctx, j1); err != nil {
			  return err
		   }
		   if err := s.persistJournal(ctx, j2); err != nil {
			  return err
		  }
		  if err := s.deleteTrack(ctx, next.ID, TrackFreeze); err != nil {
			  return err
		  }
		  return s.persistTrack(ctx, &Track{
			  ReservationID: next.ID, Kind: TrackBroadcast, Asset: next.Asset,
			   Amount: next.Amount,
			  Ref:    next.BroadcastRef, UpdatedAt: now,
		  })
	  }); err != nil {
		  return Reservation{}, err
	   }
	  *row = next
	   s.rememberJournal(j1)
	   s.rememberJournal(j2)
	  delete(s.tracks, trackKey(row.ID, TrackFreeze))
	  s.tracks[trackKey(row.ID, TrackBroadcast)] = &Track{
		  ReservationID: row.ID, Kind: TrackBroadcast, Asset: row.Asset,
		   Amount: row.Amount, Ref: row.BroadcastRef, UpdatedAt: now,
	   }
	   return row.copy(), nil
   }

// Cancel 取消未广播的预留。已兑付或已有广播跟踪时拒绝。

  func (s *Service) Cancel(ctx context.Context, id, note string) (Reservation, error) {
	  return s.release(ctx, id, KindCancel, "", "", note, false)
  }

// Expire 只释放仍处于 open、且没有冻结跟踪的预留。
// 不变量：earmarked 与 consumed 不因到期改状态，也不清广播跟踪。

   func (s *Service) Expire(ctx context.Context, id string) (Reservation, error) {
	   if s == nil {
		   return Reservation{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  row := s.reserves[strings.TrimSpace(id)]
	  if row == nil {
		  return Reservation{}, ErrNotFound
	  }
	   if row.State != StateOpen {
		   return row.copy(), nil
	  }
	  if s.now().Before(row.ExpiresAt) {
		   return row.copy(), nil
	   }
	  if s.trackAmt(row.ID, TrackFreeze).IsPositive() || s.trackAmt(row.ID, TrackBroadcast).IsPositive() {
		  return Reservation{},
			  ErrBroadcastHeld
	  }
	  return s.finishReleaseLocked(ctx, row, KindExpire, "", "", "ttl", StateExpired)
   }

  func (s *Service) release(ctx context.Context, id, kind, requester, approver, note string, force bool) (Reservation, error) {
	  if s == nil {
		   return Reservation{},
			  ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   row := s.reserves[strings.TrimSpace(id)]
	   if row == nil {
		  return Reservation{}, ErrNotFound
	  }
	  if row.State == StateConsumed || s.trackAmt(row.ID, TrackBroadcast).IsPositive() {
		  return Reservation{}, ErrBroadcastHeld
	  }
	   if row.State != StateOpen && row.State != StateEarmarked {
		   return Reservation{}, ErrState
	   }
	   term := StateCancelled
	  if kind == KindExpire {
		   term = StateExpired
	   }
	  _ = force
	  return s.finishReleaseLocked(ctx, row, kind, requester, approver, note, term)
  }

  func (s *Service) finishReleaseLocked(ctx context.Context, row *Reservation, kind, requester, approver, note string, term State) (Reservation, error) {
	  now := s.now()
	  next := *row
	   from := row.State
	  next.State = term
	   next.ReleasedAt = now
	   next.UpdatedAt = now
	  next.ReleaseKind = kind
	  if requester != "" {
		   next.Requester = requester
	  }
	  if approver != "" {
		   next.Approver = approver
	   }
	  if note != "" {
		  next.Note = note
	   }
	   credit := SubOpen
	   if from == StateEarmarked {
		   credit = SubEarmarked
	   }
	  j1, err := s.buildJournal(&next, kind, SubHot, credit, note, now)
	   if err != nil {
		  return Reservation{}, err
	  }
	   j1.Seq = len(s.journals[row.ID]) + 1
	   j1.EventID = eventOf(&next, kind, j1.Seq)
	  var j2 *ReservationJournal
	   if from == StateEarmarked {
		   offKind := KindCancelFreeze
		   if kind == KindForce {
			   offKind = KindForceFreeze
		   }
		   built, berr := s.buildJournal(&next, offKind, SubMemoFrz, SubFreeze, note, now)
		  if berr != nil {
			   return Reservation{}, berr
		  }
		   built.Seq = j1.Seq + 1
		  built.EventID = eventOf(&next,
			   offKind, built.Seq)
		   j2 = &built
	  }
	  if err := s.tx(ctx, func(ctx context.Context) error {
		   if err := s.persistReservation(ctx, &next); err != nil {
			  return err
		   }
		  if err := s.persistJournal(ctx, j1); err != nil {
			  return err
		   }
		  if j2 != nil {
			   if err := s.persistJournal(ctx, *j2); err != nil {
				   return err
			   }
		   }
		   return s.deleteTrack(ctx, next.ID, TrackFreeze)
	  }); err != nil {
		  return Reservation{}, err
	   }
	  *row = next
	  s.rememberJournal(j1)
	  if j2 != nil {
		  s.rememberJournal(*j2)
	  }
	   delete(s.tracks, trackKey(row.ID, TrackFreeze))
	  return row.copy(), nil
   }

   func eventOf(r *Reservation, kind string, seq int) string {
	   return "hr.reserve." + r.ID + "." + kind + "." + itoa(seq)
   }

  func itoa(n int) string {
	   if n == 0 {
		   return "0"
	   }
	   var b [16]byte
	   i := len(b)
	  for n > 0 {
		   i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	   }
	   return string(b[i:])
  }

   func (s *Service) coveredLocked(asset, movementID string) (covered decimal.Decimal, consumed bool) {
	   if movementID == "" {
		  return decimal.Zero, false
	   }
	   for _, r := range s.reserves {
		   if r.Asset != asset {
			  continue
		   }
		   if r.MovementID != movementID && r.IntentID != movementID && r.ID != movementID {
			   continue
		   }
		  switch r.State {
		  case StateConsumed:
			   return decimal.Zero, true
		   case StateOpen, StateEarmarked:
			  if d, err := parsePos(r.Amount); err == nil {
				   covered = covered.Add(d)
			  }
		  }
	  }
	   return covered, false
  }
