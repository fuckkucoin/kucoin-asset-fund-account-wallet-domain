package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const (
	kindBook = "subacct.book"
	kindPost = "subacct.post"
)

// hydratePageSize 单页行数。可调小以证明游标翻过截断崖。
var hydratePageSize = 200

type storedBook struct {
	OwnerID   string `json:"owner_id"`
	UserID    string `json:"user_id"`
	Book      string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

type storedPost struct {
	Key string  `json:"key"`
	Bal BookBal `json:"bal"`
}

// Hydrate 从 KV 装回母子树、overlay 账面、过账幂等与 hop。集群进程崩溃后必须先 Hydrate 再 RecoverHop。

func (s *Service) Hydrate(ctx context.Context) error {
	if s == nil {
		return ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hydrated {
		return nil
	}
	if s.kv == nil {
		s.hydrated = true
		return nil
	}
	if err := s.loadParentsLocked(ctx); err != nil {
		return err
	}
	if err := s.loadChildrenLocked(ctx); err != nil {
		return err
	}
	if err := s.loadBooksLocked(ctx); err != nil {
		return err
	}
	if err := s.loadPostsLocked(ctx); err != nil {
		return err
	}
	if err := s.loadHopsLocked(ctx); err != nil {
		return err
	}
	if err := s.loadFreezeAuditLocked(ctx); err != nil {
		return err
	}
	if err := s.HydrateQuotas(ctx); err != nil {
		return err
	}
	if s.vel != nil {
		if s.kv != nil {
			s.vel.BindKV(s.kv)
		}
		if s.sql != nil {
			s.vel.BindSQL(s.sql)
		}
		if err := s.vel.Hydrate(ctx); err != nil {
			return err
		}
	}
	s.hydrated = true
	return nil
}

func (s *Service) ensureHydrated() error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	done := s.hydrated
	s.mu.Unlock()
	if done {
		return nil
	}
	return s.Hydrate(context.Background())
}

func (s *Service) scanKind(ctx context.Context, kind string, fn func(persist.Row) error) error {
	after := ""
	for {
		page,
			err := persist.PageKind(ctx, s.kv, kind, persist.Cursor{After: after, Limit: hydratePageSize})
		if err != nil {
			return err
		}
		for _, row := range page.Rows {
			if err := fn(row); err != nil {
				return err
			}
		}
		if page.Done {
			return nil
		}
		after = page.Next
		if after == "" {
			return nil
		}
	}
}

func (s *Service) loadParentsLocked(ctx context.Context) error {
	return s.scanKind(ctx, kindParent, func(row persist.Row) error {
		var out storedParent
		if err := json.Unmarshal(row.Body, &out); err != nil {
			return err
		}
		p := out.Parent
		p.Fence = row.Fence
		cp := p
		s.parents[p.ID] = &cp
		if p.UserID != "" {
			s.byUser[p.UserID] = p.ID
		}
		if _, ok := s.byParent[p.ID]; !ok {
			s.byParent[p.ID] = nil
		}
		return nil
	})
}

func (s *Service) loadChildrenLocked(ctx context.Context) error {
	return s.scanKind(ctx, kindChild, func(row persist.Row) error {
		var out storedChild
		if err := json.Unmarshal(row.Body, &out); err != nil {
			return err
		}
		c := out.Child
		c.Fence = row.Fence
		cp := c
		s.children[c.ID] = &cp
		if c.UserID != "" {
			s.byUser[c.UserID] = c.ID
		}
		if c.ParentID != "" {
			s.byParent[c.ParentID] = append(s.byParent[c.ParentID], c.ID)
		}
		return nil
	})
}

func (s *Service) loadBooksLocked(ctx context.Context) error {
	return s.scanKind(ctx, kindBook, func(row persist.Row) error {
		var out storedBook
		if err := json.Unmarshal(row.Body, &out); err != nil {
			return err
		}
		av,
			err := money.Parse(out.Available)
		if err != nil {
			av = money.Zero()
		}
		fr, err := money.Parse(out.Frozen)
		if err != nil {
			fr = money.Zero()
		}
		book := strings.TrimSpace(out.Book)
		if book == "" {
			book = "funding"
		}
		k := bookKey(out.OwnerID, book, out.Asset)
		s.books[k] = &bookRow{
			OwnerID: out.OwnerID, UserID: out.UserID, Book: book, Asset: out.Asset,
			Available: av, Frozen: fr, Fence: row.Fence,
		}
		return nil
	})
}

func (s *Service) loadPostsLocked(ctx context.Context) error {
	if s.postIdem == nil {
		s.postIdem = map[string]BookBal{}
	}
	return s.scanKind(ctx, kindPost, func(row persist.Row) error {
		var out storedPost
		if err := json.Unmarshal(row.Body, &out); err != nil {
			return err
		}
		key := strings.TrimSpace(out.Key)
		if key == "" {
			key = row.Key
		}
		if key != "" {
			s.postIdem[key] = out.Bal
		}
		return nil
	})
}

func (s *Service) loadHopsLocked(ctx context.Context) error {
	return s.scanKind(ctx, kindHop, func(row persist.Row) error {
		var h HopRec
		if err := json.Unmarshal(row.Body, &h); err != nil {
			return err
		}
		if h.ID == "" {
			return nil
		}
		cp := h
		s.hops[h.ID] = &cp
		if h.Idem != "" && !h.Reversed {
			s.byIdem[h.Idem] = h.ID
		}
		return nil
	})
}

func (s *Service) persistBookKV(row *bookRow) error {
	if s == nil || s.kv == nil || row == nil {
		return nil
	}
	body, err := json.Marshal(storedBook{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(), Frozen: row.Frozen.String(),
	})
	if err != nil {
		return err
	}
	key := bookKey(row.OwnerID, row.Book, row.Asset)
	exp := row.Fence
	if err := s.kv.Put(context.Background(), persist.Row{Kind: kindBook, Key: key, Body: body}, exp); err != nil {
		if errors.Is(err, persist.ErrStaleFence) {
			return ErrFence
		}
		return err
	}
	got, err := s.kv.Get(context.Background(), kindBook, key)
	if err != nil {
		return err
	}
	row.Fence = got.Fence
	return nil
}

func (s *Service) persistPostKV(key string, bal BookBal) error {
	if s == nil || s.kv == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	body, err := json.Marshal(storedPost{Key: key, Bal: bal})
	if err != nil {
		return err
	}
	cur,
		err := s.kv.Get(context.Background(), kindPost, key)
	exp := persist.Fence{}
	if err == nil {
		exp = cur.Fence
	} else if !errors.Is(err, persist.ErrNotFound) {
		return err
	}
	if err := s.kv.Put(context.Background(), persist.Row{Kind: kindPost, Key: key, Body: body}, exp); err != nil {
		if errors.Is(err, persist.ErrStaleFence) {
			return ErrFence
		}
		return err
	}
	return nil
}

func (s *Service) deletePostKV(key string) {
	if s == nil || s.kv == nil || strings.TrimSpace(key) == "" {
		return
	}
	cur, err := s.kv.Get(context.Background(), kindPost, key)
	if err != nil {
		return
	}
	_ = s.kv.Delete(context.Background(), kindPost, key, cur.Fence)
}

func (s *Service) persistHopKV(h HopRec) error {
	if s == nil || s.kv == nil || strings.TrimSpace(h.Idem) == "" {
		return nil
	}
	body, err := json.Marshal(h)
	if err != nil {
		return err
	}
	cur, err := s.kv.Get(context.Background(), kindHop, h.Idem)
	exp := persist.Fence{}
	if err == nil {
		exp = cur.Fence
	} else if !errors.Is(err, persist.ErrNotFound) {
		return err
	}
	if err := s.kv.Put(context.Background(), persist.Row{Kind: kindHop, Key: h.Idem, Body: body}, exp); err != nil {
		if errors.Is(err, persist.ErrStaleFence) {
			return ErrFence
		}
		return err
	}
	if err := s.persistHopSQL(h); err != nil {
		return err
	}
	return nil
}
