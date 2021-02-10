package collide

import (
	"fmt"
	"strings"
	"time"
)

type edge struct {
	From State
	Kind string
	To State
	Guard string
}

// 迁移表。同一对 (from, kind) 最多一条能通过守卫。

var edges = []edge{
	{StClear, "conflict", StBlocked, "block_both"},
	{StClear, "conflict", StSuspect, "review"},
	{StSuspect, "conflict", StBlocked, ""},
	{StSuspect, "assign", StDesk, "actor"},
	{StSuspect, "timeout", StBlocked, "worker"},
	{StBlocked, "assign", StDesk, "actor"},
	{StDesk, "prove", StEvidence, "evidence"},
	{StDesk, "pick", StReleased, "pick"},
	{StDesk, "pick_none", StRejected, "reason"},
	{StDesk, "timeout", StExpired, "worker"},
	{StEvidence, "accept", StArbitrate, "actor"},
	{StEvidence, "reject_proof", StBlocked, "actor"},
	{StEvidence, "timeout", StExpired, "worker"},
	{StArbitrate, "pick", StReleased, "pick"},
	{StArbitrate, "pick_none", StRejected, "reason"},
	{StArbitrate, "timeout", StExpired, "worker"},
	{StReleased, "conflict", StBlocked, "fresh_user"},
	{StRejected, "conflict", StBlocked, "fresh_user"},
	{StExpired, "conflict", StBlocked, "fresh_user"},
}

// Transit 纯函数。不写存储。失败时返回原案和错误。

   func Transit(c Case, ev Event, now time.Time) (Case, error) {
	   if ev.Kind == "" {
		   return c, fmt.Errorf("collide event required")
	  }
	  var matched *edge
	  for i := range edges {
		   e := &edges[i]
		  if e.From != c.State || e.Kind != ev.Kind {
			   continue
		  }
		  if !guardOK(e.Guard, c, ev) {
			  continue
		  }
		  matched = e
		   break
	   }
	  if matched == nil {
		   return c, fmt.Errorf("collide illegal %s from %s", ev.Kind, c.State)
	   }
	  next := c
	  next.State = matched.To
	  next.Version++
	  if ev.At.IsZero() {
		   ev.At = now
	  }
	   next.Updated = ev.At
	   next.Log = append(append([]Event{},
		  c.Log...), ev)
	   if ev.User != "" && !hasClaim(next.Claimants, ev.User) && ev.User != next.Origin {
		  next.Claimants = append(next.Claimants, ev.User)
	   }
	  switch next.State {
	  case StReleased:
		   next.Winner = ev.Winner
		   next.Reason = ev.Reason
		   next.Deadline = time.Time{}
	   case StRejected, StExpired:
		  next.Winner = ""
		   next.Reason = ev.Reason
		   next.Deadline = time.Time{}
	  case StBlocked:
		  next.Winner = ""
		   if ev.Reason != "" {
			   next.Reason = ev.Reason
		   }
		   next.Deadline = time.Time{}
	  case StSuspect:
		   next.Deadline = ev.At.Add(30 * time.Minute)
	  case StDesk:
		  next.Deadline = ev.At.Add(24 * time.Hour)
		  next.Reason = ev.Reason
	  case StEvidence:
		   next.Evidence = ev.Evidence
		   next.Deadline = ev.At.Add(24 * time.Hour)
	  case StArbitrate:
		   next.Deadline = ev.At.Add(8 * time.Hour)
	   }
	   return next, nil
   }

   func guardOK(name string, c Case, ev Event) bool {
	  switch name {
	  case "":
		  return true
	  case "block_both":
		   return c.Mode != "review"
	   case "review":
		  return c.Mode == "review"
	   case "actor":
		  return actorOK(ev.Actor)
	   case "worker":
		  return ev.Actor == "attrib-worker"
	   case "evidence":
		  return actorOK(ev.Actor) && len(strings.TrimSpace(ev.Evidence)) >= 8
	  case "reason":
		   return actorOK(ev.Actor) && len(strings.TrimSpace(ev.Reason)) >= 8
	  case "pick":
		   if !actorOK(ev.Actor) || len(strings.TrimSpace(ev.Reason)) < 8 {
			   return false
		   }
		   return ev.Winner == c.Origin || hasClaim(c.Claimants, ev.Winner)
	  case "fresh_user":
		   // 已经在案里的用户再次出现不推翻已作出的裁决。
		   return ev.User != "" && ev.User != c.Origin && !hasClaim(c.Claimants, ev.User)
	   default:
		  return false
	  }
  }

   func actorOK(actor string) bool {
	  actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		  return false
	  }
	   return true
  }

  func hasClaim(xs []string, u string) bool {
	   for _, x := range xs {
		  if x == u {
			   return true
		  }
	   }
	  return false
  }

// Replay 从 clear 重放日志。终态或胜诉人不一致则失败。

  func Replay(c Case) (Case, error) {
	   sim := Case{
		   ID: c.ID, Chain: c.Chain, Namespace: c.Namespace, Fold: c.Fold,
		   Origin: c.Origin, Claimants: []string{}, State: StClear, Mode: c.Mode,
	  }
	   if c.Origin != "" {
		   sim.Claimants = []string{c.Origin}
	  }
	   for _, ev := range c.Log {
		   next, err := Transit(sim, ev, ev.At)
		   if err != nil {
			   return Case{}, err
		  }
		   sim = next
	  }
	  if sim.State != c.State || sim.Winner != c.Winner {
		   return Case{}, fmt.Errorf("collide replay diverged")
	  }
	  return sim, nil
  }
