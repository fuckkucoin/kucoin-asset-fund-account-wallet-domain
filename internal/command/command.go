package command

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusNew Status = "new"
	StatusSucceeded Status = "succeeded"
	StatusFailed   Status = "failed"
	StatusInFlight Status = "in_flight"
)

type Record struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Hash string `json:"payload_hash"`
	Status Status `json:"status"`
	ResultID  string    `json:"result_id,omitempty"`
	Err       string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Log struct {
	mu   sync.Mutex
	rows map[string]*Record
}

  func NewLog() *Log { return &Log{rows: map[string]*Record{}} }

   func HashPayload(parts ...string) string {
	  h := sha256.New()
	  for _, p := range parts {
		   _, _ = h.Write([]byte(p))
		   _, _ = h.Write([]byte{0})
	   }
	   return hex.EncodeToString(h.Sum(nil))
  }

var (
	ErrMissingKey = fmt.Errorf("idempotency key required")
	ErrConflict   = fmt.Errorf("idempotency key reused with different payload")
	ErrInFlight   = fmt.Errorf("command in flight")
)

// Begin 登记命令。已成功则返回 existing=true 和原记录，调用方直接回放结果。

  func (l *Log) Begin(key, kind, payloadHash string) (rec *Record, existing bool, err error) {
	   key = trim(key)
	  if key == "" {
		  return nil, false, ErrMissingKey
	  }
	  l.mu.Lock()
	   defer l.mu.Unlock()
	   if cur, ok := l.rows[key]; ok {
		  if cur.Hash != payloadHash {
			  return cur, true, ErrConflict
		  }
		  if cur.Status == StatusInFlight {
			  return cur, true, ErrInFlight
		  }
		   cp := *cur
		  return &cp, true, nil
	   }
	  now := time.Now().UTC()
	  rec = &Record{Key: key, Kind: kind, Hash: payloadHash, Status: StatusInFlight, CreatedAt: now, UpdatedAt: now}
	  l.rows[key] = rec
	  cp := *rec
	   return &cp, false, nil
   }

   func (l *Log) Succeed(key, resultID string) {
	  l.mu.Lock()
	   defer l.mu.Unlock()
	   if cur, ok := l.rows[key]; ok {
		  cur.Status = StatusSucceeded
		  cur.ResultID = resultID
		   cur.UpdatedAt = time.Now().UTC()
	  }
   }

  func (l *Log) Fail(key string, cause error) {
	  l.mu.Lock()
	  defer l.mu.Unlock()
	  if cur, ok := l.rows[key]; ok {
		  cur.Status = StatusFailed
		   if cause != nil {
			  cur.Err = cause.Error()
		  }
		   cur.UpdatedAt = time.Now().UTC()
	  }
  }

   func (l *Log) Get(key string) (*Record, bool) {
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  cur, ok := l.rows[key]
	  if !ok {
		  return nil, false
	   }
	   cp := *cur
	  return &cp, true
  }

  func trim(s string) string {
	  for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		  s = s[1:]
	   }
	  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		  s = s[:len(s)-1]
	   }
	  return s
   }
