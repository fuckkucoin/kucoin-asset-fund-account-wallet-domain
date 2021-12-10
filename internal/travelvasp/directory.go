package travelvasp

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu    sync.Mutex
	vasps map[string]*VASP
	byDID map[string]string
	byLEI map[string]string
	msgs   map[string]*Message
	byIdem map[string]string
	cases  map[string]*Case
	byMove map[string]string
	chainHead string
	chain     []string
	clock     func() time.Time
	sql  *persist.SQLDB
	book *ThresholdBook
	adapters *AdapterHub
	pii    *PIIBox
	outbox *CaseOutbox
	policy   *PolicyBook
	fallback *FallbackBook
	sunrise  *SunriseBook
	health *HealthBook
	retry *RetryBook
	links *LinkBook
	cacheTTL time.Duration
	cacheAt time.Time
	cacheList []VASP
}

  func New() *Service {
	  s := &Service{
		   vasps: map[string]*VASP{}, byDID: map[string]string{}, byLEI: map[string]string{},
		  msgs: map[string]*Message{}, byIdem: map[string]string{},
		  cases: map[string]*Case{}, byMove: map[string]string{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	   _ = s.PutVASP(VASP{Code: OriginCode(), Name: "origin-desk", Country: "XX", Protocol: ProtoLOCAL, Enabled: true})
	  _ = s.PutVASP(VASP{Code: PeerCode(), Name: "peer-desk", Country: "XX", Protocol: ProtoLOCAL, Enabled: true})
	   _ = s.PutVASP(VASP{Code: CatalogUnhosted, Name: "unhosted-wallet", Country: "XX", Protocol: ProtoUNHOSTED, Unhosted: true, Enabled: true})
	  return s
  }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
  }

  func (s *Service) PutVASP(v VASP) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  code := strings.ToUpper(strings.TrimSpace(v.Code))
	  if len(code) < 3 || len(code) > 16 {
		   return ErrVASP
	  }
	  for _, r := range code {
		  if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			  return ErrVASP
		   }
	  }
	  if strings.TrimSpace(v.Name) == "" || len(v.Name) > 64 {
		   return ErrVASP
	  }
	  if secretInEndpoint(v.Endpoint) {
		  return ErrSecret
	  }
	  proto, err := normalizeProtocol(v.Protocol, v.Unhosted)
	   if err != nil {
		  return err
	   }
	   v.Code = code
	   v.Name = strings.TrimSpace(v.Name)
	  v.Country = strings.ToUpper(strings.TrimSpace(v.Country))
	   v.Endpoint = strings.TrimSpace(v.Endpoint)
	   v.DID = strings.TrimSpace(v.DID)
	  v.LEI = strings.ToUpper(strings.TrimSpace(v.LEI))
	   v.Protocol = proto
	   if proto == ProtoUNHOSTED {
		   v.Unhosted = true
	  }
	  v.UpdatedAt = s.now()
	  s.mu.Lock()
	   if old, ok := s.vasps[code]; ok {
		  s.dropIndexLocked(old)
	   }
	   cp := v
	  s.vasps[code] = &cp
	  s.indexLocked(&cp)
	  s.mu.Unlock()
	  s.persistVASP(cp)
	  s.InvalidateVASPCache()
	   return nil
  }

  func (s *Service) GetVASP(code string) (VASP, error) {
	   if s == nil {
		  return VASP{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.vasps[strings.ToUpper(strings.TrimSpace(code))]
	   if !ok {
		   return VASP{}, ErrVASP
	  }
	  return cur.Copy(), nil
   }

   func (s *Service) RequireEnabled(code string) (VASP, error) {
	  v, err := s.GetVASP(code)
	  if err != nil {
		   return VASP{}, err
	  }
	   if !v.Enabled {
		  return v, ErrDisabled
	  }
	  return v, nil
  }

  func (s *Service) ListVASP() []VASP {
	  if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]VASP, 0, len(s.vasps))
	  for _, v := range s.vasps {
		   out = append(out, v.Copy())
	   }
	   return out
  }

   func (s *Service) SetEnabled(code,
	  actor string, on bool) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	   }
	   s.mu.Lock()
	  cur, ok := s.vasps[strings.ToUpper(strings.TrimSpace(code))]
	   if !ok {
		  s.mu.Unlock()
		   return ErrVASP
	  }
	   cur.Enabled = on
	  cur.UpdatedAt = s.now()
	   s.mu.Unlock()
	  s.InvalidateVASPCache()
	  return nil
  }
