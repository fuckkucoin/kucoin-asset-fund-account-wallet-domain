package xferloop

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrActor 指派必须是具体运维身份。
var ErrActor = errors.New("xfer loop actor required")

// Query 工作台过滤。空字段表示不限。
type Query struct {
	Kind  Kind
	State State
	Asset string
	UserID string
	Breached *bool
	Limit    int
}

// Board 收口看板。

type Board struct {
	Open      int `json:"open"`
	Escalated int `json:"escalated"`
	Breached int `json:"breached"`
	Recovered int            `json:"recovered"`
	ByKind    map[string]int `json:"by_kind"`
	ByState   map[string]int `json:"by_state"`
	Oldest *Case     `json:"oldest,omitempty"`
	At     time.Time `json:"at"`
}

// ListQuery 按条件列未恢复案件。
  func (b *Book) ListQuery(q Query) []Case {
	   if b == nil {
		  return nil
	   }
	  _ = b.hydrate(context.Background())
	   limit := q.Limit
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	  asset := strings.ToUpper(strings.TrimSpace(q.Asset))
	   user := strings.TrimSpace(q.UserID)
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   out := make([]Case, 0, limit)
	  for _, c := range b.rows {
		   if c == nil || c.State == StateRecovered {
			   continue
		   }
		  if q.Kind != "" && c.Kind != q.Kind {
			   continue
		   }
		  if q.State != "" && c.State != q.State {
			   continue
		  }
		   if asset != "" && strings.ToUpper(c.Asset) != asset {
			  continue
		   }
		   if user != "" && c.UserID != user {
			  continue
		  }
		   if q.Breached != nil && c.SLABreach != *q.Breached {
			  continue
		  }
		   out = append(out, *c)
		   if len(out) >= limit {
			   break
		  }
	  }
	  return out
  }

// Board 汇总未恢复与已恢复案件。
   func (b *Book) Board() Board {
	  rep := Board{
		  ByKind: map[string]int{}, ByState: map[string]int{}, At: time.Now().UTC(),
	  }
	   if b == nil {
		   return rep
	  }
	   _ = b.hydrate(context.Background())
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   var oldest *Case
	  for _, c := range b.rows {
		   if c == nil {
			   continue
		  }
		  applyPolicy(c, rep.At)
		   rep.ByKind[string(c.Kind)]++
		   rep.ByState[string(c.State)]++
		   if c.State == StateRecovered {
			   rep.Recovered++
			   continue
		   }
		  rep.Open++
		  if c.State == StateEscalated {
			  rep.Escalated++
		   }
		  if c.SLABreach {
			  rep.Breached++
		   }
		  if oldest == nil || (!c.SeenAt.IsZero() && (oldest.SeenAt.IsZero() || c.SeenAt.Before(oldest.SeenAt))) {
			   cp := *c
			  oldest = &cp
		   }
	  }
	  rep.Oldest = oldest
	   return rep
  }

// Assign 把已升级或收口中的案件指派给运维。已恢复的不改。
   func (b *Book) Assign(ctx context.Context, id, actor string) (Case, error) {
	  if b == nil {
		  return Case{}, ErrNotFound
	  }
	   actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		   return Case{}, ErrActor
	  }
	   _ = b.hydrate(ctx)
	   b.mu.Lock()
	   c := b.rows[strings.TrimSpace(id)]
	  if c == nil {
		   b.mu.Unlock()
		   return Case{}, ErrNotFound
	   }
	  if c.State == StateRecovered {
		  cp := *c
		  b.mu.Unlock()
		   return cp, nil
	  }
	   c.Assignee = actor
	   c.UpdatedAt = time.Now().UTC()
	  cp := *c
	   b.mu.Unlock()
	   _ = b.save(ctx, &cp)
	   return cp, nil
   }
