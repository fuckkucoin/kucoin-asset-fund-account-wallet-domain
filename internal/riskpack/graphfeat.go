package riskpack

import (
	"strings"
	"time"
)

// NoteKYT 把筛查结论记到节点上并重算簇分。空结论视为清除特征。
  func (s *Store) NoteKYT(chain, addr, hint string) (DestNode, error) {
	   if s == nil {
		   return DestNode{}, ErrUnavailable
	  }
	   hint = strings.ToLower(strings.TrimSpace(hint))
	  switch hint {
	  case "", "allow", "review", "deny":
	   default:
		  return DestNode{}, ErrDest
	  }
	   if _, err := s.UpsertNode(chain, addr, ""); err != nil {
		   return DestNode{}, err
	  }
	   key, _, _, err := normDest(chain, addr)
	  if err != nil {
		   return DestNode{}, err
	   }
	  now := s.now()
	   s.mu.Lock()
	  n := s.nodes[key]
	   if n == nil {
		   s.mu.Unlock()
		  return DestNode{}, ErrDest
	  }
	  before := n.KYTHint
	  n.KYTHint = hint
	  n.Version++
	   n.LastSeen = now
	  out := *n
	   s.pushAudit(GraphAudit{
		  Kind: AuditKYTFeature, Subject: key, Before: before, After: hint,
		   Detail: "kyt hint is a feature, not a screening decision", At: now,
	  })
	   if err := s.persistNode(out); err != nil {
		   s.mu.Unlock()
		  return DestNode{}, err
	   }
	   s.mu.Unlock()
	  if _, err := s.Recompute(now); err != nil {
		  return DestNode{}, err
	  }
	  got, ok := s.Node(chain, addr)
	  if !ok {
		  return out, nil
	  }
	   return got, nil
  }

// FamilyHint 给同链尾号相近地址补一条弱边。权值低于默认连通阈时不会并簇。

   func (s *Store) FamilyHint(chain, aAddr, bAddr string) (DestEdge, error) {
	   if s == nil {
		  return DestEdge{}, ErrUnavailable
	   }
	  return s.Link(chain, aAddr, chain, bAddr, "0.2", EdgeFamily, "family suffix", 72*time.Hour)
  }
