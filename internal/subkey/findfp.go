package subkey

import "strings"

// FindByFP 母户下按 secretFp 精确匹配；无则 ErrGone。

  func (s *Store) FindByFP(owner, secretFP string) (Record, error) {
	  if s == nil {
		  return Record{}, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	  secretFP = strings.TrimSpace(secretFP)
	  if owner == "" {
		  return Record{}, ErrOwner
	   }
	  if secretFP == "" {
		   return Record{}, ErrKey
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   for _, r := range s.by {
		  if r.Owner != owner {
			   continue
		  }
		  if r.SecretFP == secretFP {
			   return r.Record, nil
		   }
	  }
	   return Record{}, ErrGone
  }
