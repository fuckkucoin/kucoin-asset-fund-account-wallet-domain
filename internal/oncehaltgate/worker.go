package oncehaltgate

import (
	"context"
	"time"
)

// Workers 后台工人组合。
type Workers struct {
	Revoke  *RevokeScanner
	Patrol  *DoubleMarkPatrol
	started bool
}

// NewWorkers 创建工人组合。
   func NewWorkers(gate *Gate, revokeInterval, patrolInterval time.Duration) *Workers {
	   return &Workers{
		  Revoke: NewRevokeScanner(gate, revokeInterval),
		  Patrol: NewDoubleMarkPatrol(gate, patrolInterval),
	  }
   }

// Start 启动所有后台工人。幂等。
   func (w *Workers) Start(ctx context.Context) {
	  if w == nil || w.started {
		   return
	   }
	  w.started = true
	  w.Revoke.Start(ctx)
	   w.Patrol.Start(ctx)
  }

// Stop 停止所有后台工人。
  func (w *Workers) Stop() {
	  if w == nil {
		   return
	  }
	  w.Revoke.Stop()
	  w.Patrol.Stop()
	   w.started = false
  }

// WorkerStatus 工人状态快照（运维用）。
type WorkerStatus struct {
	RevokeLast RevokeReport `json:"revoke_last"`
	PatrolLast PatrolReport `json:"patrol_last"`
}

// Status 返回工人状态快照。

   func (w *Workers) Status() WorkerStatus {
	   if w == nil {
		   return WorkerStatus{}
	  }
	  return WorkerStatus{
		  RevokeLast: w.Revoke.LastReport(),
		   PatrolLast: w.Patrol.LastReport(),
	  }
  }
