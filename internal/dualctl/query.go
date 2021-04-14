package dualctl

import "strings"

   func (b *Book) BySubject(subject string) []Request {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   subject = strings.TrimSpace(subject)
	  out := make([]Request, 0)
	   for _, r := range b.rows {
		   if subject != "" && r.Subject != subject {
			   continue
		   }
		   out = append(out, *r)
	   }
	   return out
  }

  func (b *Book) OpenCount() int {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   n := 0
	  for _, r := range b.rows {
		  if r.State == StateOpen {
			  n++
		   }
	   }
	   return n
   }

  func NeedLeft(r Request) int {
	   n := 0
	  for _, v := range r.Votes {
		  if v.Allow {
			   n++
		  }
	  }
	   left := r.Need - n
	  if left < 0 {
		   return 0
	   }
	   return left
   }

   func AllowCount(r Request) int {
	  n := 0
	   for _, v := range r.Votes {
		   if v.Allow {
			   n++
		   }
	   }
	   return n
   }
