package signpipe

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

type State string

const (
	StateQueued State = "queued"
	StateApproved State = "approved"
	StateSigned State = "signed"
	StateFailed    State = "failed"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

type Job struct {
	ID string `json:"id"`
	BatchID    string `json:"batch_id,omitempty"`
	MovementID string `json:"movement_id"`
	Chain      string `json:"chain"`
	To string `json:"to"`
	Amount    string `json:"amount"`
	Canonical string `json:"canonical"`
	RequestedBy string `json:"requested_by"`
	ApprovedBy string `json:"approved_by,omitempty"`
	SignedBy   string `json:"signed_by,omitempty"`
	State      State  `json:"state"`
	SigHex  string `json:"sig_hex,omitempty"`
	Note    string `json:"note,omitempty"`
	Retries int    `json:"retries,omitempty"`
	ExpiresAt time.Time     `json:"expires_at"`
	Fence     persist.Fence `json:"fence"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Signer func(canonical string) (string,
	  error)

type Pipe struct {
	mu  sync.Mutex
	ttl time.Duration
	clock func() time.Time
	rows  map[string]*Job
	cold ColdAllow
}

   func New(ttl time.Duration) *Pipe {
	   if ttl <= 0 {
		  ttl = 10 * time.Minute
	  }
	   return &Pipe{ttl: ttl, clock: func() time.Time { return time.Now().UTC() }, rows: map[string]*Job{}}
  }

   func Canonical(movementID, chain, to, amount string) string {
	   return strings.Join([]string{movementID, strings.ToUpper(chain), strings.ToLower(to), amount}, "|")
   }

  func actorOK(actor string) error {
	   actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return fmt.Errorf("sign pipeline requires a distinct actor")
	  }
	  return nil
   }

// Open 登记待签。amount 必须过 money.Parse。同一 movement 未结单复用。
  func (p *Pipe) Open(actor, movementID, chain, to, amount string) (*Job, error) {
	  if err := actorOK(actor); err != nil {
		  return nil, err
	  }
	   if _, err := money.Parse(amount); err != nil {
		  return nil, err
	   }
	  movementID = strings.TrimSpace(movementID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  to = strings.TrimSpace(to)
	   if movementID == "" || chain == "" || to == "" {
		  return nil, fmt.Errorf("movement, chain and destination required")
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   for _, j := range p.rows {
		   if j.MovementID == movementID && (j.State == StateQueued || j.State == StateApproved) {
			  cp := *j
			  return &cp, nil
		   }
	   }
	   now := p.clock()
	   j := &Job{
		  ID: uuid.NewString(), MovementID: movementID, Chain: chain, To: to, Amount: amount,
		   Canonical: Canonical(movementID, chain, to, amount), RequestedBy: strings.TrimSpace(actor),
		   State: StateQueued, ExpiresAt: now.Add(p.ttl), CreatedAt: now, UpdatedAt: now,
		  Fence: persist.Fence{Epoch: 1, Token: persist.NewToken()},
	   }
	  p.rows[j.ID] = j
	  metrics.SignPipeline.WithLabelValues("queued").Inc()
	   cp := *j
	   return &cp, nil
  }

  func (p *Pipe) getLocked(id string) (*Job, error) {
	   j, ok := p.rows[id]
	   if !ok {
		   return nil, fmt.Errorf("sign job not found")
	  }
	   return j, nil
  }

   func (p *Pipe) checkFence(j *Job, epoch int64, token string) error {
	  if !j.Fence.Match(epoch, token) {
		   return persist.ErrStaleFence
	  }
	  return nil
   }

  func (p *Pipe) bump(j *Job) {
	  j.Fence.Epoch++
	  j.Fence.Token = persist.NewToken()
	  j.UpdatedAt = p.clock()
  }

// Approve 换人复核。canonical 必须与开单时一致，防止批一张单去签另一笔。
   func (p *Pipe) Approve(id, actor string, epoch int64, token, movementID, chain, to, amount string) (*Job, error) {
	   if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	  p.mu.Lock()
	  defer p.mu.Unlock()
	  j, err := p.getLocked(id)
	   if err != nil {
		  return nil, err
	  }
	  if err := p.checkFence(j, epoch, token); err != nil {
		   return nil, err
	   }
	  if j.State != StateQueued {
		   return nil, fmt.Errorf("sign job not queued")
	  }
	  if p.clock().After(j.ExpiresAt) {
		   j.State = StateExpired
		   p.bump(j)
		  metrics.SignPipeline.WithLabelValues("expired").Inc()
		   return nil, fmt.Errorf("sign job expired")
	   }
	  if strings.EqualFold(actor, j.RequestedBy) {
		  return nil, fmt.Errorf("approver must be a distinct actor")
	   }
	  want := Canonical(movementID, chain, to, amount)
	  if want != j.Canonical {
		   return nil, fmt.Errorf("canonical mismatch")
	   }
	   j.ApprovedBy = strings.TrimSpace(actor)
	   j.State = StateApproved
	  p.bump(j)
	   metrics.SignPipeline.WithLabelValues("approved").Inc()
	  cp := *j
	  return &cp, nil
  }

// Sign 第三人执行。signer 失败整单标 failed，围栏已加，旧 token 不能重试成功路径。
  func (p *Pipe) Sign(id, actor string, epoch int64, token string, signer Signer) (*Job, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	  if signer == nil {
		  return nil, fmt.Errorf("signer unavailable")
	   }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  j, err := p.getLocked(id)
	   if err != nil {
		   return nil, err
	   }
	  if err := p.checkFence(j, epoch, token); err != nil {
		   return nil, err
	  }
	  if j.State != StateApproved {
		  return nil, fmt.Errorf("sign job not approved")
	   }
	  if p.clock().After(j.ExpiresAt) {
		   j.State = StateExpired
		  p.bump(j)
		  metrics.SignPipeline.WithLabelValues("expired").Inc()
		   return nil, fmt.Errorf("sign job expired")
	  }
	   if strings.EqualFold(actor, j.RequestedBy) || strings.EqualFold(actor, j.ApprovedBy) {
		  return nil, fmt.Errorf("signer must be a third actor")
	  }
	   if err := p.rejectCold(j); err != nil {
		  return nil, err
	  }
	   sig, err := signer(j.Canonical)
	  if err != nil {
		   j.State = StateFailed
		  j.Note = err.Error()
		   p.bump(j)
		   metrics.SignPipeline.WithLabelValues("failed").Inc()
		   return nil, err
	   }
	   j.SignedBy = strings.TrimSpace(actor)
	  j.SigHex = sig
	  j.State = StateSigned
	  p.bump(j)
	   metrics.SignPipeline.WithLabelValues("signed").Inc()
	   cp := *j
	   return &cp, nil
   }

   func (p *Pipe) Cancel(id, actor string, epoch int64, token string) (*Job, error) {
	   if err := actorOK(actor); err != nil {
		  return nil, err
	   }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   j, err := p.getLocked(id)
	   if err != nil {
		  return nil, err
	  }
	   if err := p.checkFence(j, epoch, token); err != nil {
		   return nil, err
	   }
	  if j.State == StateSigned {
		  return nil, fmt.Errorf("signed job cannot cancel")
	  }
	   j.State = StateCancelled
	   j.Note = "cancelled by " + actor
	   p.bump(j)
	  metrics.SignPipeline.WithLabelValues("cancelled").Inc()
	   cp := *j
	   return &cp, nil
   }

  func (p *Pipe) ExpireDue() []Job {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  now := p.clock()
	   out := make([]Job, 0)
	  for _, j := range p.rows {
		   if j.State != StateQueued && j.State != StateApproved {
			  continue
		  }
		  if now.Before(j.ExpiresAt) {
			   continue
		  }
		  j.State = StateExpired
		  p.bump(j)
		  metrics.SignPipeline.WithLabelValues("expired").Inc()
		  out = append(out, *j)
	  }
	  return out
   }

  func (p *Pipe) Get(id string) (*Job, error) {
	  p.mu.Lock()
	  defer p.mu.Unlock()
	  j, err := p.getLocked(id)
	  if err != nil {
		   return nil, err
	  }
	   cp := *j
	   return &cp, nil
  }

   func (p *Pipe) List(limit int) []Job {
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	   }
	   out := make([]Job, 0, limit)
	   for _, j := range p.rows {
		  out = append(out, *j)
		  if len(out) >= limit {
			   break
		  }
	   }
	  return out
   }
