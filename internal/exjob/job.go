package exjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/stmt"
	"github.com/google/uuid"
)

var (
	ErrUser = errors.New("export user required")
	ErrKind   = errors.New("export kind required")
	ErrFormat = errors.New("export format csv|json")
	ErrGone  = errors.New("export job not found")
	ErrOwner = errors.New("export job owner mismatch")
	ErrDaily = errors.New("export daily limit reached")
	ErrChecksum = errors.New("export body checksum mismatch")
)

const (
	kind = "aa_export_job"
	dailyLimit = 5
	maxWin     = 100 * 24 * time.Hour
)

type Job struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Status string `json:"status"`
	StartAt time.Time `json:"start_at,omitempty"`
	EndAt time.Time `json:"end_at,omitempty"`
	Body     string `json:"body,omitempty"`
	Checksum string `json:"checksum,omitempty"`
	Err      string `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiredAt time.Time `json:"expired_at,omitempty"`
}

type Book struct {
	mu   sync.Mutex
	jobs map[string]Job
	kv persist.KV
	led *ledger.Service
	sql *SQLStore
	audits    []stmt.AuditEvent
	expireCfg ExpireLoopConfig
	expireBound bool
}

  func New() *Book { return &Book{jobs: map[string]Job{}} }

   func (b *Book) BindKV(kv persist.KV) *Book {
	   if b != nil {
		   b.kv = kv
	   }
	  return b
   }

   func (b *Book) BindLedger(led *ledger.Service) *Book {
	   if b != nil {
		  b.led = led
	  }
	  return b
   }

// Enqueue 缺省按近 100 天窗口导出，一致支持中心单次窗口上限。
  func (b *Book) Enqueue(userID, kindName, format string) (Job, error) {
	  end := time.Now().UTC()
	   start := end.Add(-maxWin)
	  return b.EnqueueWindow(userID, kindName, format, start, end)
   }

  func (b *Book) EnqueueWindow(userID, kindName, format string, start, end time.Time) (Job, error) {
	   _, _, _ = b.ExpireAndReclaim(time.Now().UTC(), 0, 0)
	   userID = strings.TrimSpace(userID)
	  kindName = strings.ToLower(strings.TrimSpace(kindName))
	   format = strings.ToLower(strings.TrimSpace(format))
	  if userID == "" {
		  return Job{}, ErrUser
	  }
	  switch kindName {
	   case "deposit", "withdraw", "convert", "fiat", "earn", "p2p", "all":
	  default:
		   return Job{}, ErrKind
	   }
	   switch format {
	   case "", "csv":
		  format = "csv"
	   case "json":
	  default:
		  return Job{}, ErrFormat
	  }
	  if start.IsZero() && end.IsZero() {
		   end = time.Now().UTC()
		  start = end.Add(-maxWin)
	  }
	  if err := fundsurf.CheckExportWindow(start, end); err != nil {
		   return Job{}, err
	  }
	   if err := b.checkDaily(userID); err != nil {
		   return Job{}, err
	   }
	   j := Job{
		  ID:     uuid.NewString(),
		   UserID: userID, Kind: kindName, Format: format,
		   Status: "queued", StartAt: start, EndAt: end, CreatedAt: time.Now().UTC(),
	   }
	   j.Status = StatusQueued
	  if err := b.render(&j); err != nil {
		   st,
			  _ := AdvanceStatus(j.Status, StatusFailed)
		  j.Status = st
		  return Job{}, err
	  }
	   if j.Body != "" {
		  st, err := AdvanceStatus(StatusQueued, StatusRunning)
		  if err != nil {
  }

  func (b *Book) checkDaily(userID string) error {
	   if b == nil {
		  return nil
	   }
	  day := time.Now().UTC().Format("2006-01-02")
	  seen := map[string]struct{}{}
	   n := 0
	  b.mu.Lock()
	   for id, j := range b.jobs {
		  if j.UserID == userID && j.CreatedAt.UTC().Format("2006-01-02") == day {
			   seen[id] = struct{}{}
			   n++
		   }
	   }
	  b.mu.Unlock()
	   if b.kv != nil {
		   ctx := context.Background()
		   cur := persist.Cursor{Limit: 200}
		   for {
			  page, err := persist.PageKind(ctx, b.kv, kind, cur)
			  if err != nil {
				  return err
			  }
			   for _, row := range page.Rows {
				   if _, ok := seen[row.Key]; ok {
					   continue
				   }
				   var stored Job
				  if json.Unmarshal(row.Body, &stored) != nil || stored.UserID != userID {
					   continue
				  }
				  if stored.CreatedAt.UTC().Format("2006-01-02") == day {
					   seen[row.Key] = struct{}{}
					   n++
				  }
			  }
			   if page.Done {
				  break
			  }
			   cur.After = page.Next
		  }
	  }
	   if b.sql != nil {
		   dayStart, _ := time.ParseInLocation("2006-01-02", day, time.UTC)
		  cn, err := b.sql.CountToday(context.Background(), userID, dayStart)
		   if err != nil {
			   return err
		   }
		   if cn > n {
			   n = cn
		  }
	  }
	  if n >= dailyLimit {
		  return ErrDaily
	  }
	  return nil
  }

  func (b *Book) render(j *Job) error {
	  if b == nil || b.led == nil || j == nil {
		   return nil
	  }
	  list,
		   err := b.led.List(context.Background(), j.UserID, 500)
	   if err != nil {
		  return err
	  }
	   var bld strings.Builder
	   if j.Format == "csv" {
		  bld.WriteString("id,direction,asset,amount,chain,status,created_at\n")
	  }
	  rows := make([]map[string]string, 0)
	   for _, tr := range list {
		  if tr == nil {
			   continue
		   }
		  if !j.StartAt.IsZero() && tr.CreatedAt.Before(j.StartAt) {
			  continue
		  }
		   if !j.EndAt.IsZero() && tr.CreatedAt.After(j.EndAt) {
			   continue
		   }
		   switch j.Kind {
		  case "deposit":
			  if tr.Direction != ledger.WalletDeposit {
				   continue
			  }
		  case "withdraw":
			   if tr.Direction != ledger.WalletWithdraw && tr.Direction != ledger.SpotSend {
				   continue
			   }
		  case "convert":
			   if tr.Direction != ledger.ConvertDebit && tr.Direction != ledger.ConvertCredit {
				   continue
			   }
		  case "fiat":
			   if tr.Direction != ledger.FiatOnramp && tr.Direction != ledger.FiatOfframp {
				  continue
			  }
		  case "earn":
			  if tr.Direction != ledger.EarnSubscribe && tr.Direction != ledger.EarnRedeem && tr.Direction != ledger.EarnInterest {
				   continue
			  }
		  case "p2p":
			   // P2P 资金流水导出（买入入账/卖出占用）；盘口订单报表仍属交易面。
			  if tr.Direction != ledger.P2PCredit && tr.Direction != ledger.P2PDebit {
				  continue
			  }
		   }
		   if j.Format == "csv" {
			   bld.WriteString(tr.ID + "," + string(tr.Direction) + "," + tr.Asset + "," + tr.Amount.String() + "," + tr.Chain + "," + string(tr.State) + "," + tr.CreatedAt.UTC().Format(time.RFC3339) + "\n")
			  continue
		   }
		   rows = append(rows, map[string]string{
			   "id": tr.ID, "direction": string(tr.Direction), "asset": tr.Asset,
			  "amount": tr.Amount.String(), "chain": tr.Chain, "status": string(tr.State),
		  })
	   }
	   if j.Format == "json" {
		  raw, err := json.Marshal(rows)
		  if err != nil {
			   return err
		   }
		   j.Body = string(raw)
	  } else {
		  j.Body = bld.String()
	  }
	  sum := sha256.Sum256([]byte(j.Body))
	   j.Checksum = hex.EncodeToString(sum[:])
	  return nil
  }

  func (b *Book) save(j Job, exp persist.Fence) error {
	   b.mu.Lock()
	  b.jobs[j.ID] = j
	  b.mu.Unlock()
	   if b.sql != nil {
		  if err := b.sql.UpsertMeta(context.Background(), j); err != nil {
			   return err
		  }
	   }
	  if b.kv == nil {
		   return nil
	   }
	  body, err := json.Marshal(j)
	   if err != nil {
		  return err
	  }
	   return b.kv.Put(context.Background(),
		   persist.Row{Kind: kind, Key: j.ID, Body: body}, exp)
   }

  func (b *Book) Get(id string) (Job, error) {
	   id = strings.TrimSpace(id)
	   b.mu.Lock()
	   j, ok := b.jobs[id]
	   b.mu.Unlock()
	   if ok {
		  return j, nil
	   }
	  if b.kv != nil {
		   row, err := b.kv.Get(context.Background(), kind, id)
		   if err == nil {
			   var stored Job
			  if json.Unmarshal(row.Body, &stored) == nil && stored.ID != "" {
				   return stored, nil
			  }
		  }
	   }
	  return Job{}, ErrGone
   }

   func (b *Book) GetFor(userID, id string) (Job, error) {
	  _,
		  _, _ = b.ExpireAndReclaim(time.Now().UTC(), 0, 0)
	  j, err := b.Get(id)
	   if err != nil {
		   return Job{}, err
	  }
	  if userID != "" && j.UserID != userID {
		  return Job{}, ErrOwner
	  }
	   if err := b.EnsureFresh(j, time.Now().UTC()); err != nil {
		  return j, err
	  }
	   return j, nil
  }

// NoteAudit 记录一次导出审计；校验失败则丢弃。
  func (b *Book) NoteAudit(e stmt.AuditEvent) error {
	  if b == nil {
		   return nil
	   }
	  if err := e.Validate(); err != nil {
		   return err
	   }
	  b.mu.Lock()
	   b.audits = append(b.audits, e)
	  b.mu.Unlock()
	   return nil
  }

// Audits 返回已记录的导出审计副本。

   func (b *Book) Audits() []stmt.AuditEvent {
	   if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   out := make([]stmt.AuditEvent, len(b.audits))
	  copy(out, b.audits)
	   return out
   }

// GetBody 仅 done 状态返回正文；expired/failed/queued 拒绝。
   func (b *Book) GetBody(id, userID string) (string, error) {
	   _,
		  _, _ = b.ExpireAndReclaim(time.Now().UTC(), 0, 0)
	  j, err := b.Get(id)
	  if err != nil {
		   return "", err
	  }
	   if j.UserID != strings.TrimSpace(userID) {
		   return "", ErrOwner
	  }
	  if err := b.EnsureFresh(j, time.Now().UTC()); err != nil {
		  return "", err
	   }
	   if j.Status != StatusDone {
		   return "", fmt.Errorf("export body unavailable: status=%s", j.Status)
	   }
	   if j.Checksum != "" {
		   sum := sha256.Sum256([]byte(j.Body))
		   if hex.EncodeToString(sum[:]) != j.Checksum {
			   return "", ErrChecksum
		  }
	   }
	   return j.Body, nil
   }
