package withdraw

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type BroadcastStatus string

const (
	BcSigned BroadcastStatus = "signed"
	BcBroadcast BroadcastStatus = "broadcast"
	BcConfirming BroadcastStatus = "confirming"
	BcConfirmed  BroadcastStatus = "confirmed"
	BcReorged BroadcastStatus = "reorged"
	BcFailed BroadcastStatus = "failed"
)

// Saga 相位：reorged → resigning → resubmitting → broadcast/confirming。

const (
	SagaNone      = ""
	SagaReorged   = "reorged"
	SagaResigning = "resigning"
	SagaResubmitting = "resubmitting"
)

type Broadcast struct {
	TransferID string `json:"transfer_id"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Status BroadcastStatus `json:"status"`
	Confirmations int   `json:"confirmations"`
	Need          int   `json:"need"`
	Height        int64 `json:"height,omitempty"`
	BlockHash string `json:"block_hash,omitempty"`
	Err       string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	SagaPhase string    `json:"saga_phase,omitempty"`
	OldTxHash string    `json:"old_tx_hash,omitempty"`
	ResubmitN int `json:"resubmit_n,omitempty"`
	SagaNote string `json:"saga_note,omitempty"`
}

type Broadcaster struct {
	mu   sync.Mutex
	rows map[string]*Broadcast
	need  map[string]int
	clock func() time.Time
	sql *persist.SQLDB
	lastPersistErr string
	hydrateErr string
	hydratedAt time.Time
	hydratedN int
}

  func NewBroadcaster() *Broadcaster {
	   return &Broadcaster{
		   rows:  map[string]*Broadcast{},
		   need:  map[string]int{"ETH": 12, "BTC": 3, "TRON": 20, "SOL": 32, "BASE": 20, "TON": 16},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// BindSQL 广播确认落库；OpenMem 不绑。

   func (b *Broadcaster) BindSQL(db *persist.SQLDB) *Broadcaster {
	   if b != nil {
		  b.sql = db
	  }
	   return b
  }

  func (b *Broadcaster) sqlOn() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
  }

// Hydrate 启动从 SQL 回填广播确认，跨重启 By-ID remainingConfirmations 可观测。
// Open 后须 Hydrate；失败不得清空内存已跟踪行。
// Hydrate 失败时确认面空，不得捏造未上链确认数。
   func (b *Broadcaster) Hydrate(ctx context.Context) error {
	   if b == nil || !b.sqlOn() {
		  return nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  list, err := sqlrepo.NewWDTrack(b.sql).ListBroadcasts(ctx)
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if err != nil {
		  b.hydrateErr = err.Error()
		   return err
	   }
	   if b.rows == nil {
		   b.rows = map[string]*Broadcast{}
	   }
	  for _, row := range list {
		  cp := Broadcast{
			  TransferID: row.TransferID,
			  Chain: row.Chain, TxHash: row.TxHash, Asset: row.Asset,
			  Amount: row.Amount, Status: BroadcastStatus(row.Status), Confirmations: row.Confirmations,
			  Need: row.Need, Height: row.Height, BlockHash: row.BlockHash, Err: row.Err, UpdatedAt: row.UpdatedAt,
			   SagaPhase: row.SagaPhase, OldTxHash: row.OldTxHash, ResubmitN: row.ResubmitN, SagaNote: row.SagaNote,
		   }
		  b.rows[row.TransferID] = &cp
	  }
	   b.hydratedN = len(list)
	  b.hydratedAt = b.clock()
	   b.hydrateErr = ""
	  return nil
   }

// TrackHealth 广播 SQL Hydrate/落库健康面（供 ops）。

type TrackHealth struct {
	SQLBound bool `json:"sql_bound"`
	HydratedN int `json:"hydrated_n"`
	HydratedAt time.Time `json:"hydrated_at,omitempty"`
	HydrateErr string    `json:"hydrate_err,omitempty"`
	MemoryN int `json:"memory_n"`
	LastPersistErr string `json:"last_persist_err,omitempty"`
}

// Health 只读广播跟踪健康。
   func (b *Broadcaster) Health() TrackHealth {
	   if b == nil {
		   return TrackHealth{}
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  return TrackHealth{
		   SQLBound: b.sqlOn(), HydratedN: b.hydratedN, HydratedAt: b.hydratedAt,
		   HydrateErr: b.hydrateErr, MemoryN: len(b.rows), LastPersistErr: b.lastPersistErr,
	   }
  }

// persistLocked 持锁落库广播确认；错误写入 lastPersistErr 并返回。
// Observe/Track/Fail 均在 mu 内调用；OpenMem 跳过。
// 落库失败不得假装已持久化；调用方须上浮错误。

   func (b *Broadcaster) persistLocked(row *Broadcast) error {
	   if !b.sqlOn() || row == nil {
		  return nil
	  }
	  err := sqlrepo.NewWDTrack(b.sql).PutBroadcast(context.Background(), sqlrepo.WDBroadcast{
		  TransferID: row.TransferID, Chain: row.Chain, TxHash: row.TxHash, Asset: row.Asset,
		  Amount: row.Amount, Status: string(row.Status), Confirmations: row.Confirmations,
		   Need: row.Need, Height: row.Height, BlockHash: row.BlockHash, Err: row.Err, UpdatedAt: row.UpdatedAt,
		   SagaPhase: row.SagaPhase, OldTxHash: row.OldTxHash, ResubmitN: row.ResubmitN, SagaNote: row.SagaNote,
	  })
	   if err != nil {
		  b.lastPersistErr = err.Error()
		   return fmt.Errorf("broadcast persist: %w", err)
	  }
	   b.lastPersistErr = ""
	  return nil
  }

   func (b *Broadcaster) Need(chain string, n int) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  b.need[strings.ToUpper(strings.TrimSpace(chain))] = n
  }

  func (b *Broadcaster) getNeed(chain string) int {
	  if n,
		   ok := b.need[chain]; ok && n > 0 {
		   return n
	   }
	  return 12
   }

// Track 从已签章开始跟广播。hash 空只能停在 signed，不得当已上链。

  func (b *Broadcaster) Track(trID, chain, hash, asset, amount string) (Broadcast, error) {
	   if strings.TrimSpace(trID) == "" {
		   return Broadcast{}, fmt.Errorf("transfer_id required")
	   }
	  if _, err := money.Parse(amount); err != nil {
		  return Broadcast{}, err
	  }
	   st := BcSigned
	  if strings.TrimSpace(hash) != "" {
		   st = BcBroadcast
	  }
	   row := Broadcast{
		   TransferID: trID, Chain: strings.ToUpper(strings.TrimSpace(chain)),
		  TxHash: strings.TrimSpace(hash), Asset: strings.ToUpper(strings.TrimSpace(asset)),
		   Amount: amount, Status: st, Need: b.getNeed(strings.ToUpper(strings.TrimSpace(chain))),
		   UpdatedAt: b.clock(),
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  cp := row
	   b.rows[trID] = &cp
	  if err := b.persistLocked(&cp); err != nil {
		  return row, err
	   }
	  return row, nil
   }

// Observe 用链上观察推进确认。重组回到 broadcast，确认数清零，不得当 confirmed。
  func (b *Broadcaster) Observe(trID string, obs wallet.Observation) (Broadcast, error) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  row, ok := b.rows[trID]
	   if !ok {
		   return Broadcast{}, fmt.Errorf("broadcast not tracked")
	  }
	   if row.Status == BcConfirmed || row.Status == BcFailed {
		  return *row, nil
	  }
	  if obs.Reorged || obs.Status == wallet.TxReorged {
		  // 重组进入 reorg→resubmit saga；保留旧 hash 供重提。
		  if row.OldTxHash == "" && row.TxHash != "" {
			  row.OldTxHash = row.TxHash
		  }
		   row.Status = BcReorged
		   row.SagaPhase = SagaReorged
		   row.Confirmations = 0
		  row.Err = "reorg"
		  row.SagaNote = "await_resign"
		  row.UpdatedAt = b.clock()
		  if err := b.persistLocked(row); err != nil {
			   return *row, err
		   }
		   return *row, nil
	   }
	  if row.TxHash != "" && obs.Hash != "" && row.TxHash != obs.Hash {
		  return *row, fmt.Errorf("hash mismatch: tracked=%s observed=%s", row.TxHash, obs.Hash)
	   }
	  if obs.Hash != "" {
		   row.TxHash = obs.Hash
	   }
	  row.Confirmations = obs.Confirmations
	   row.Height = obs.Height
	  row.BlockHash = obs.BlockHash
	  if row.Status == BcSigned && row.TxHash != "" {
		  row.Status = BcBroadcast
	   }
	   if row.Status == BcReorged && row.TxHash != "" && row.SagaPhase != SagaResigning && row.SagaPhase != SagaResubmitting {
		   // 同 hash 重新出现在链上：退出 saga 回确认路径
		   row.Status = BcBroadcast
		  row.SagaPhase = SagaNone
		  row.SagaNote = ""
		  row.Err = ""
	  }
	   if row.Confirmations > 0 && row.Status == BcBroadcast {
		   row.Status = BcConfirming
	   }
	  if row.Confirmations >= row.Need && row.Need > 0 {
		   row.Status = BcConfirmed
	   }
	  row.UpdatedAt = b.clock()
	   if err := b.persistLocked(row); err != nil {
		  return *row, err
	   }
	  return *row, nil
   }

  func (b *Broadcaster) Fail(trID, reason string) (Broadcast, error) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   row, ok := b.rows[trID]
	   if !ok {
		  return Broadcast{}, fmt.Errorf("broadcast not tracked")
	   }
	  if row.Status == BcConfirmed {
		   return *row, fmt.Errorf("already confirmed")
	  }
	   row.Status = BcFailed
	  row.Err = reason
	  row.UpdatedAt = b.clock()
	   if err := b.persistLocked(row); err != nil {
		  return *row, err
	  }
	   return *row, nil
  }

  func (b *Broadcaster) Get(trID string) (Broadcast, bool) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   r, ok := b.rows[trID]
	   if !ok {
		  return Broadcast{}, false
	   }
	   return *r, true
   }

   func (b *Broadcaster) Len() int {
	   if b == nil {
		  return 0
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   return len(b.rows)
  }

   func (b *Broadcaster) All() []Broadcast {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Broadcast,
		  0, len(b.rows))
	  for _, r := range b.rows {
		   out = append(out, *r)
	  }
	   return out
   }

  func (b *Broadcaster) Confirming() []Broadcast {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Broadcast, 0)
	   for _, r := range b.rows {
		  if r.Status == BcBroadcast || r.Status == BcConfirming || r.Status == BcReorged {
			  out = append(out, *r)
		   }
	  }
	  return out
  }

// BeginResubmit 重组后进入 resigning 相位，清空当前 hash 等待重新签章。
// 须已是 reorged；落库后跨重启可续。
// 已确认不得重提；不得在无旧 hash 时捏造。

  func (b *Broadcaster) BeginResubmit(trID, note string) (Broadcast, error) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  row, ok := b.rows[trID]
	   if !ok {
		   return Broadcast{}, fmt.Errorf("broadcast not tracked")
	  }
	  if row.Status == BcConfirmed {
		  return *row, fmt.Errorf("already confirmed")
	   }
	   if row.Status != BcReorged && row.SagaPhase != SagaReorged && row.SagaPhase != SagaResigning {
		  return *row, fmt.Errorf("resubmit requires reorged status")
	   }
	  if row.OldTxHash == "" && row.TxHash != "" {
		   row.OldTxHash = row.TxHash
	   }
	  row.Status = BcSigned
	  row.SagaPhase = SagaResigning
	   row.TxHash = ""
	  row.Confirmations = 0
	  row.BlockHash = ""
	   row.Height = 0
	  row.Err = ""
	   row.SagaNote = strings.TrimSpace(note)
	   if row.SagaNote == "" {
		  row.SagaNote = "resigning_after_reorg"
	  }
	   row.UpdatedAt = b.clock()
	  if err := b.persistLocked(row); err != nil {
		  return *row, err
	   }
	   return *row, nil
   }

// CompleteResubmit 新签章 hash 上链后进入 resubmitting→broadcast。
// 新 hash 不得与 old_tx_hash 相同；ResubmitN++；落库。
// 空 hash 只能停在 resigning，不得当已广播。
   func (b *Broadcaster) CompleteResubmit(trID, newHash string) (Broadcast, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  row, ok := b.rows[trID]
	   if !ok {
		   return Broadcast{}, fmt.Errorf("broadcast not tracked")
	   }
	  if row.SagaPhase != SagaResigning && row.SagaPhase != SagaResubmitting && row.Status != BcSigned {
		  return *row, fmt.Errorf("complete resubmit requires resigning phase")
	   }
	   newHash = strings.TrimSpace(newHash)
	  if newHash == "" {
		   return *row, fmt.Errorf("new hash required")
	   }
	  if row.OldTxHash != "" && strings.EqualFold(row.OldTxHash, newHash) {
		  return *row, fmt.Errorf("resubmit hash must differ from reorged hash")
	  }
	  row.TxHash = newHash
	  row.Status = BcBroadcast
	  row.SagaPhase = SagaResubmitting
	   row.ResubmitN++
	   row.Confirmations = 0
	  row.Err = ""
	   row.SagaNote = "resubmitted"
	  row.UpdatedAt = b.clock()
	   if err := b.persistLocked(row); err != nil {
		  return *row, err
	  }
	  return *row, nil
  }

// OpenSagas 列出未完结的 reorg→resubmit saga（ops/metrics）。
   func (b *Broadcaster) OpenSagas() []Broadcast {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Broadcast, 0)
	  for _, r := range b.rows {
		  switch r.SagaPhase {
		  case SagaReorged,
			  SagaResigning, SagaResubmitting:
			   out = append(out, *r)
		   }
	   }
	   return out
   }

// SagaMaxAges 各相位开放 saga 最大年龄（秒）；告警调谐用。
// age 相对 UpdatedAt；无开放相位记 0。
// 时钟回拨会使 age 偏小，不得据此清告警。
  func (b *Broadcaster) SagaMaxAges() map[string]float64 {
	  out := map[string]float64{
		  SagaReorged: 0, SagaResigning: 0, SagaResubmitting: 0,
	   }
	  if b == nil {
		  return out
	   }
	   now := b.clock()
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  for _, r := range b.rows {
		   switch r.SagaPhase {
		  case SagaReorged, SagaResigning, SagaResubmitting:
			   age := now.Sub(r.UpdatedAt).Seconds()
			  if age < 0 {
				   age = 0
			  }
			   if age > out[r.SagaPhase] {
				  out[r.SagaPhase] = age
			  }
		  }
	  }
	   return out
   }

// AlignFSM 和共用出金图一致：signed→submit→submitted→confirm→confirmed。
  func AlignFSM(st BroadcastStatus) (string, error) {
	  m := fsm.Withdraw()
	   switch st {
	  case BcSigned:
		   return fsm.WdSigned, nil
	   case BcBroadcast, BcConfirming:
		   return fsm.WdSubmitted, nil
	  case BcConfirmed:
		  if _, err := m.Apply(fsm.WdSubmitted, fsm.WdEvConfirm); err != nil {
			   return "", err
		  }
		  return fsm.WdConfirmed, nil
	   case BcFailed:
		   return fsm.WdFailed, nil
	   case BcReorged:
		   return fsm.WdSubmitted, nil
	   default:
		  return "", fmt.Errorf("unknown broadcast status %s", st)
	   }
   }

   func (s *Service) Broadcasts() *Broadcaster {
	  if s == nil {
		  return NewBroadcaster()
	   }
	   if s.bc == nil {
		   s.bc = NewBroadcaster()
	  }
	   return s.bc
   }

// SagaTickResult reorg saga worker 一轮结果。

type SagaTickResult struct {
	AutoResign int `json:"auto_resign"`
	TimedOut   int `json:"timed_out"`
	Open int `json:"open"`
}

// DefaultSagaTimeout resign/resubmit 超时默认（超时 → fail）。

const DefaultSagaTimeout = 30 * time.Minute

// SagaTicketAge 开放 saga 软阈值；超过发 ticket，未到 DefaultSagaTimeout。
// 与 stuck ticket 区分——软阈告警运维跟进，硬阈 page 表示 worker 未推进。

const SagaTicketAge = 10 * time.Minute

// TickSagas worker 周期推进 reorg saga。
// reorged 自动进入 resigning；resigning/resubmitting 超时则 Fail。
// 已确认不得动；超时失败须可人工重开，不得吞旧 hash。

  func (b *Broadcaster) TickSagas(timeout time.Duration) SagaTickResult {
	  if b == nil {
		   return SagaTickResult{}
	   }
	  if timeout <= 0 {
		  timeout = DefaultSagaTimeout
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  now := b.clock()
	   out := SagaTickResult{}
	   for _, row := range b.rows {
		  switch row.SagaPhase {
		  case SagaReorged,
			   SagaResigning, SagaResubmitting:
			  out.Open++
		  default:
			   continue
		  }
		   if row.Status == BcConfirmed {
			   continue
		   }
		   age := now.Sub(row.UpdatedAt)
		   switch row.SagaPhase {
		  case SagaReorged:
			   // 自动进入 resigning，等待重新签章/重提
			   if row.OldTxHash == "" && row.TxHash != "" {
				   row.OldTxHash = row.TxHash
			  }
			  row.Status = BcSigned
			  row.SagaPhase = SagaResigning
			   row.TxHash = ""
			   row.Confirmations = 0
			  row.BlockHash = ""
			  row.Height = 0
			   row.Err = ""
			   row.SagaNote = "auto_resign"
			   row.UpdatedAt = now
			   _ = b.persistLocked(row)
			   out.AutoResign++
		   case SagaResigning,
			   SagaResubmitting:
			   if age >= timeout {
				   row.Status = BcFailed
				   row.Err = "saga_timeout"
				  row.SagaNote = "timeout_fail:" + row.SagaPhase
				  row.SagaPhase = SagaNone
				  row.UpdatedAt = now
				  _ = b.persistLocked(row)
				   out.TimedOut++
				  out.Open--
			  }
		  }
	  }
	  return out
   }

// TimedOutSagas 列出 saga 超时失败行（须人工重开）。
// Err=saga_timeout 或 SagaNote 前缀 timeout_fail；OldTxHash 须仍在。
// 不得把普通 Fail 当超时重开候选。
   func (b *Broadcaster) TimedOutSagas() []Broadcast {
	   if b == nil {
		   return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Broadcast, 0)
	  for _, r := range b.rows {
		   if isSagaTimeoutFail(r) {
			   out = append(out, *r)
		  }
	  }
	  return out
  }

  func isSagaTimeoutFail(row *Broadcast) bool {
	   if row == nil || row.Status != BcFailed {
		   return false
	  }
	  if row.Err == "saga_timeout" {
		  return true
	   }
	  return strings.HasPrefix(row.SagaNote, "timeout_fail:")
   }

   func priorPhaseFromTimeoutNote(note string) string {
	  note = strings.TrimSpace(note)
	  const pfx = "timeout_fail:"
	   if !strings.HasPrefix(note, pfx) {
		   return SagaResigning
	   }
	  ph := strings.TrimSpace(strings.TrimPrefix(note, pfx))
	  switch ph {
	   case SagaResigning, SagaResubmitting, SagaReorged:
		  return ph
	   default:
		  return SagaResigning
	   }
  }

// ReopenAfterTimeout 超时失败后人工重开 reorg→resubmit saga（ops）。
// 仅 saga_timeout；保留 OldTxHash；回到 resigning 等待重新签章。
// 已确认不得重开；无旧 hash 不得捏造；普通 Fail 拒。

  func (b *Broadcaster) ReopenAfterTimeout(trID, note string) (Broadcast, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   row, ok := b.rows[trID]
	  if !ok {
		   return Broadcast{}, fmt.Errorf("broadcast not tracked")
	  }
	   if row.Status == BcConfirmed {
		   return *row, fmt.Errorf("already confirmed")
	  }
	   if !isSagaTimeoutFail(row) {
		   return *row, fmt.Errorf("reopen requires saga_timeout failure")
	  }
	   if strings.TrimSpace(row.OldTxHash) == "" {
		   return *row, fmt.Errorf("reopen requires retained old_tx_hash")
	   }
	  _ = priorPhaseFromTimeoutNote(row.SagaNote) // 记录来源相位；统一回到 resigning
	   row.Status = BcSigned
	  row.SagaPhase = SagaResigning
	  row.TxHash = ""
	  row.Confirmations = 0
	  row.BlockHash = ""
	  row.Height = 0
	   row.Err = ""
	   note = strings.TrimSpace(note)
	  if note == "" {
		   note = "ops_reopen_after_timeout"
	  }
	  row.SagaNote = note
	  row.UpdatedAt = b.clock()
	   if err := b.persistLocked(row); err != nil {
		   return *row, err
	   }
	  return *row, nil
   }
