// @FileName: service.go
// @Summary: brokerbook/service.go
// @Author: Marston
// @Created: 2021.03.08

package brokerbook

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu      sync.Mutex
	masters map[string]*Master
	subs map[string]*Sub
	books map[string]*Book
	allocs map[string]*Alloc
	fills  []Fill
	byIdem map[string]string
	clock  func() time.Time
	fenceGuard FenceGuard // 可选，由 cmd 层在启动时注入
}

  func New() *Service {
	  return &Service{
		   masters: map[string]*Master{}, subs: map[string]*Sub{},
		   books: map[string]*Book{}, allocs: map[string]*Alloc{},
		   byIdem: map[string]string{},
		  clock:  func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
   }

  func bookKey(owner, asset string) string {
	  return owner + "|" + asset
  }

   func (s *Service) OpenMaster(req OpenMasterReq) (Master, error) {
	   if s == nil {
		  return Master{}, ErrUnavailable
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		   return Master{}, ErrUser
	   }
	   label := strings.TrimSpace(req.Label)
	   if label == "" || len(label) > 64 {
		  return Master{}, ErrLabel
	  }
	  now := s.now()
	   row := &Master{ID: uuid.NewString(), UserID: uid, Label: label, OpenedAt: now, UpdatedAt: now}
	   s.mu.Lock()
	   s.masters[row.ID] = row
	  s.mu.Unlock()
	  return row.Copy(), nil
  }

  func (s *Service) OpenSub(req OpenSubReq) (Sub, error) {
	  if s == nil {
		   return Sub{}, ErrUnavailable
	  }
	   if _, err := s.GetMaster(req.MasterID); err != nil {
		  return Sub{}, err
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		  return Sub{}, ErrUser
	  }
	   label := strings.TrimSpace(req.Label)
	  if label == "" || len(label) > 64 {
		   return Sub{}, ErrLabel
	  }
	   privs := make([]Priv, 0, len(req.Privileges))
	  seen := map[Priv]struct{}{}
	   for _, p := range req.Privileges {
		   got, err := ParsePriv(string(p))
		  if err != nil {
			   return Sub{}, err
		   }
		  if _, ok := seen[got]; ok {
			   continue
		   }
		   seen[got] = struct{}{}
		  privs = append(privs, got)
	  }
	  now := s.now()
	  row := &Sub{
		   ID:       uuid.NewString(),
		  MasterID: strings.TrimSpace(req.MasterID),
		  UserID:   uid, Label: label, Privileges: privs, OpenedAt: now, UpdatedAt: now,
	   }
	  s.mu.Lock()
	  s.subs[row.ID] = row
	   s.mu.Unlock()
	   return row.Copy(), nil
  }

  func (s *Service) GetMaster(id string) (Master, error) {
	  if s == nil {
		   return Master{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.masters[strings.TrimSpace(id)]
	   if !ok {
		  return Master{}, ErrMaster
	   }
	   return cur.Copy(), nil
   }

  func (s *Service) GetSub(id string) (Sub, error) {
	   if s == nil {
		  return Sub{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.subs[strings.TrimSpace(id)]
	  if !ok {
		   return Sub{}, ErrSub
	  }
	  return cur.Copy(), nil
  }

   func (s *Service) ListSubs(masterID string) []Sub {
	   if s == nil {
		  return nil
	  }
	  masterID = strings.TrimSpace(masterID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Sub, 0)
	  for _, r := range s.subs {
		   if masterID != "" && r.MasterID != masterID {
			  continue
		  }
		  out = append(out, r.Copy())
	   }
	   return out
   }

  func (s *Service) BookOf(owner, asset string) (Book, error) {
	  if s == nil {
		   return Book{}, ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return Book{}, ErrAsset
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur := s.books[bookKey(strings.TrimSpace(owner), as)]
	   if cur == nil {
		   return Book{OwnerID: strings.TrimSpace(owner), Asset: as, Available: "0"}, nil
	   }
	  return cur.Copy(), nil
   }

  func (s *Service) CreditMaster(id, asset, amount string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   if _, err := s.GetMaster(id); err != nil {
		   return err
	   }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return ErrAsset
	  }
	  amt, err := money.Parse(amount)
	   if err != nil {
		   return ErrAmount
	  }
	  // 旁路挂钩：若 master 下有围栏态子账，检测并挂账，拒绝写入。
	  if err := s.creditMasterBypassCheck(strings.TrimSpace(id), as, amt.String()); err != nil {
		  return err
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.addLocked(strings.TrimSpace(id), as, amt)
   }

  func (s *Service) addLocked(owner, asset string, delta money.Amount) error {
	  key := bookKey(owner, asset)
	  cur := s.books[key]
	  if cur == nil {
		  cur = &Book{OwnerID: owner, Asset: asset, Available: "0"}
		   s.books[key] = cur
	  }
	   got, err := money.ParseBook(orZ(cur.Available))
	  if err != nil {
		  return ErrAmount
	  }
	  next, err := got.Add(delta)
	  if err != nil {
		   return err
	  }
	  cur.Available = next.String()
	  return nil
   }

   func (s *Service) subLocked(owner, asset string, delta money.Amount) error {
	   key := bookKey(owner, asset)
	   cur := s.books[key]
	  if cur == nil {
		   return ErrAmount
	   }
	   got,
		   err := money.ParseBook(orZ(cur.Available))
	  if err != nil {
		  return ErrAmount
	   }
	   next, err := got.Sub(delta)
	   if err != nil {
		  return err
	  }
	  cur.Available = next.String()
	   return nil
   }

   func (s *Service) availLocked(owner, asset string) (money.Amount, error) {
	   cur := s.books[bookKey(owner, asset)]
	  if cur == nil {
		  return money.Zero(), nil
	   }
	  return money.ParseBook(orZ(cur.Available))
  }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	  }
	  return s
   }
