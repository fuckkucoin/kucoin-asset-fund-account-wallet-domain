package recycleepoch

import (
	"context"
	"strings"
)

   func (s *Service) GetBind(ctx context.Context, chain, address string) (*ObserveBind, error) {
	  slot := NormalizeSlot(chain, address)
	  if slot.Empty() {
		   return nil, ErrAddressEmpty
	   }
	   return s.store.GetBind(ctx, slot)
   }

  func (s *Service) ListUserBinds(ctx context.Context, userID string, limit int) ([]ObserveBind, error) {
	  uid := strings.TrimSpace(userID)
	  if uid == "" {
		  return nil, ErrUserEmpty
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	   return s.store.ListBindsByUser(ctx, uid, limit)
   }

   func (s *Service) BindAllows(ctx context.Context, chain, address, user string, gen int64) (bool, error) {
	   b, err := s.GetBind(ctx, chain, address)
	  if err != nil {
		   return false, err
	   }
	  return b.AllowsCredit(user, gen), nil
   }
