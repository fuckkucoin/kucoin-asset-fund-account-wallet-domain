package fundmat

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// Patrol 刷新水位。滞后超过时限，或与账本出现差异时，打开一条告警。
// 同一类未关闭告警不重复打开。

   func (s *Service) Patrol(ctx context.Context) ([]Alert, error) {
	  if s == nil || s.st == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  now := time.Now().UTC()
	  for _, st := range Streams() {
		   if _, err := s.refreshLocked(ctx, st, now); err != nil {
			  return nil, err
		  }
	  }
	   wms, err := s.st.ListWM(ctx)
	   if err != nil {
		   return nil, err
	  }
	   open, err := s.st.ListAlerts(ctx, true, 500)
	  if err != nil {
		  return nil, err
	  }
	   has := map[string]struct{}{}
	  for _, a := range open {
		   has[alertKey(a)] = struct{}{}
	   }
	  var raised []Alert
	  for _, wm := range wms {
		  if wm.LagEvents == 0 || wm.WithinSLA {
			   continue
		  }
		   a := Alert{
			   ID: uuid.NewString(), Kind: "lag", Stream: string(wm.Stream),
			   Detail: fmt.Sprintf("lag_events=%d lag_ms=%d sla_ms=%d", wm.LagEvents, wm.LagMs, wm.SLAMs),
			  Open: true,
			  RaisedAt: now,
		   }
		   if _, ok := has[alertKey(a)]; ok {
			  continue
		  }
		   if err := s.st.PutAlert(ctx, a); err != nil {
			  return raised, err
		  }
		  has[alertKey(a)] = struct{}{}
		   raised = append(raised, a)
		  noteAlert("lag")
	   }
	   if s.books == nil {
		  return raised, nil
	  }
	   users, err := s.st.ListUsers(ctx, 32)
	  if err != nil {
		   return raised, err
	   }
	  for _, user := range users {
		  lines, err := s.books.Lines(ctx, user)
		  if err != nil {
			  return raised, err
		   }
		   rows, err := s.st.ListUser(ctx, user)
		   if err != nil {
			  return raised, err
		   }
		  drifts, err := Compare(rows, lines, "0")
		   if err != nil {
			  return raised, err
		  }
		   if len(drifts) == 0 {
			  continue
		  }
		  a := Alert{
			   ID: uuid.NewString(), Kind: "drift", UserID: user,
			  Detail: formatDrifts(drifts),
			  Open:   true, RaisedAt: now,
		   }
		  if _, ok := has[alertKey(a)]; ok {
			  continue
		   }
		   if err := s.st.PutAlert(ctx, a); err != nil {
			  return raised, err
		  }
		   has[alertKey(a)] = struct{}{}
		   raised = append(raised, a)
		  noteAlert("drift")
	   }
	  return raised, nil
   }

// CloseAlert 关闭一条告警。巡检不会把它再次打开，除非种类或用户变了。
  func (s *Service) CloseAlert(ctx context.Context, id string) (Alert, error) {
	   if s == nil || s.st == nil {
		   return Alert{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  list, err := s.st.ListAlerts(ctx, false, 500)
	  if err != nil {
		   return Alert{}, err
	   }
	   for _, a := range list {
		  if a.ID != id {
			  continue
		   }
		  a.Open = false
		  if err := s.st.PutAlert(ctx, a); err != nil {
			   return Alert{}, err
		  }
		   return a, nil
	   }
	   return Alert{}, ErrNotFound
  }

   func alertKey(a Alert) string {
	  return a.Kind + "|" + a.Stream + "|" + a.UserID
   }
