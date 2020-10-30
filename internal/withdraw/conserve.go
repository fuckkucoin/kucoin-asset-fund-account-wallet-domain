package withdraw

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

const (
	AlertFreezeBroadcastDrift = "WDConserveFreezeBroadcastDrift"
	AlertHotIntentDrift = "WDConserveHotIntentDrift"
	AlertUserAssetBlocked = "WDConserveUserAssetBlocked"
	AlertLiveHashDenied = "WDLiveHashAutoUnfreezeDenied"
	AlertHalfFrozen     = "WDIdemHalfFrozen"
	AlertAbuseTrip      = "WDAbuseCircuitTripped"
	conserveActor = "wd-conserve-desk"
)

// DriftKind 三类腿对不齐时的原因。
type DriftKind string

const (
	DriftFreezeBroadcast DriftKind = "freeze_broadcast"
	DriftLiveUnfrozen    DriftKind = "live_hash_unfrozen"
	DriftHotIntent DriftKind = "hot_intent"
	DriftHotOrphan DriftKind = "hot_intent_orphan"
)

// HotIntent 热钱包待出金意图。只作核对，不代表已广播。

type HotIntent struct {
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	Chain string `json:"chain"`
	At time.Time `json:"at"`
}

// Drift 一条守恒偏差。
type Drift struct {
	Kind       DriftKind `json:"kind"`
	Alert      string    `json:"alert"`
	UserID     string    `json:"user_id,omitempty"`
	Asset      string    `json:"asset,omitempty"`
	TransferID string    `json:"transfer_id,omitempty"`
	Note       string    `json:"note"`
}

// Block 同用户同资产新出金阻断。不作用于入金贷记。

type Block struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Reason string `json:"reason"`
	Alert string `json:"alert"`
	At time.Time `json:"at"`
}

// ConserveReport 一次扫描的告警与阻断。
type ConserveReport struct {
	Drifts []Drift `json:"drifts"`
	Blocks []Block `json:"blocks"`
	Alerts     []string `json:"alerts"`
	PersistErr string   `json:"persist_err,omitempty"`
}

// Conserve 阻断表、热意图与已开工单。

