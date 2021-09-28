package policypack

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
)

type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeShadow  Mode = "shadow"
)

  func ParseMode(s string) Mode {
	  if strings.EqualFold(strings.TrimSpace(s), string(ModeShadow)) {
		   return ModeShadow
	   }
	   return ModeEnforce
   }

// Engine 代次化策略。缺包或评估失败默认失败即关；shadow 只记命中不挡单。

type Engine struct {
	cat        *Catalog
	mode       Mode
	failClosed bool
	hits *HitBook
}

  func NewEngine(cat *Catalog, mode Mode, failClosed bool) *Engine {
	   if cat == nil {
		  cat = NewCatalog()
	  }
	  return &Engine{cat: cat, mode: mode, failClosed: failClosed, hits: NewHitBook()}
  }

   func DefaultEngine() *Engine {
	   return NewEngine(NewCatalog(), ModeEnforce, true)
   }

  func (e *Engine) Mode() Mode {
	   if e == nil {
		  return ModeEnforce
	   }
	  return e.mode
  }

   func (e *Engine) Hits() *HitBook {
	  if e == nil {
		   return NewHitBook()
	  }
	  return e.hits
   }

  func (e *Engine) Catalog() *Catalog {
	  if e == nil {
		  return NewCatalog()
	   }
	  return e.cat
   }

// Decide 用冻结当时的包。评估错误在 fail-closed 下不得放行。
  func (e *Engine) Decide(name string, in Input) (Decision, error) {
	   if e == nil {
		   return Decision{}, fmt.Errorf("policy engine unavailable")
	  }
	  p, err := e.cat.Get(name)
	   if err != nil {
		   e.record(name, 0, "pack_missing", false)
		  if e.failClosed {
			   return Decision{Pack: name, Reason: "pack unavailable"}, err
		  }
		   return Decision{Allow: true, Pack: name, Reason: "pack missing open"}, nil
	   }
	   d, err := Eval(p, in)
	  if err != nil {
		  e.record(p.Name, p.Doc.Version, "eval_error", false)
		  if e.failClosed {
			   return Decision{Pack: p.Name, Version: p.Doc.Version, Reason: "eval failed"}, err
		   }
		   return Decision{Allow: true, Pack: p.Name, Version: p.Doc.Version, Reason: "eval error open"}, nil
	   }
	  rule := d.Reason
	  if rule == "" {
		   rule = "allow"
	   }
	   e.record(p.Name, p.Doc.Version, rule, d.Allow)
	   if !d.Allow && e.mode == ModeShadow {
		  d.Allow = true
		  d.Reason = "shadow:" + d.Reason
	  }
	   return d, nil
  }

   func (e *Engine) record(pack string, ver int, rule string, allow bool) {
	  if e.hits == nil {
		   e.hits = NewHitBook()
	   }
	  e.hits.Add(Hit{Pack: pack,
		   Version: ver, Rule: rule, Mode: string(e.mode), Allow: allow})
	  metrics.PolicyHits.WithLabelValues(pack, rule, string(e.mode)).Inc()
	  if e.mode == ModeShadow && !allow {
		  metrics.PolicyShadow.WithLabelValues(pack, rule).Inc()
	  }
  }
