package hotbound

import "context"

// PreSubmit 只读热库存。超 cap 或库存不可读则失败即关。

  func (s *Service) PreSubmit(ctx context.Context, asset string) error {
	  if s == nil || s.hot == nil {
		  return ErrUnavailable
	   }
	   if s.store == nil {
		   return ErrStore
	   }
	   return s.hot.PreSubmit(ctx, s.store, asset)
   }
