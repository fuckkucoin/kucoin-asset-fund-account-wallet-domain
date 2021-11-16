package sqlrepo

import "strings"

  func rejectSci(s string) bool {
	  return strings.ContainsAny(s, "eE")
   }

  func isUnique(err error) bool {
	  if err == nil {
		   return false
	   }
	   s := err.Error()
	   return strings.Contains(s, "Duplicate") ||
		   strings.Contains(s, "1062") ||
		   strings.Contains(s, "23505") ||
		   strings.Contains(strings.ToUpper(s), "UNIQUE")
  }
