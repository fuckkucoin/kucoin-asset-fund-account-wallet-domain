package fundmat

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// UserDirectory 全量重建时枚举用户。未实现时只覆盖投影里已有的用户。
type UserDirectory interface {
	Users(ctx context.Context) ([]string, error)
}

// Enqueue 登记重建。同一用户已有活动任务时返回该任务，不另开一单。

  func (s *Service) Enqueue(ctx context.Context, userID, scope, tol string) (RebuildJob, error) {
	   if s == nil || s.st == nil {
		  return RebuildJob{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  userID = NormUser(userID)
	  scope = normScope(scope)
	   if scope == "user" && userID == "" {
		   return RebuildJob{}, ErrBadEvent
	   }
	   tol = normTol(tol)
	   if userID != "" {
		   if j, ok, err := s.activeLocked(ctx, userID); err != nil {
			  return RebuildJob{}, err
		  } else if ok {
			   return j, nil
		  }
	   }
	   now := time.Now().UTC()
	  j := RebuildJob{
		   ID: uuid.NewString(), Scope: scope, UserID: userID,
		   Status: JobQueued, Tolerance: tol, QueuedAt: now,
	  }
	  if err := s.st.PutJob(ctx, j); err != nil {
		   return RebuildJob{}, err
	   }
	   noteRebuild("queued")
	  return j, nil
  }

// Drain 把排队任务推进到完成或失败。单轮最多处理 20 单。

   func (s *Service) Drain(ctx context.Context) ([]RebuildJob, error) {
	   if s == nil || s.st == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  queued, err := s.st.ListJobs(ctx, JobQueued, 20)
	   if err != nil {
		   return nil, err
	  }
	   out := make([]RebuildJob, 0, len(queued))
	  for _, j := range queued {
		  now := time.Now().UTC()
		  running := j
		  running.Status = JobRunning
		   running.StartedAt = &now
		   running, err = s.st.CasJob(ctx, j.ID, JobQueued, running)
		  if err != nil {
			   return out, err
		   }
		  n, sample, runErr := s.execLocked(ctx, running, now)
		  end := time.Now().UTC()
		  done := running
		  done.DriftCount = n
		  done.DriftSample = sample
		   done.FinishedAt = &end
		   if runErr != nil {
			   done.Status = JobFailed
			   done.Error = runErr.Error()
			  noteRebuild("failed")
		  } else {
			  done.Status = JobDone
			  noteRebuild("done")
		   }
		   done, err = s.st.CasJob(ctx, j.ID, JobRunning, done)
		   if err != nil {
			  return out, err
		  }
		   out = append(out, done)
	   }
	  return out, nil
  }

// RebuildNow 入队并立刻执行，供读面与回归一次走完。
   func (s *Service) RebuildNow(ctx context.Context, userID, scope, tol string) (RebuildJob, error) {
	  j, err := s.Enqueue(ctx, userID, scope, tol)
	  if err != nil {
		   return RebuildJob{}, err
	   }
	  if j.Status != JobQueued {
		   return j, nil
	  }
	   done, err := s.Drain(ctx)
	   if err != nil {
		   return RebuildJob{}, err
	   }
	   for _, item := range done {
		   if item.ID == j.ID {
			  return item, nil
		   }
	   }
	   return s.Job(ctx, j.ID)
   }

  func (s *Service) execLocked(ctx context.Context, j RebuildJob, now time.Time) (int, string, error) {
	  if s.books == nil {
		  return 0, "", ErrNoBooks
	  }
	  users, err := s.jobUsers(ctx, j)
	   if err != nil {
		   return 0, "", err
	   }
	  var all []Drift
	  for _, user := range users {
		  ds, err := s.rebuildUserLocked(ctx, user, j.Tolerance, now)
		   if err != nil {
			  return len(all) + len(ds),
				   formatDrifts(append(all, ds...)), err
		  }
		   all = append(all, ds...)
	   }
	  if err := s.bumpEpochLocked(ctx, now); err != nil {
		  return len(all),
			   formatDrifts(all), err
	  }
	  for _, user := range users {
		  if err := s.bumpUserLocked(ctx, user, now); err != nil {
			   return len(all), formatDrifts(all), err
		   }
	   }
	   if len(all) > 0 {
		   return len(all),
			   formatDrifts(all), ErrDrift
	  }
	   return 0, "", nil
   }

   func (s *Service) jobUsers(ctx context.Context, j RebuildJob) ([]string, error) {
	   if j.Scope != "all" && j.UserID != "" {
		  return []string{j.UserID}, nil
	   }
	  seen := map[string]struct{}{}
	  var users []string
	   add := func(list []string) {
		  for _, u := range list {
			  u = NormUser(u)
			   if u == "" {
				   continue
			   }
			  if _, ok := seen[u]; ok {
				  continue
			  }
			  seen[u] = struct{}{}
			   users = append(users, u)
		  }
	   }
	  if dir, ok := s.books.(UserDirectory); ok {
		   list, err := dir.Users(ctx)
		  if err != nil {
			   return nil, err
		  }
		   add(list)
	   }
	  have, err := s.st.ListUsers(ctx, 500)
	  if err != nil {
		   return nil, err
	   }
	   add(have)
	  if j.UserID != "" {
		   add([]string{j.UserID})
	   }
	  return users, nil
  }

  func (s *Service) rebuildUserLocked(ctx context.Context, user, tol string, now time.Time) ([]Drift, error) {
	  lines, err := s.books.Lines(ctx, user)
	   if err != nil {
		  return nil, err
	  }
	   type key struct{ asset, subject string }
	   seen := map[key]BookLine{}
	  for _, ln := range lines {
		  ln.UserID = NormUser(user)
		   ln.Asset = NormAsset(ln.Asset)
		  ln.Subject = NormSubject(ln.Subject)
		   if ln.Asset == "" || ln.Subject == "" {
			   continue
		  }
		   avail, err := Canon(ln.Available)
		   if err != nil {
			   return nil, err
		  }
		  frozen, err := Canon(ln.Frozen)
		   if err != nil {
			   return nil, err
		  }
		  row, ok, err := s.st.GetRow(ctx, ln.UserID, ln.Asset, ln.Subject)
		   if err != nil {
			   return nil, err
		   }
		   if !ok {
			  row = BlankRow(ln.UserID, ln.Asset, ln.Subject, now)
		  }
		   row.Available = avail
		   row.Frozen = frozen
		  row.RowVersion++
		   row.UpdatedAt = now.UTC()
		   if err := s.st.PutRow(ctx, row); err != nil {
			   return nil, err
		  }
		   seen[key{ln.Asset,
			  ln.Subject}] = ln
	  }
	  existing, err := s.st.ListUser(ctx, user)
	   if err != nil {
		  return nil, err
	   }
	   for _, row := range existing {
		   if _, ok := seen[key{row.Asset, row.Subject}]; ok {
			   continue
		  }
		   row.Available = "0"
		  row.Frozen = "0"
		   row.RowVersion++
		   row.UpdatedAt = now.UTC()
		   if err := s.st.PutRow(ctx, row); err != nil {
			  return nil, err
		   }
	  }
	  if _, err := s.st.SealUserBalance(ctx, user, now); err != nil {
		   return nil, err
	  }
	  rows, err := s.st.ListUser(ctx, user)
	  if err != nil {
		   return nil, err
	   }
	  return Compare(rows, lines, tol)
   }

  func (s *Service) activeLocked(ctx context.Context, userID string) (RebuildJob, bool, error) {
	  jobs, err := s.st.ListJobs(ctx, "", 500)
	  if err != nil {
		   return RebuildJob{}, false, err
	  }
	  for _, j := range jobs {
		   if j.UserID == userID && j.Status.Active() {
			   return j, true, nil
		   }
	  }
	  return RebuildJob{}, false, nil
   }

  func normScope(scope string) string {
	  switch scope {
	  case "all":
		   return "all"
	   default:
		   return "user"
	   }
  }

  func normTol(tol string) string {
	  if tol == "" {
		  return "0"
	  }
	  return tol
  }