type Conserve struct {
	mu     sync.Mutex
	blocks map[string]Block
	intents map[string]HotIntent
	ticketed map[string]string
	clock    func() time.Time
	kv persist.KV
	storeErr error
}

  func NewConserve() *Conserve {
	  return &Conserve{
		  blocks: map[string]Block{},
		   intents: map[string]HotIntent{},
		  ticketed: map[string]string{},
		   clock:    func() time.Time { return time.Now().UTC() },
	  }
   }

  func pairKey(user, asset string) string {
	   return strings.TrimSpace(user) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

  func (c *Conserve) now() time.Time {
	  if c == nil || c.clock == nil {
		  return time.Now().UTC()
	   }
	  return c.clock()
  }

// NoteIntent 登记热出金意图。金额以冻结额为准。
   func (c *Conserve) NoteIntent(in HotIntent) {
	   if c == nil || strings.TrimSpace(in.TransferID) == "" {
		  return
	   }
	  in.Asset = strings.ToUpper(strings.TrimSpace(in.Asset))
	   in.UserID = strings.TrimSpace(in.UserID)
	  if in.At.IsZero() {
		   in.At = c.now()
	   }
	   c.mu.Lock()
	   c.intents[in.TransferID] = in
	   c.mu.Unlock()
	   if err := c.saveIntent(in); err != nil {
		   c.setStoreErr(err)
	  }
  }

// DropIntent 取消且未上链后摘掉意图，避免孤儿漂移。
   func (c *Conserve) DropIntent(transferID string) {
	   if c == nil {
		   return
	  }
	   id := strings.TrimSpace(transferID)
	   c.mu.Lock()
	   delete(c.intents, id)
	  c.mu.Unlock()
	  if err := c.deleteIntent(id); err != nil {
		   c.setStoreErr(err)
	  }
  }

   func (c *Conserve) intentCopy() map[string]HotIntent {
	  if c == nil {
		  return map[string]HotIntent{}
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   out := make(map[string]HotIntent, len(c.intents))
	  for k, v := range c.intents {
		   out[k] = v
	   }
	   return out
   }

   func (c *Conserve) Blocked(user, asset string) (Block, bool) {
	  if c == nil {
		  return Block{}, false
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	  b, ok := c.blocks[pairKey(user, asset)]
	  return b, ok
   }

   func (c *Conserve) Blocks() []Block {
	   if c == nil {
		  return nil
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  out := make([]Block, 0, len(c.blocks))
	   for _, b := range c.blocks {
		   out = append(out, b)
	  }
	  return out
   }

// ApplyBlocks 用本次漂移重写阻断表。已消失的用户资产解除阻断。
   func (c *Conserve) ApplyBlocks(drifts []Drift) []Block {
	  return c.applyBlocks(drifts,
		   nil)
  }

// applyBlocks 写入漂移对应的阻断。retain 为真时留下本次漂移没有覆盖、但又不能证明已结清的旧阻断。
  func (c *Conserve) applyBlocks(drifts []Drift, retain func(Block) bool) []Block {
	   if c == nil {
		  return nil
	  }
	   next := map[string]Block{}
	   now := c.now()
	   for _, d := range drifts {
		  if d.UserID == "" || d.Asset == "" {
			   continue
		   }
		   k := pairKey(d.UserID, d.Asset)
		   if _, ok := next[k]; ok {
			  continue
		  }
		   next[k] = Block{
			   UserID: d.UserID, Asset: strings.ToUpper(d.Asset),
			   Reason: d.Note, Alert: AlertUserAssetBlocked, At: now,
		  }
	  }
	   if retain != nil {
		   for _, b := range c.Blocks() {
			  k := pairKey(b.UserID, b.Asset)
			  if _, ok := next[k]; ok {
				  continue
			   }
			   if retain(b) {
				  next[k] = b
			   }
		  }
	   }
	   c.mu.Lock()
	  c.blocks = next
	  c.mu.Unlock()
	   if err := c.syncBlocks(next); err != nil {
		   c.setStoreErr(err)
	   } else if c.boundKV() != nil {
		  c.setStoreErr(nil)
	  }
	   out := make([]Block, 0, len(next))
	   for _, b := range next {
		  out = append(out, b)
	  }
	   return out
  }

  func (c *Conserve) seenTicket(key string) (string, bool) {
	  if c == nil {
		   return "", false
	   }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   id, ok := c.ticketed[guardKey(key)]
	   if !ok {
		   id, ok = c.ticketed[key]
	  }
	  return id, ok
  }

   func (c *Conserve) markTicket(key, id string) {
	   if c == nil || key == "" {
		  return
	  }
	   c.mu.Lock()
	   c.ticketed[guardKey(key)] = id
	  c.mu.Unlock()
	   if err := c.saveTicketMark(key, id); err != nil {
		  c.setStoreErr(err)
	  }
   }

   func sameAmt(a, b string) bool {
	  a = strings.TrimSpace(a)
	  b = strings.TrimSpace(b)
	   if a == "" || b == "" {
		  return a == b
	   }
	   da, ea := decimal.NewFromString(a)
	  db, eb := decimal.NewFromString(b)
	   if ea != nil || eb != nil {
		   return a == b
	   }
	   return da.Equal(db)
   }

   func freezeOpen(st FreezeState) bool {
	  switch st {
	  case FreezeHeld, FreezeScreening, FreezeApproved, FreezeSigning, FreezeBroadcast, FreezeConfirming:
		   return true
	  default:
		   return false
	  }
  }

  func freezeExpectsBroadcast(st FreezeState) bool {
	  return st == FreezeBroadcast || st == FreezeConfirming
  }

   func freezeMatchesLive(st FreezeState) bool {
	  switch st {
	   case FreezeBroadcast, FreezeConfirming, FreezeSucceeded:
		   return true
	   default:
		  return false
	  }
  }

  func broadcastSeen(row Broadcast, ok bool) bool {
	  if !ok {
		  return false
	   }
	   if LiveHash(row.TxHash) {
		  return true
	   }
	  switch row.Status {
	  case BcBroadcast,
		   BcConfirming, BcReorged, BcConfirmed:
		   return true
	  default:
		   return false
	   }
   }

   func runFreeze(r Run) FreezeState {
	  if r.Freeze != FreezeNone {
		   return r.Freeze
	  }
	   return freezeFromStep(r.Step)
  }

// collectDrifts 只读三腿。不写阻断、不开单。
   func (s *Service) collectDrifts() []Drift {
	   if s == nil || s.book == nil {
		  return nil
	  }
	   return s.collectFrom(s.book.List())
  }

// collectFrom 用给定冻结位广播与热意图。调用方可以并入账本投影，不写冻结簿。

   func (s *Service) collectFrom(runs []Run) []Drift {
	   if s == nil {
		  return nil
	   }
	  bcs := s.Broadcasts().All()
	   intents := s.conserve.intentCopy()
	   byTr := map[string]Run{}
	  for _, r := range runs {
		  if r.TransferID == "" {
			  continue
		   }
		   byTr[r.TransferID] = r
	   }
	   byBC := map[string]Broadcast{}
	  for _, b := range bcs {
		  byBC[b.TransferID] = b
	  }
	   var out []Drift
	  seen := map[string]struct{}{}
	   add := func(d Drift) {
		  k := string(d.Kind) + "|" + d.TransferID + "|" + d.UserID + "|" + d.Asset
		   if _, ok := seen[k]; ok {
			   return
		  }
		  seen[k] = struct{}{}
		   out = append(out, d)
	   }
	   for id, r := range byTr {
		   st := runFreeze(r)
		  bc, ok := byBC[id]
		   in, hasIn := intents[id]
		   asset := r.Asset
		   if asset == "" {
			   asset = bc.Asset
		   }
		   if asset == "" && hasIn {
			  asset = in.Asset
		  }
		   user := r.UserID
		  if user == "" && hasIn {
			  user = in.UserID
		  }
		  if freezeExpectsBroadcast(st) && !broadcastSeen(bc, ok) {
			   add(Drift{
				   Kind:   DriftFreezeBroadcast,
				  Alert:  AlertFreezeBroadcastDrift,
				   UserID: user, Asset: asset, TransferID: id,
				   Note: "冻结已进入广播相位但广播跟踪无 live hash",
			   })
		   }
		  if ok && LiveHash(bc.TxHash) && !freezeMatchesLive(st) {
			  add(Drift{
				   Kind: DriftLiveUnfrozen, Alert: AlertFreezeBroadcastDrift,
				   UserID: user, Asset: asset, TransferID: id,
				  Note: "live hash 仍在但冻结位不在广播或确认",
			   })
		   }
		  if ok && r.Amount != "" && bc.Amount != "" && !sameAmt(r.Amount, bc.Amount) {
			   add(Drift{
				   Kind: DriftFreezeBroadcast, Alert: AlertFreezeBroadcastDrift,
				   UserID: user, Asset: asset, TransferID: id,
				   Note: "冻结额与广播额不一致",
			  })
		  }
		   if freezeOpen(st) && r.Amount != "" {
			   if !hasIn {
				  add(Drift{
					   Kind: DriftHotIntent, Alert: AlertHotIntentDrift,
					   UserID: user,
					   Asset:  asset, TransferID: id,
					  Note: "在途冻结缺少热出金意图",
				   })
			  } else if !sameAmt(r.Amount, in.Amount) || (in.Asset != "" && asset != "" && !strings.EqualFold(in.Asset, asset)) {
				  add(Drift{
					   Kind: DriftHotIntent, Alert: AlertHotIntentDrift,
					  UserID: user, Asset: asset, TransferID: id,
					  Note: "热出金意图与冻结额或资产不一致",
				  })
			   }
		   }
	   }
	  for id, bc := range byBC {
		   if !LiveHash(bc.TxHash) {
			  continue
		   }
		   if _, ok := byTr[id]; ok {
			  continue
		  }
		  user, asset := "", bc.Asset
		   if in, ok := intents[id]; ok {
			  user, asset = in.UserID, in.Asset
		   }
		  add(Drift{
			   Kind: DriftLiveUnfrozen,
			   Alert: AlertFreezeBroadcastDrift,
			  UserID: user, Asset: asset, TransferID: id,
			  Note: "live hash 没有对应冻结位",
		  })
	   }
	   for id, in := range intents {
		  r, ok := byTr[id]
		   bc, hasBC := byBC[id]
		  open := ok && freezeOpen(runFreeze(r))
		   live := hasBC && LiveHash(bc.TxHash)
		   if open || live {
			   continue
		  }
		  add(Drift{
			  Kind: DriftHotOrphan, Alert: AlertHotIntentDrift,
			   UserID: in.UserID, Asset: in.Asset, TransferID: id,
			  Note: "热出金意图无在途冻结且无 live hash",
		   })
	  }
	  return out
   }

   func alertList(drifts []Drift, blocks []Block) []string {
	  seen := map[string]struct{}{}
	   var out []string
	  push := func(n string) {
		   if n == "" {
			  return
		   }
		  if _, ok := seen[n]; ok {
			  return
		  }
		   seen[n] = struct{}{}
		  out = append(out, n)
	  }
	   for _, d := range drifts {
		  push(d.Alert)
	   }
	  if len(blocks) > 0 {
		   push(AlertUserAssetBlocked)
	   }
	  return out
  }

// ScanConserve 扫描三腿，刷新阻断并补开未登记的事故单。
// 账本上仍开着的出金并入冻结位；证明不了结清的旧阻断留在表上。冷启动冻结簿是空的，整表重写会把已回填的阻断删掉。

   func (s *Service) ScanConserve() ConserveReport {
	   drifts, retain := s.watchDrifts()
	  blocks := s.conserve.applyBlocks(drifts, retain)
	   s.openDriftTickets(drifts)
	   rep := ConserveReport{Drifts: drifts,
		   Blocks: blocks, Alerts: alertList(drifts, blocks)}
	  if err := s.conserve.readStoreErr(); err != nil {
		  rep.PersistErr = err.Error()
	   }
	   return rep
   }

   func (s *Service) openDriftTickets(drifts []Drift) {
	   if s == nil || s.tickets == nil || s.conserve == nil {
		   return
	  }
	   for _, d := range drifts {
		   if d.UserID == "" {
			  continue
		  }
		   key := string(d.Kind) + "|" + d.TransferID + "|" + d.Note
		   if _, ok := s.conserve.seenTicket(key); ok {
			   continue
		  }
		   note := d.Alert + " " + d.Note
		  ex, err := s.tickets.OpenException(conserveActor, d.UserID, d.TransferID, string(TicketConserve), note)
		   if err != nil {
			  continue
		  }
		  s.conserve.markTicket(key, ex.ID)
	   }
  }

// AllowNew 拒绝同用户同资产的新出金。入金贷记不走这里。

  func (s *Service) AllowNew(user, asset string) error {
	   if s == nil {
		   return nil
	  }
	   if s.conserve != nil && s.conserve.boundKV() != nil {
		   if err := s.conserve.readStoreErr(); err != nil {
			  return ErrGuardStore
		   }
	   }
	   user = strings.TrimSpace(user)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if s.abuse != nil && s.abuse.Blocks(user, asset) {
		  return ErrAbuseBlocked
	  }
	  for _, d := range s.collectDrifts() {
		  if d.UserID == user && strings.EqualFold(d.Asset, asset) {
			  if s.conserve != nil {
				  // 申请路径只增补阻断，不在这里解除别的用户资产。
				   s.conserve.applyBlocks(s.collectDrifts(), func(Block) bool { return true })
			  }
			   s.openDriftTickets([]Drift{d})
			   return ErrConserveBlocked
		   }
	   }
	  if _, ok := s.conserve.Blocked(user, asset); ok {
		   return ErrConserveBlocked
	   }
	  return nil
   }

  func (s *Service) noteHotIntent(trID, user, asset, amount, chain string) {
	  if s == nil || s.conserve == nil {
		  return
	   }
	  s.conserve.NoteIntent(HotIntent{
		   TransferID: trID, UserID: user, Asset: asset, Amount: amount, Chain: chain,
	  })
   }

   func (s *Service) dropHotIntent(trID string) {
	   if s == nil || s.conserve == nil {
		   return
	   }
	  s.conserve.DropIntent(trID)
  }
