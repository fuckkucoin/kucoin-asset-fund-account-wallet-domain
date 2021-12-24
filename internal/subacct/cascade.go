package subacct

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// FreezeParent 冻母户并级联冻全部子户。已关户不得再冻。

   func (s *Service) FreezeParent(parentID, reason string) (Parent, error) {
	  if s == nil {
		   return Parent{},
			  ErrUnavailable
	  }
	   reason = strings.TrimSpace(reason)
	   if reason == "" {
		   reason = "freeze"
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.parents[strings.TrimSpace(parentID)]
	  if !ok {
		   return Parent{}, ErrParent
	   }
	  if p.Status == StatusClosed {
		  return Parent{}, ErrClosed
	  }
	   now := s.now()
	   prevP := p.Copy()
	   prevKids := map[string]Child{}
	   for _, id := range s.byParent[p.ID] {
		   if c := s.children[id]; c != nil {
			   prevKids[id] = c.Copy()
		   }
	   }
	   p.Status = StatusFrozen
	   p.Reason = reason
	   p.UpdatedAt = now
	   var children []string
	  for _, id := range s.byParent[p.ID] {
		   c := s.children[id]
		  if c == nil || c.Status == StatusClosed {
			   continue
		   }
		  c.Status = StatusFrozen
		   c.Reason = "parent:" + reason
		  c.UpdatedAt = now
		   children = append(children, id)
	  }
	  if err := s.persistParentSQL(p.Copy()); err != nil {
		  *p = prevP
		  for id, prev := range prevKids {
			  if c := s.children[id]; c != nil {
				  *c = prev
			  }
		  }
		  return Parent{}, err
	  }
	  for _, id := range children {
		  c := s.children[id]
		  if c == nil {
			  continue
		  }
		  if err := s.persistChildSQL(c.Copy()); err != nil {
			  *p = prevP
			  for id2, prev := range prevKids {
				  if ch := s.children[id2]; ch != nil {
					  *ch = prev
				  }
			  }
			  return Parent{}, err
		  }
	  }
	  s.recordFreezeLocked(FreezeAudit{
		  Action: "freeze_parent", ParentID: p.ID, Children: children,
		  Detail: reason,
	   })
	   return p.Copy(), nil
  }

// UnfreezeParent 只解母户。子户保持冻结，须单独解冻，避免一次解冻放行整树。

   func (s *Service) UnfreezeParent(parentID string) (Parent, error) {
	  if s == nil {
		  return Parent{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.parents[strings.TrimSpace(parentID)]
	   if !ok {
		   return Parent{}, ErrParent
	   }
	   if p.Status != StatusFrozen {
		   return p.Copy(), nil
	  }
	  prevP := p.Copy()
	  p.Status = StatusActive
	   p.Reason = ""
	   p.UpdatedAt = s.now()
	   if err := s.persistParentSQL(p.Copy()); err != nil {
		   *p = prevP
		   return Parent{}, err
	   }
	   var kept []string
	  for _, id := range s.byParent[p.ID] {
		  c := s.children[id]
		  if c != nil && c.Status == StatusFrozen {
			  kept = append(kept, id)
		   }
	   }
	   s.recordFreezeLocked(FreezeAudit{
		   Action: "unfreeze_parent", ParentID: p.ID, Children: kept,
		  Detail: "children remain frozen",
	   })
	   return p.Copy(), nil
  }

  func (s *Service) FreezeChild(childID, reason string) (Child, error) {
	   if s == nil {
		   return Child{}, ErrUnavailable
	  }
	   reason = strings.TrimSpace(reason)
	  if reason == "" {
		  reason = "freeze"
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   c, ok := s.children[strings.TrimSpace(childID)]
	  if !ok {
		  return Child{}, ErrChild
	   }
	  if c.Status == StatusClosed {
		   return Child{}, ErrClosed
	  }
	   prev := c.Copy()
	   c.Status = StatusFrozen
	   c.Reason = reason
	  c.UpdatedAt = s.now()
	   if err := s.persistChildSQL(c.Copy()); err != nil {
		   *c = prev
		   return Child{}, err
	   }
	   s.recordFreezeLocked(FreezeAudit{
		  Action: "freeze_child", ParentID: c.ParentID, ChildID: c.ID, Detail: reason,
	  })
	  return c.Copy(), nil
  }

// UnfreezeChild 母户仍冻则拒绝。级联冻未解开时子户不得单独复活。

  func (s *Service) UnfreezeChild(childID string) (Child, error) {
	  if s == nil {
		   return Child{}, ErrUnavailable
	   }
	  if err := s.guardUnfreezeChild(strings.TrimSpace(childID)); err != nil {
		   return Child{}, err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  c, ok := s.children[strings.TrimSpace(childID)]
	   if !ok {
		  return Child{}, ErrChild
	  }
	  p := s.parents[c.ParentID]
	   if p == nil {
		  return Child{}, ErrParent
	  }
	   if p.Status == StatusFrozen {
		  s.recordFreezeLocked(FreezeAudit{
			   Action: "unfreeze_rejected", ParentID: c.ParentID, ChildID: c.ID,
			   Detail: "child stays frozen while parent is frozen",
		   })
		   return c.Copy(), ErrFrozen
	   }
	   if c.Status != StatusFrozen {
		   return c.Copy(), nil
	  }
	   prev := c.Copy()
	   c.Status = StatusActive
	   c.Reason = ""
	   c.UpdatedAt = s.now()
	   if err := s.persistChildSQL(c.Copy()); err != nil {
		   *c = prev
		   return Child{}, err
	   }
	   s.recordFreezeLocked(FreezeAudit{
		   Action: "unfreeze_child", ParentID: c.ParentID, ChildID: c.ID,
		  Detail: "parent active",
	   })
	  return c.Copy(), nil
  }

  func (s *Service) SetPrivileges(childID string, privs []Privilege) (Child, error) {
	   if s == nil {
		  return Child{}, ErrUnavailable
	   }
	   got, err := normalizePrivs(privs)
	  if err != nil {
		   return Child{}, err
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   c, ok := s.children[strings.TrimSpace(childID)]
	  if !ok {
		  return Child{}, ErrChild
	   }
	   if c.Status == StatusClosed {
		  return Child{}, ErrClosed
	   }
	   parentFrozen := false
	  if p := s.parents[c.ParentID]; p != nil && p.Status == StatusFrozen {
		  parentFrozen = true
	   }
	  if c.Status == StatusFrozen || parentFrozen {
		   if !privIsSubset(got, c.Privileges) {
			   s.recordFreezeLocked(FreezeAudit{
				   Action: "privilege_widen_rejected", ParentID: c.ParentID, ChildID: c.ID,
				   Detail: "frozen account cannot gain privileges",
			   })
			  return c.Copy(), ErrFrozen
		  }
	  }
	  prev := c.Copy()
	  c.Privileges = got
	  c.UpdatedAt = s.now()
	  if err := s.persistChildSQL(c.Copy()); err != nil {
		  *c = prev
		  return Child{}, err
	  }
	  return c.Copy(), nil
   }

  func (s *Service) SetLabel(childID, label string) (Child, error) {
	if s == nil {
		return Child{}, ErrUnavailable
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return Child{}, ErrChild
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.children[strings.TrimSpace(childID)]
	if !ok {
		return Child{}, ErrChild
	}
	if c.Status == StatusClosed {
		return Child{}, ErrClosed
	}
	prev := c.Copy()
	c.Label = label
	c.UpdatedAt = s.now()
	if err := s.persistChildSQL(c.Copy()); err != nil {
		*c = prev
		return Child{}, err
	}
	return c.Copy(), nil
}

func (s *Service) CloseChild(childID string) (Child, error) {
	  if s == nil {
		   return Child{}, ErrUnavailable
	   }
	  childID = strings.TrimSpace(childID)
	  s.mu.Lock()
	  c0, ok := s.children[childID]
	  uid := ""
	  if ok && c0 != nil {
		  uid = c0.UserID
	  }
	  s.mu.Unlock()
	  if !ok {
		  return Child{}, ErrChild
	  }
	  if err := s.requireLedgerZero(uid); err != nil {
		  return Child{}, err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  c, ok := s.children[childID]
	   if !ok {
		   return Child{}, ErrChild
	  }
	   if err := s.requireZeroLocked(c.ID); err != nil {
		  return Child{}, err
	  }
	   prev := c.Copy()
	   c.Status = StatusClosed
	   c.Reason = "close"
	   c.UpdatedAt = s.now()
	   if err := s.persistChildSQL(c.Copy()); err != nil {
		   *c = prev
		   return Child{}, err
	   }
	   return c.Copy(), nil
  }

  func (s *Service) parentOfLocked(ownerID string) (*Parent, *Child, error) {
	   if p, ok := s.parents[ownerID]; ok {
		  return p, nil, nil
	  }
	  if c, ok := s.children[ownerID]; ok {
		   p := s.parents[c.ParentID]
		   if p == nil {
			  return nil, nil, ErrParent
		   }
		   return p, c, nil
	   }
	   return nil, nil, ErrUser
   }

  func (s *Service) requireActiveLocked(ownerID string, debit bool) error {
	   p, c, err := s.parentOfLocked(ownerID)
	  if err != nil {
		  return err
	   }
	   if p.Status == StatusClosed {
		  return ErrClosed
	   }
	  if p.Status == StatusFrozen && debit {
		   return ErrFrozen
	   }
	   if c != nil {
		  if c.Status == StatusClosed {
			  return ErrClosed
		  }
		  if c.Status == StatusFrozen && debit {
			  return ErrFrozen
		  }
	  }
	   return nil
   }

   func (s *Service) requirePrivLocked(ownerID string, priv Privilege) error {
	   _,
		   c, err := s.parentOfLocked(ownerID)
	  if err != nil {
		   return err
	   }
	  if c == nil {
		  return nil
	  }
	  if !hasPriv(c.Privileges, priv) {
		  return ErrPrivilege
	   }
	  return nil
   }

  func (s *Service) GateDeposit(userID string) error {
	  kind, id, err := s.OwnerOf(userID)
	   if err != nil {
		   return err
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if err := s.requireActiveLocked(id, false); err != nil {
		  return err
	  }
	  if kind == "child" {
		   return s.requirePrivLocked(id, PrivDeposit)
	   }
	   return nil
   }

   func (s *Service) GateWithdraw(userID string) error {
	   kind, id, err := s.OwnerOf(userID)
	   if err != nil {
		   return err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if err := s.requireActiveLocked(id, true); err != nil {
		   return err
	  }
	  if kind == "child" {
		  return s.requirePrivLocked(id,
			  PrivWithdraw)
	  }
	  return nil
  }

   func (s *Service) GateUser(userID string) error {
	   if _, err := validate.UserID(userID); err != nil {
		  return ErrUser
	   }
	   return nil
  }
