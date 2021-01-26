package addrpool

import (
	"fmt"
	"strings"
)

// AssignSticky 粘性领址：uid+asset+chain 已绑则复用。
  func AssignSticky(pool *Pool, shared *SharedMemoPool, binder MemoBinder, marks *MarkBook, req AssignReq) (AssignedView, error) {
	   uid := strings.TrimSpace(req.UserID)
	   ch := strings.ToUpper(strings.TrimSpace(req.Chain))
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  if uid == "" || ch == "" {
		  return AssignedView{}, fmt.Errorf("user_id and chain required")
	  }
	  if pool != nil {
		  if b, ok := pool.Lookup(uid, ch); ok && b.Allocated {
			  if marks != nil && marks.Blocked(b.Address) {
				   return AssignedView{}, ErrCompromised
			   }
			  view := AssignedView{
				  UserID: b.UserID, Chain: b.Chain, Asset: asset, Family: b.Family, Address: b.Address,
				  BoundAt: b.BoundAt, Allocated: b.Allocated, Found: true, ReuseAfter: b.Cooldown,
			   }
			   stampFormat(&view)
			  return view, nil
		  }
	  }
	   if shared != nil {
		   view := InspectAssigned(pool, shared, uid, ch, asset, req.Memo)
		   if view.Found && view.Address != "" {
			  if marks != nil && marks.Blocked(view.Address) {
				   return AssignedView{}, ErrCompromised
			   }
			  return view, nil
		   }
	  }
	  got, err := Assign(pool, shared, binder, req)
	  if err != nil {
		  return AssignedView{}, err
	  }
	  if marks != nil && marks.Blocked(got.Address) {
		   return AssignedView{}, ErrCompromised
	  }
	  return got, nil
   }
