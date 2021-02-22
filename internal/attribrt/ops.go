package attribrt

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/attribrt/collide"
)

  func (e *Engine) applyCase(id string, ev collide.Event) (collide.Case, error) {
	  if e == nil || e.book == nil {
		  return collide.Case{}, collide.ErrBook()
	  }
	   c, err := e.book.Apply(id, ev)
	  e.saveCollision(c)
	   return c, err
   }

// Assign 把碰撞案派给运营。

   func (e *Engine) Assign(id, actor, reason string) (collide.Case, error) {
	   return e.applyCase(id, collide.Event{Kind: "assign", Actor: actor, Reason: reason, At: e.now()})
   }

// Prove 提交证据摘要，进入取证。

   func (e *Engine) Prove(id, actor, evidence string) (collide.Case, error) {
	   return e.applyCase(id, collide.Event{Kind: "prove", Actor: actor, Evidence: evidence, At: e.now()})
   }

// AcceptProof 证据被接受，进入裁决。

   func (e *Engine) AcceptProof(id, actor string) (collide.Case, error) {
	  return e.applyCase(id, collide.Event{Kind: "accept", Actor: actor, At: e.now()})
  }

// RejectProof 证据无效，退回 blocked。
  func (e *Engine) RejectProof(id, actor, reason string) (collide.Case, error) {
	  return e.applyCase(id, collide.Event{Kind: "reject_proof", Actor: actor, Reason: reason, At: e.now()})
  }

// Pick 指定胜诉用户。理由至少 8 个字符。败诉方仍然不能入账。

  func (e *Engine) Pick(id, actor, winner, reason string) (collide.Case, error) {
	   return e.applyCase(id, collide.Event{Kind: "pick", Actor: actor, Winner: winner, Reason: reason, At: e.now()})
   }

// PickNone 两边都不放行。

  func (e *Engine) PickNone(id, actor, reason string) (collide.Case, error) {
	   return e.applyCase(id, collide.Event{Kind: "pick_none", Actor: actor, Reason: reason, At: e.now()})
  }

// ExpireCases 处理到期案件。

   func (e *Engine) ExpireCases() []collide.Case {
	  if e == nil || e.book == nil {
		  return nil
	  }
	   exp := e.book.Expire()
	  for i := range exp {
		  e.saveCollision(exp[i])
	   }
	   return exp
   }

// Tick 工人一拍：到期碰撞案，以及到点的延迟结论重跑。
// 重跑只更新结论，不产生贷记。
  func (e *Engine) Tick(now time.Time) (expired int, rerun int) {
	  if e == nil {
		  return 0, 0
	  }
	  if now.IsZero() {
		  now = e.now()
	  }
	  exp := e.ExpireCases()
	   due := e.DueDelays(now)
	   for _, s := range due {
		  s.At = now
		  s.Replay = false
		  _, _ = e.Resolve(s)
		  rerun++
	   }
	  return len(exp), rerun
   }

// CloseQueue 把队列项标成 closed。这不是入账。

  func (e *Engine) CloseQueue(id, actor string) bool {
	   if e == nil || strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		   return false
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  for i := range e.queue {
		  if e.queue[i].ID == id && e.queue[i].State == "open" {
			   e.queue[i].State = "closed"
			   e.queue[i].Note = actor
			   return true
		   }
	   }
	  return false
  }
