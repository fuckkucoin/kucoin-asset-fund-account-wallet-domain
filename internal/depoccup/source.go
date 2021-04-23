package depoccup

import (
	"strings"
	"sync"
)

   func ParseSource(s string) (Source, error) {
	   switch Source(strings.ToLower(strings.TrimSpace(s))) {
	   case SourceSelf:
		  return SourceSelf, nil
	   case SourceVASP:
		  return SourceVASP, nil
	   case SourceExchange:
		   return SourceExchange, nil
	  case SourceContract:
		   return SourceContract, nil
	  case SourceUnknown:
		   return SourceUnknown, nil
	  case SourceMixer:
		  return SourceMixer, nil
	   case "*":
		  return "*", nil
	  default:
		   return "", ErrSource
	   }
   }

  func SourceRank(s Source) int {
	  switch s {
	   case SourceMixer:
		   return 6
	  case SourceUnknown:
		  return 5
	  case SourceContract:
		   return 4
	   case SourceExchange:
		  return 3
	  case SourceVASP:
		   return 2
	   case SourceSelf:
		   return 1
	   default:
		   return 0
	   }
   }

   func RiskierSource(a, b Source) Source {
	  if SourceRank(a) >= SourceRank(b) {
		   return a
	   }
	   return b
   }

type SourceBook struct {
	mu     sync.Mutex
	byAddr map[string]Source
	byTag map[string]Source
}

   func NewSourceBook() *SourceBook {
	   b := &SourceBook{byAddr: map[string]Source{}, byTag: map[string]Source{}}
	   b.seedTags()
	  return b
  }

  func (b *SourceBook) seedTags() {
	   tags := map[string]Source{
		   "self":   SourceSelf,
		   "own":    SourceSelf,
		  "vasp":   SourceVASP,
		  "travel": SourceVASP,
		   "exchange": SourceExchange,
		   "cex":      SourceExchange,
		   "contract": SourceContract,
		   "token":    SourceContract,
		   "unknown": SourceUnknown,
		   "mixer":   SourceMixer,
		   "tumbler": SourceMixer,
	  }
	   for k, v := range tags {
		   b.byTag[k] = v
	   }
  }

  func (b *SourceBook) PutAddress(addr string, src Source) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	  src, err := ParseSource(string(src))
	   if err != nil || src == "*" {
		   return ErrSource
	  }
	   addr = normAddr(addr)
	  if addr == "" {
		  return ErrSource
	  }
	   b.mu.Lock()
	  b.byAddr[addr] = src
	  b.mu.Unlock()
	  return nil
  }

  func (b *SourceBook) Address(addr string) (Source, bool) {
	   if b == nil {
		  return "", false
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   s, ok := b.byAddr[normAddr(addr)]
	   return s, ok
  }

  func normAddr(s string) string {
	  return strings.ToLower(strings.TrimSpace(s))
   }

// Classify 提示、地址册、标签三步。mixer 命中后不再降级。
  func (b *SourceBook) Classify(hint, from string) (Source, error) {
	   if b == nil {
		  b = NewSourceBook()
	   }
	   hint = strings.ToLower(strings.TrimSpace(hint))
	  if hint != "" {
		  if src, err := ParseSource(hint); err == nil && src != "*" {
			   return src, nil
		  }
		  b.mu.Lock()
		   tag, ok := b.byTag[hint]
		  b.mu.Unlock()
		   if ok {
			  return tag, nil
		   }
		   return "", ErrSource
	  }
	  if src, ok := b.Address(from); ok {
		  return src, nil
	  }
	   return SourceUnknown, nil
   }
