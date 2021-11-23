package stablebr

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Attest
// 勾选并校验一条稳定币桥接证明。
// 按 kind/asset 匹配已知证明条目。
// 过期或未知条目必须失败关闭，不得依赖缺省放行。
   func (s *Service) Attest(req AttestReq) (Check, error) {
	   if s == nil {
		  return Check{}, ErrUnavailable
	  }
	   kind, err := ParseKind(req.Kind)
	  if err != nil {
		  return Check{}, err
	   }
	  as, err := allowedAsset(req.Asset)
	  if err != nil {
		   return Check{}, err
	   }
	  item, err := ParseItem(req.Item)
	  if err != nil {
		   return Check{}, err
	  }
	   need := required(kind)
	   ok := false
	   for _, n := range need {
		  if n == item {
			   ok = true
			  break
		   }
	   }
	  if !ok {
		  return Check{}, ErrItem
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		   return Check{}, ErrActor
	   }
	  ttl := s.ttl
	   if req.TTLSec > 0 {
		  ttl = time.Duration(req.TTLSec) * time.Second
	   }
	   now := s.now()
	  row := Check{
		  Item: item, Actor: actor, Note: strings.TrimSpace(req.Note),
		   At: now, Expiry: now.Add(ttl),
	  }
	   s.mu.Lock()
	  s.checks[checkKey(kind, as, item)] = row
	   s.mu.Unlock()
	  return row.Copy(), nil
  }

  func (s *Service) ClearAttest(kindS, asset, actor string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   kind, err := ParseKind(kindS)
	  if err != nil {
		  return err
	  }
	  as, err := allowedAsset(asset)
	   if err != nil {
		   return err
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	   }
	   s.mu.Lock()
	   for _, item := range required(kind) {
		   delete(s.checks, checkKey(kind, as, item))
	   }
	  key := genKey(kind, as)
	   s.mintGen[key] = s.mintGen[key] + 1
	   s.mu.Unlock()
	   s.fireClearHooks(kind, as, actor)
	  return nil
   }

  func (s *Service) Checklist(kindS, asset string) ([]Check, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  kind, err := ParseKind(kindS)
	   if err != nil {
		   return nil, err
	   }
	  as, err := allowedAsset(asset)
	   if err != nil {
		   return nil, err
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Check, 0, 4)
	  for _, item := range required(kind) {
		  if c, ok := s.checks[checkKey(kind, as, item)]; ok {
			   out = append(out, c.Copy())
		  }
	   }
	  return out, nil
  }

   func (s *Service) gateLocked(kind Kind, asset string) error {
	  now := s.now()
	   for _, item := range required(kind) {
		  c, ok := s.checks[checkKey(kind, asset, item)]
		   if !ok {
			   return ErrAttest
		   }
		   if !c.Expiry.IsZero() && !c.Expiry.After(now) {
			   return ErrExpired
		   }
	  }
	   return nil
  }
