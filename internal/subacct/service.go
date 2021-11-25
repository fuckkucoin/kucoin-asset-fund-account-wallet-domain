package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"context"
	"github.com/google/uuid"
	"strings"
	"sync"
	"time"
)

type Service struct {
	quota       *quotaBook
	mu          sync.Mutex
	parents     map[string]*Parent
	children    map[string]*Child
	byParent    map[string][]string
	byUser      map[string]string
	books       map[string]*bookRow
	hops        map[string]*HopRec
	byIdem      map[string]string
	postIdem    map[string]BookBal
	vel         *Velocity
	kv          persist.KV
	sql         *persist.SQLDB
	ledgers     BookReader
	clock       func() time.Time
	mx          *matrixStore
	hydrated    bool
	audits      []FreezeAudit
	cas         CASGuard
	casInFlight int
}

func New() *Service {
	return &Service{
		parents: map[string]*Parent{}, children: map[string]*Child{},
		byParent: map[string][]string{}, byUser: map[string]string{},
		books: map[string]*bookRow{}, hops: map[string]*HopRec{},
		byIdem:   map[string]string{},
		postIdem: map[string]BookBal{}, vel: NewVelocity(),
		kv: persist.NewMemKV(), clock: func() time.Time { return time.Now().UTC() },
	}
}

func NewWithKV(kv persist.KV) *Service {
	s := New()
	if kv != nil {
		s.kv = kv
		if s.vel == nil {
			s.vel = NewVelocity()
		}
		s.vel.BindKV(kv)
	}
	return s
}

func (s *Service) now() time.Time {
	if s == nil || s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock()
}

func (s *Service) KV() persist.KV {
	if s == nil {
		return nil
	}
	return s.kv
}

func (s *Service) Velocity() *Velocity {
	if s == nil {
		return NewVelocity()
	}
	return s.vel
}

func (s *Service) OpenParent(req OpenParentReq) (Parent, error) {
	if s == nil {
		return Parent{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return Parent{}, err
	}
	uid, err := validate.UserID(req.UserID)
	if err != nil {
		return Parent{}, ErrUser
	}
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > 64 {
		return Parent{}, ErrLabel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byUser[uid]; ok {
		if p, ok := s.parents[id]; ok {
			return p.Copy(), nil
		}
		return Parent{}, ErrParent
	}
	now := s.now()
	row := &Parent{
		ID: uuid.NewString(), UserID: uid, Label: label,
		Status: StatusActive, OpenedAt: now, UpdatedAt: now,
	}
	s.parents[row.ID] = row
	s.byUser[uid] = row.ID
	s.byParent[row.ID] = nil
	got, err := s.PersistParent(context.Background(), row.Copy())
	if err != nil {
		delete(s.parents, row.ID)
		delete(s.byUser, uid)
		delete(s.byParent, row.ID)
		return Parent{}, err
	}
	row.Fence = got.Fence
	return row.Copy(), nil
}

func (s *Service) OpenChild(req OpenChildReq) (Child, error) {
	if s == nil {
		return Child{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return Child{}, err
	}
	uid, err := validate.UserID(req.UserID)
	if err != nil {
		return Child{}, ErrUser
	}
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > 64 {
		return Child{}, ErrLabel
	}
	privs, err := normalizePrivs(req.Privileges)
	if err != nil {
		return Child{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p,
		ok := s.parents[strings.TrimSpace(req.ParentID)]
	if !ok {
		return Child{}, ErrParent
	}
	if p.Status == StatusClosed {
		return Child{}, ErrClosed
	}
	if p.Status == StatusFrozen {
		return Child{}, ErrFrozen
	}
	if id, ok := s.byUser[uid]; ok {
		if c, ok := s.children[id]; ok && c.ParentID == p.ID {
			return c.Copy(), nil
		}
		return Child{}, ErrChild
	}
	now := s.now()
	row := &Child{
		ID: uuid.NewString(), ParentID: p.ID, UserID: uid, Label: label,
		Status: StatusActive, Privileges: privs, OpenedAt: now, UpdatedAt: now,
	}
	s.children[row.ID] = row
	s.byUser[uid] = row.ID
	s.byParent[p.ID] = append(s.byParent[p.ID], row.ID)
	got, err := s.PersistChild(context.Background(), row.Copy())
	if err != nil {
		delete(s.children, row.ID)
		delete(s.byUser, uid)
		kids := s.byParent[p.ID]
		if n := len(kids); n > 0 && kids[n-1] == row.ID {
			s.byParent[p.ID] = kids[:n-1]
		}
		return Child{}, err
	}
	row.Fence = got.Fence
	return row.Copy(), nil
}

func normalizePrivs(in []Privilege) ([]Privilege, error) {
	if len(in) == 0 {
		return AllPrivileges(), nil
	}
	seen := map[Privilege]struct{}{}
	out := make([]Privilege,
		0, len(in))
	for _, p := range in {
		got, err := ParsePrivilege(string(p))
		if err != nil {
			return nil, err
		}
		if _, ok := seen[got]; ok {
			continue
		}
		seen[got] = struct{}{}
		out = append(out, got)
	}
	return out, nil
}

func (s *Service) GetParent(id string) (Parent, error) {
	if s == nil {
		return Parent{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return Parent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.parents[strings.TrimSpace(id)]
	if !ok {
		return Parent{}, ErrParent
	}
	return p.Copy(), nil
}

func (s *Service) GetChild(id string) (Child, error) {
	if s == nil {
		return Child{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return Child{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.children[strings.TrimSpace(id)]
	if !ok {
		return Child{}, ErrChild
	}
	return c.Copy(), nil
}

func (s *Service) OwnerOf(userID string) (kind, id string, err error) {
	if s == nil {
		return "", "", ErrUnavailable
	}
	uid, err := validate.UserID(userID)
	if err != nil {
		return "", "", ErrUser
	}
	if err := s.ensureHydrated(); err != nil {
		return "", "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byUser[uid]
	if !ok {
		return "", "", ErrUser
	}
	if _, ok := s.parents[id]; ok {
		return "parent", id, nil
	}
	if _, ok := s.children[id]; ok {
		return "child", id, nil
	}
	return "", "", ErrUser
}

func (s *Service) ListChildren(parentID string) []Child {
	if s == nil {
		return nil
	}
	_ = s.ensureHydrated()
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.byParent[strings.TrimSpace(parentID)]
	out := make([]Child, 0, len(ids))
	for _, id := range ids {
		if c, ok := s.children[id]; ok {
			out = append(out, c.Copy())
		}
	}
	return out
}

func (s *Service) Snapshot(parentID string) (Snapshot, error) {
	if err := s.ensureHydrated(); err != nil {
		return Snapshot{}, err
	}
	p, err := s.GetParent(parentID)
	if err != nil {
		return Snapshot{}, err
	}
	kids := s.ListChildren(parentID)
	s.mu.Lock()
	books := s.listBooksLocked(parentID)
	s.mu.Unlock()
	return Snapshot{
		Parent: p, Children: kids, Books: books,
		Host: "account-book.kucoin.net", Region: persist.RegionSG,
	}, nil
}
