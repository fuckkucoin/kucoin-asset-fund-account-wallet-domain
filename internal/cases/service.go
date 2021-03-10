// 功能：cases/service.go
// 作者：Blaise
// 日期：2021.03.08

package cases

import (
	"fmt"
	"strings"
	"time"
)

type Service struct {
	st *Store
}

   func New(st *Store) *Service {
	   if st == nil {
		   st = NewStore()
	   }
	   return &Service{st: st}
  }

  func (s *Service) OpenOrGet(movementID, userID, kind, reason string, policyVer int) (*Case, error) {
	  c,
		   err := s.st.Open(movementID, userID, kind, reason, policyVer)
	   if err == ErrExists {
		  return c, nil
	  }
	   return c, err
  }

   func (s *Service) Assign(id, actor string) (*Case, error) {
	  actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		   return nil, ErrMakerCheck
	  }
	   return s.st.Transition(id, actor, EventAssign)
   }

// Decide 结案。actor 不能为空 / system / 案件 UserID。decision 只认 allow|approve|deny。
// 已关闭或过期的案拒绝再转，避免同一笔出金被两次放行。
  func (s *Service) Decide(id, actor, decision string) (*Case, error) {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return nil, ErrMakerCheck
	  }
	   cur, err := s.st.Get(id)
	   if err != nil {
		   return nil, err
	   }
	  if strings.EqualFold(actor, cur.UserID) {
		   return nil, fmt.Errorf("%w: actor cannot be the user", ErrMakerCheck)
	  }
	   ev := EventApprove
	   if strings.ToLower(strings.TrimSpace(decision)) == "deny" {
		   ev = EventDeny
	   } else if strings.ToLower(strings.TrimSpace(decision)) != "allow" && strings.ToLower(strings.TrimSpace(decision)) != "approve" {
		  return nil, fmt.Errorf("%w: decision", ErrIllegal)
	   }
	  return s.st.Transition(id, actor, ev)
   }

// DecideFenced 带围栏结案。epoch/token 对不上返回 ErrStaleFence，不得当第二次放行。

   func (s *Service) DecideFenced(id, actor, decision string, epoch int64, token string) (*Case, error) {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return nil, ErrMakerCheck
	  }
	  cur, err := s.st.Get(id)
	  if err != nil {
		  return nil, err
	  }
	   if strings.EqualFold(actor, cur.UserID) {
		   return nil, fmt.Errorf("%w: actor cannot be the user", ErrMakerCheck)
	  }
	  ev := EventApprove
	  if strings.ToLower(strings.TrimSpace(decision)) == "deny" {
		  ev = EventDeny
	   } else if strings.ToLower(strings.TrimSpace(decision)) != "allow" && strings.ToLower(strings.TrimSpace(decision)) != "approve" {
		  return nil, fmt.Errorf("%w: decision", ErrIllegal)
	   }
	  return s.st.TransitionFenced(id, actor, ev, epoch, token)
   }

   func (s *Service) Get(id string) (*Case, error) { return s.st.Get(id) }

  func (s *Service) ByMovement(id string) (*Case, error) { return s.st.ByMovement(id) }

   func (s *Service) Queue(limit int) []Case { return s.st.ListOpen(limit) }

  func (s *Service) Expire(age time.Duration) []Case { return s.st.ExpireOlderThan(age) }

   func (s *Service) Audit(caseID string,
	  limit int) []AuditEvent {
	  return s.st.Audit(caseID, limit)
   }

  func (s *Service) Store() *Store { return s.st }
