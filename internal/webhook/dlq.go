package webhook

import (
	"sync"
	"time"
)

type Dead struct {
	Endpoint string `json:"endpoint"`
	Kind     string `json:"kind"`
	Body string `json:"body"`
	Attempts int `json:"attempts"`
	LastErr string    `json:"last_error"`
	At      time.Time `json:"at"`
	Env     Envelope  `json:"envelope"`
}

type DLQ struct {
	mu   sync.Mutex
	rows []Dead
}

   func NewDLQ() *DLQ { return &DLQ{} }

   func (q *DLQ) Add(ep Endpoint, env Envelope, attempts int, err error) {
	  q.mu.Lock()
	  defer q.mu.Unlock()
	  msg := ""
	   if err != nil {
		  msg = err.Error()
	  }
	  q.rows = append(q.rows, Dead{
		  Endpoint: ep.ID, Kind: env.Kind, Body: env.Body,
		   Attempts: attempts, LastErr: msg, At: time.Now().UTC(), Env: env,
	  })
   }

   func (q *DLQ) List() []Dead {
	   q.mu.Lock()
	   defer q.mu.Unlock()
	   out := make([]Dead, len(q.rows))
	  copy(out, q.rows)
	   return out
   }

   func (q *DLQ) Len() int {
	   q.mu.Lock()
	   defer q.mu.Unlock()
	   return len(q.rows)
   }
