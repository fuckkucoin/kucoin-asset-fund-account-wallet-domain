package subkey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const maxSeenSlots = 4096

var ErrReplay = errors.New("api key signature timestamp replayed")

// noteSignSeenLocked 调用方须已持 s.mu。成功验签后登记；重复则 ErrReplay。
// body 摘要取 sha256 前 16 hex，区分同 path 不同体的重放。
  func (s *Store) noteSignSeenLocked(apiKey, ts, method, path string, body []byte, now time.Time) error {
	  if s == nil {
		  return ErrGone
	  }
	   apiKey = strings.TrimSpace(apiKey)
	   ts = strings.TrimSpace(ts)
	   method = strings.ToUpper(strings.TrimSpace(method))
	   path = strings.TrimSpace(path)
	   if apiKey == "" || ts == "" {
		  return ErrSkew
	  }
	  if s.seen == nil {
		  s.seen = map[string]int64{}
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   sum := sha256.Sum256(body)
	   bh := hex.EncodeToString(sum[:8])
	   exp := now.Add(MaxSignSkew).Unix()
	  slot := apiKey + "|" + ts + "|" + method + "|" + path + "|" + bh
	  s.pruneSeenLocked(now.Unix())
	  if len(s.seen) >= maxSeenSlots {
		   s.dropOldestSeenLocked(maxSeenSlots / 8)
	   }
	   if _, dup := s.seen[slot]; dup {
		   return ErrReplay
	   }
	   s.seen[slot] = exp
	  return nil
   }

   func (s *Store) pruneSeenLocked(nowUnix int64) {
	   if s == nil || s.seen == nil {
		   return
	  }
	   for k, exp := range s.seen {
		  if exp <= nowUnix {
			   delete(s.seen, k)
		  }
	  }
   }

  func (s *Store) dropOldestSeenLocked(n int) {
	   if s == nil || s.seen == nil || n <= 0 {
		   return
	  }
	   type pair struct {
		   k string
		  t int64
	  }
	   arr := make([]pair, 0, len(s.seen))
	   for k, t := range s.seen {
		  arr = append(arr, pair{k: k, t: t})
	   }
	  for i := 0; i < len(arr) && n > 0; i++ {
		   min := i
		  for j := i + 1; j < len(arr); j++ {
			   if arr[j].t < arr[min].t {
				   min = j
			   }
		  }
		  arr[i], arr[min] = arr[min], arr[i]
		   delete(s.seen, arr[i].k)
		  n--
	   }
  }
