package persist

import "strings"

// CompareAndBump 仅当 epoch/token 仍匹配时换新 token。并发写失败即关。
   func (t *Table) CompareAndBump(id string, epoch int64, token, next string) (Fence, error) {
	   if t == nil {
		   return Fence{}, ErrNotFound
	  }
	  id = strings.TrimSpace(id)
	   if id == "" {
		  return Fence{}, ErrNotFound
	   }
	  if err := t.Check(id, epoch, token); err != nil {
		  return Fence{}, err
	   }
	  if strings.TrimSpace(next) == "" {
		  next = NewToken()
	  }
	  return t.Bump(id, next)
  }

// CompareAndHold 只核对，不升 epoch。读路径确认仍持有围栏。
   func (t *Table) CompareAndHold(id string, epoch int64, token string) error {
	   return t.Check(id, epoch, token)
   }

// Init 首次盖围栏。已存在则冲突，避免覆盖活行。
  func (t *Table) Init(id, token string) (Fence, error) {
	   if t == nil {
		  return Fence{}, ErrNotFound
	  }
	  id = strings.TrimSpace(id)
	   if id == "" {
		  return Fence{}, ErrNotFound
	   }
	   if _, ok := t.Get(id); ok {
		  return Fence{}, ErrConflict
	  }
	  if strings.TrimSpace(token) == "" {
		  token = NewToken()
	  }
	  return t.Bump(id, token)
   }
