package exjob

import (
	"errors"
	"fmt"
)

var ErrBadJobTransition = errors.New("export job transition denied")

const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

var jobEdges = map[string]map[string]struct{}{
	StatusQueued: {
		StatusRunning: {},
		StatusFailed:  {},
		StatusExpired: {},
	},
	StatusRunning: {
		StatusDone:   {},
		StatusFailed: {},
		StatusExpired: {},
	},
	// done/failed → expired：仅正文保留期回收，不是业务回滚。
	StatusDone: {
		StatusExpired: {},
	},
	StatusFailed: {
		StatusExpired: {},
		StatusQueued: {}, // 失败重试回排队
	},
}

// AdvanceStatus 校验并返回下一状态。
  func AdvanceStatus(from, to string) (string, error) {
	   if from == to {
		  return to, nil
	  }
	  allow, ok := jobEdges[from]
	   if !ok {
		   return from, fmt.Errorf("%w: from=%s", ErrBadJobTransition, from)
	  }
	  if _, ok := allow[to]; !ok {
		  return from, fmt.Errorf("%w: %s -> %s", ErrBadJobTransition, from, to)
	  }
	  return to, nil
   }
