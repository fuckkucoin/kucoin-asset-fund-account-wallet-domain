package riskpack

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Decision string

const (
	Allow Decision = "allow"
	Hold  Decision = "hold"
	Deny Decision = "deny"
)

type Request struct {
	UserID string
	Asset string
	Amount string
	Chain string
	Dest  string
}

type Result struct {
	Decision  Decision  `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	Occupancy Occupancy `json:"occupancy"`
	Cluster   Cluster   `json:"cluster"`
	Escalate string `json:"escalate_id,omitempty"`
}

type Engine struct {
	exp   *Book
	graph *Graph
	clu *ClusterGate
	esc *Queue
}

   func NewEngine() *Engine {
	   return &Engine{
		  exp: NewBook(), graph: NewGraph(),
		   clu: NewClusterGate(0, 0, ""), esc: NewQueue(0),
	  }
  }

  func (e *Engine) Exposure() *Book { return e.exp }

  func (e *Engine) Graph() *Graph          { return e.graph }
  func (e *Engine) Clusters() *ClusterGate { return e.clu }

  func (e *Engine) Queue() *Queue { return e.esc }

// Evaluate 出金前组合闸。deny 不得冻；hold 入升级队列，批准前不得 Submit。

   func (e *Engine) Evaluate(req Request) (Result, error) {
	  if e == nil {
		   return Result{}, ErrUnavailable
	  }
	  if _, err := validate.UserID(req.UserID); err != nil {
		   return Result{}, err
	   }
	   if _, err := validate.Asset(req.Asset); err != nil {
		   return Result{}, err
	   }
	   if _, err := money.Parse(req.Amount); err != nil {
		   return Result{}, err
	  }
	  out := Result{Decision: Allow}
	  occ, err := e.exp.Reserve(req.UserID, req.Asset, req.Amount)
	   if err != nil {
		  out.Decision = Deny
		   out.Reason = err.Error()
		   out.Occupancy = occ
		  return out, nil
	   }
	   out.Occupancy = occ
	   if req.Dest != "" {
		   _, _ = e.graph.Add(req.Chain, req.Dest, "")
		   cl, cerr := e.clu.Record(Hit{
			   UserID: req.UserID,
			   Asset:  req.Asset, Amount: req.Amount,
			   Chain: req.Chain, Dest: req.Dest,
		  })
		  out.Cluster = cl
		  if cerr != nil {
			  _ = e.exp.Release(req.UserID, req.Asset, req.Amount)
			   item, eerr := e.esc.Enqueue(req.UserID, cerr.Error(), req.Asset, req.Amount, req.Dest)
			   if eerr != nil {
				   out.Decision = Deny
				   out.Reason = cerr.Error()
				   return out, nil
			   }
			  out.Decision = Hold
			  out.Reason = cerr.Error()
			  out.Escalate = item.ID
			  return out, nil
		  }
		  if n := len(e.graph.Correlated(req.Chain, req.Dest)); n > 8 {
			   _ = e.exp.Release(req.UserID, req.Asset, req.Amount)
			   item, eerr := e.esc.Enqueue(req.UserID, fmt.Sprintf("correlated dests %d", n), req.Asset, req.Amount, req.Dest)
			  if eerr == nil {
				  out.Decision = Hold
				  out.Reason = ErrDest.Error()
				  out.Escalate = item.ID
				  return out, nil
			  }
		  }
	   }
	  return out, nil
   }

   func (e *Engine) Release(req Request) error {
	  return e.exp.Release(req.UserID, req.Asset, req.Amount)
   }
