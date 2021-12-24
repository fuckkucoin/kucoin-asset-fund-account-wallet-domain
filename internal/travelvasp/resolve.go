package travelvasp

import (
	"strings"
)

type ResolveQuery struct {
	Code string `json:"code,omitempty"`
	DID string `json:"did,omitempty"`
	LEI     string `json:"lei,omitempty"`
	Name    string `json:"name,omitempty"`
	Country string `json:"country,omitempty"`
	Type string `json:"type,omitempty"`
}

  func (s *Service) indexLocked(v *VASP) {
	  if s == nil || v == nil {
		   return
	  }
	  if s.byDID == nil {
		  s.byDID = map[string]string{}
	   }
	   if s.byLEI == nil {
		   s.byLEI = map[string]string{}
	   }
	   if v.DID != "" {
		   s.byDID[v.DID] = v.Code
	   }
	   if v.LEI != "" {
		   s.byLEI[v.LEI] = v.Code
	   }
   }

  func (s *Service) dropIndexLocked(v *VASP) {
	  if s == nil || v == nil {
		  return
	  }
	   if v.DID != "" && s.byDID[v.DID] == v.Code {
		   delete(s.byDID, v.DID)
	   }
	   if v.LEI != "" && s.byLEI[v.LEI] == v.Code {
		   delete(s.byLEI, v.LEI)
	  }
   }

   func normalizeProtocol(p string, unhosted bool) (string, error) {
	  p = strings.ToUpper(strings.TrimSpace(p))
	  if p == "" {
		  if unhosted {
			  return ProtoUNHOSTED, nil
		  }
		  return ProtoLOCAL, nil
	  }
	   switch p {
	   case ProtoLOCAL, ProtoTRUST, ProtoSYGNA, ProtoNOTABENE, ProtoCODE, ProtoUNHOSTED:
		  return p, nil
	   default:
		  return "", ErrVASP
	  }
   }

   func secretInEndpoint(raw string) bool {
	  s := strings.ToLower(strings.TrimSpace(raw))
	   if s == "" {
		   return false
	  }
	  for _, k := range []string{"api_key=", "apikey=", "api-key=", "secret=", "token=", "password="} {
		   if strings.Contains(s, k) {
			  return true
		  }
	  }
	   return false
  }

// Resolve 解析对手 VASP。须精确命中目录；未命中失败即关。

  func (s *Service) Resolve(q ResolveQuery) (VASP, error) {
	  if s == nil {
		   return VASP{}, ErrUnavailable
	   }
	  typ := strings.ToLower(strings.TrimSpace(q.Type))
	  if typ == "unhosted" || typ == "private" || typ == "0" {
		  return s.RequireEnabled(CatalogUnhosted)
	   }
	   if code := strings.ToUpper(strings.TrimSpace(q.Code)); code != "" {
		  return s.GetVASP(code)
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if did := strings.TrimSpace(q.DID); did != "" {
		  if code, ok := s.byDID[did]; ok {
			   return s.vasps[code].Copy(), nil
		  }
		  return VASP{}, ErrPeer
	  }
	   if lei := strings.ToUpper(strings.TrimSpace(q.LEI)); lei != "" {
		   if code, ok := s.byLEI[lei]; ok {
			  return s.vasps[code].Copy(), nil
		   }
		  return VASP{}, ErrPeer
	   }
	  name := strings.TrimSpace(q.Name)
	   country := strings.ToUpper(strings.TrimSpace(q.Country))
	  if name != "" && country != "" {
		  for _, v := range s.vasps {
			  if v != nil && strings.EqualFold(v.Name, name) && v.Country == country {
				  return v.Copy(), nil
			  }
		  }
		   return VASP{}, ErrPeer
	  }
	  return VASP{}, ErrPeer
  }

// ResolveOrPeer 有查询则精确解析，否则回落配置对端码。回落码也须在目录且启用。
   func (s *Service) ResolveOrPeer(q ResolveQuery, fallback string) (VASP, error) {
	  if strings.TrimSpace(q.Code+q.DID+q.LEI+q.Name+q.Type) != "" {
		  v, err := s.Resolve(q)
		  if err != nil {
			   return VASP{}, err
		   }
		  if !v.Enabled {
			   return v, ErrDisabled
		  }
		  return v, nil
	   }
	  code := strings.ToUpper(strings.TrimSpace(fallback))
	  if code == "" {
		   code = PeerCode()
	   }
	  return s.RequireEnabled(code)
  }
