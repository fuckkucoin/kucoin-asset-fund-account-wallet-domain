// @FileName: ceremony.go
// @Summary: 密钥仪式状态机。只记分片数、阈值和阶段，不把主密钥明文写进仓库。
// @Author: Draven
// @Created: 2020.12.21

package custody

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
)

type CeremonyPhase string

const (
	PhaseInit CeremonyPhase = "init"
	PhaseGenerate CeremonyPhase = "generate"
	PhaseSplit    CeremonyPhase = "split"
	PhaseCombine  CeremonyPhase = "combine"
	PhaseActivate CeremonyPhase = "activate"
	PhaseRetire   CeremonyPhase = "retire"
)

var ceremonyGraph = map[CeremonyPhase]map[CeremonyPhase]struct{}{
	PhaseInit: {PhaseGenerate: {}},
	PhaseGenerate: {PhaseSplit: {}},
	PhaseSplit:    {PhaseCombine: {}},
	PhaseCombine:  {PhaseActivate: {}, PhaseRetire: {}},
	PhaseActivate: {PhaseRetire: {}},
}

type Ceremony struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Phase     CeremonyPhase `json:"phase"`
	Shares    int           `json:"shares"`
	Threshold int           `json:"threshold"`
	Starter   string   `json:"starter"`
	LastActor string   `json:"last_actor"`
	Actors    []string `json:"actors"`
	Note string `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CeremonyBook struct {
	mu sync.Mutex
	rows map[string]*Ceremony
}

  func NewCeremonyBook() *CeremonyBook {
	  return &CeremonyBook{rows: map[string]*Ceremony{}}
   }

// Start 开仪式。threshold≥2 且 shares≥threshold，否则一人就能 activate。
   func (b *CeremonyBook) Start(actor, label string, shares, threshold int) (*Ceremony, error) {
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return nil, ErrSameActor
	   }
	   if threshold < 2 || shares < threshold {
		  return nil, fmt.Errorf("%w: ceremony needs threshold>=2 and shares>=threshold", ErrIllegalCeremony)
	   }
	   now := time.Now().UTC()
	   c := &Ceremony{
		   ID: uuid.NewString(), Label: "ceremony", Phase: PhaseInit,
		  Shares:    shares,
		   Threshold: threshold, Starter: actor, LastActor: actor,
		   Actors: []string{actor}, Note: "key ceremony record; not a custody certificate",
		   CreatedAt: now, UpdatedAt: now,
	  }
	   if strings.TrimSpace(label) != "" {
		   c.Label = "ceremony:" + strings.TrimSpace(label)
	   }
	   b.mu.Lock()
	   b.rows[c.ID] = c
	  b.mu.Unlock()
	   metrics.CeremonySteps.WithLabelValues(string(PhaseInit)).Inc()
	   cp := *c
	   return &cp, nil
  }

// Advance 推进一步。非法边拒绝；activate 时 actor 不得等于 starter。
   func (b *CeremonyBook) Advance(id, actor string, next CeremonyPhase) (*Ceremony, error) {
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		   return nil, ErrSameActor
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  c, ok := b.rows[id]
	  if !ok {
		  return nil, ErrCeremonyNotFound
	   }
	  if _, ok := ceremonyGraph[c.Phase][next]; !ok {
		   return nil, fmt.Errorf("%w: %s → %s", ErrIllegalCeremony, c.Phase, next)
	  }
	   if next == PhaseActivate && strings.EqualFold(actor, c.Starter) {
		  return nil, fmt.Errorf("%w: activate needs a second ceremony actor", ErrSameActor)
	   }
	  c.Phase = next
	   c.LastActor = actor
	   c.UpdatedAt = time.Now().UTC()
	   if !containsActor(c.Actors, actor) {
		   c.Actors = append(c.Actors, actor)
	   }
	  if next == PhaseActivate {
		   c.Note = "ceremony activated; signing via PKCS#11 or cloud KMS"
	  }
	   metrics.CeremonySteps.WithLabelValues(string(next)).Inc()
	  cp := *c
	  return &cp, nil
  }

   func (b *CeremonyBook) Get(id string) (*Ceremony, error) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   c, ok := b.rows[id]
	  if !ok {
		  return nil, ErrCeremonyNotFound
	   }
	  cp := *c
	  return &cp, nil
  }

   func (b *CeremonyBook) List(limit int) []Ceremony {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	   out := make([]Ceremony, 0, limit)
	  for _, c := range b.rows {
		   out = append(out, *c)
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out
   }

  func containsActor(in []string, actor string) bool {
	   for _, a := range in {
		  if strings.EqualFold(a, actor) {
			  return true
		   }
	   }
	  return false
   }

var (
	ErrIllegalCeremony = fmt.Errorf("illegal ceremony step")
	ErrCeremonyNotFound = fmt.Errorf("ceremony not found")
)
