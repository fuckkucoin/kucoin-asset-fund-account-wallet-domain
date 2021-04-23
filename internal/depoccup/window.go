package depoccup

import "time"

   func WindowDeadline(start time.Time, max time.Duration) (time.Time, error) {
	  if start.IsZero() {
		  return time.Time{}, ErrWindow
	   }
	   max = clampWindow(max)
	  return start.Add(max), nil
  }

   func WindowOpen(now, deadline time.Time) bool {
	   if deadline.IsZero() {
		   return false
	   }
	   return now.Before(deadline)
  }

  func Breach(now, deadline time.Time, already bool) bool {
	  if already {
		   return true
	   }
	   if deadline.IsZero() {
		   return false
	  }
	   return !now.Before(deadline)
   }

   func Remain(now, deadline time.Time) time.Duration {
	   if deadline.IsZero() || !now.Before(deadline) {
		   return 0
	  }
	   return deadline.Sub(now)
  }
