package regioinv

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// MemStore 是内存驱动，用于和单机部署。应用 SQLStore。
type MemStore struct {
	mu sync.Mutex
	regions map[string]Region    // id→Region
	byCode  map[string]string    // code→id
	invs    map[string]RegionInv // invKey→RegionInv
	marks   map[string]Watermark // markKey→Watermark
	policies map[string]IsoPolicy // regionID→IsoPolicy
	obss map[string]Observation // obsKey→Observation
	clock func() time.Time
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  regions: map[string]Region{},
		   byCode:  map[string]string{},
		  invs: map[string]RegionInv{},
		   marks:    map[string]Watermark{},
		   policies: map[string]IsoPolicy{},
		   obss:     map[string]Observation{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (m *MemStore) SetClock(fn func() time.Time) {
	  if fn != nil {
		  m.mu.Lock()
		   m.clock = fn
		  m.mu.Unlock()
	   }
  }

  func (m *MemStore) now() time.Time {
	  m.mu.Lock()
	  fn := m.clock
	  m.mu.Unlock()
	   return fn()
   }

   func invKey(regionID, tier, asset, chain string) string {
	  return regionID + "|" + tier + "|" + strings.ToUpper(asset) + "|" + strings.ToLower(chain)
  }

  func markKey(regionID, tier, asset, chain string) string {
	   return regionID + "|" + tier + "|" + strings.ToUpper(asset) + "|" + strings.ToLower(chain)
   }

   func obsKey(regionID, tier, asset, chain, source string) string {
	   return regionID + "|" + tier + "|" + strings.ToUpper(asset) + "|" + strings.ToLower(chain) + "|" + source
   }

  func (m *MemStore) SaveRegion(_ context.Context, r Region) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if r.ID == "" {
		   return ErrInvalid
	  }
	  m.regions[r.ID] = r
	   m.byCode[r.Code] = r.ID
	  return nil
   }

  func (m *MemStore) GetRegion(_ context.Context, id string) (Region, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  r, ok := m.regions[id]
	   if !ok {
		   return Region{}, ErrNotFound
	  }
	   return r.clone(), nil
   }

  func (m *MemStore) FindRegion(_ context.Context, code string) (Region, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.byCode[code]
	   if !ok {
		   return Region{}, ErrNotFound
	  }
	   r, ok := m.regions[id]
	   if !ok {
		   return Region{}, ErrNotFound
	   }
	  return r.clone(), nil
   }

  func (m *MemStore) ListRegions(_ context.Context) ([]Region, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Region, 0, len(m.regions))
	   for _, r := range m.regions {
		  out = append(out, r.clone())
	   }
	  return out, nil
  }

   func (m *MemStore) SaveInv(_ context.Context, row RegionInv) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.invs[invKey(row.RegionID, row.Tier, row.Asset, row.Chain)] = row.clone()
	   return nil
   }

   func (m *MemStore) FindInv(_ context.Context, regionID, tier, asset, chain string) (RegionInv, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   k := invKey(regionID, tier, asset, chain)
	   v, ok := m.invs[k]
	   if !ok {
		  return RegionInv{}, ErrNotFound
	   }
	  return v.clone(), nil
  }

  func (m *MemStore) ListInv(_ context.Context, regionID string) ([]RegionInv, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []RegionInv
	  for _, v := range m.invs {
		   if v.RegionID == regionID {
			  out = append(out, v.clone())
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) Transfer(_ context.Context, t Transfer) (RegionInv, RegionInv, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   sk := invKey(t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	  dk := invKey(t.DstRegionID, t.Tier, t.Asset, t.Chain)
	   src, ok := m.invs[sk]
	  if !ok {
		  return RegionInv{}, RegionInv{}, ErrNotFound
	   }
	   dst, ok := m.invs[dk]
	  if !ok {
		   dst = RegionInv{
			   ID: newID("ri"),
			   RegionID: t.DstRegionID,
			  Tier:  t.Tier,
			   Asset: strings.ToUpper(t.Asset),
			   Chain: strings.ToLower(t.Chain),
			   Available: "0",
			   Frozen:    "0",
			   Observed: "0",
		   }
	   }
	  now := m.clock()
	  ns, nd, err := NextPair(src, dst, t.Amount, now)
	  if err != nil {
		   return RegionInv{}, RegionInv{}, err
	  }
	   m.invs[sk] = ns
	   m.invs[dk] = nd
	   return ns.clone(), nd.clone(), nil
  }

   func (m *MemStore) SaveMark(_ context.Context, w Watermark) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.marks[markKey(w.RegionID, w.Tier, w.Asset, w.Chain)] = w.clone()
	   return nil
  }

  func (m *MemStore) FindMark(_ context.Context, regionID, tier, asset, chain string) (Watermark, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   k := markKey(regionID, tier, asset, chain)
	   v, ok := m.marks[k]
	   if !ok {
		   return Watermark{}, ErrNotFound
	  }
	  return v.clone(), nil
   }

   func (m *MemStore) ListMarks(_ context.Context, regionID string) ([]Watermark, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []Watermark
	   for _, v := range m.marks {
		  if v.RegionID == regionID {
			  out = append(out, v.clone())
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) AddUsed(_ context.Context, regionID, tier, asset, chain, delta, day string, add bool) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   k := markKey(regionID, tier, asset, chain)
	  wm, ok := m.marks[k]
	   if !ok {
		   return ErrNotFound
	  }
	   if wm.UsedDay != day {
		  wm.Used = "0"
		   wm.UsedDay = day
	  }
	   cur, err := bookOf(wm.Used)
	   if err != nil {
		  return err
	   }
	   d, err := money.Parse(delta)
	  if err != nil {
		  return err
	   }
	  if add {
		  n, err := cur.Add(d)
		  if err != nil {
			   return err
		  }
		  wm.Used = n.String()
	  } else {
		  n, err := cur.Sub(d)
		   if err != nil {
			  return err
		   }
		  wm.Used = n.String()
	  }
	  m.marks[k] = wm
	   return nil
  }

   func (m *MemStore) SavePolicy(_ context.Context, p IsoPolicy) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.policies[p.RegionID] = p.clone()
	   return nil
   }

  func (m *MemStore) FindPolicy(_ context.Context, regionID string) (IsoPolicy, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   v, ok := m.policies[regionID]
	   if !ok {
		  return IsoPolicy{}, ErrNotFound
	  }
	  return v.clone(), nil
  }

  func (m *MemStore) SaveObs(_ context.Context, o Observation) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.obss[obsKey(o.RegionID, o.Tier, o.Asset, o.Chain, o.Source)] = o.clone()
	   return nil
   }

   func (m *MemStore) ListObs(_ context.Context, regionID, tier, asset, chain string) ([]Observation, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []Observation
	   for _, v := range m.obss {
		  if v.RegionID == regionID && v.Tier == tier &&
			  strings.ToUpper(v.Asset) == strings.ToUpper(asset) &&
			   strings.ToLower(v.Chain) == strings.ToLower(chain) {
			  out = append(out, v.clone())
		   }
	   }
	   return out, nil
   }
