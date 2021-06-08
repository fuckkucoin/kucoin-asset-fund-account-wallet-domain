package fundgraph

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"
)

// Source 是只读摄取源。Pull 按游标返回下一批事实，不改上游状态机。

type Source interface {
	Name() string
	Stream() string
	Pull(ctx context.Context, token string) ([]IngestBatch, string, error)
}

// PumpReport 是一轮泵送加巡检的结果。
type PumpReport struct {
	Ingested  int `json:"ingested"`
	Replays   int `json:"replays"`
	Conflicts int `json:"conflicts"`
	Skipped int `json:"skipped"`
	Errors []string `json:"errors,omitempty"`
	Inspect InspectReport `json:"inspect"`
}

// Worker 顺序泵送各个源，然后做一轮完整性巡检。

type Worker struct {
	Svc *Service
	Sources  []Source
	Interval time.Duration
}

// Once 泵送一轮并巡检。每个源连续拉取，直到没有新游标或碰到冲突。冲突不推进该源游标。
  func (w *Worker) Once(ctx context.Context) (PumpReport, error) {
	  if w == nil || w.Svc == nil {
		  return PumpReport{}, ErrInvalid
	  }
	   report := PumpReport{}
	   for _, src := range w.Sources {
		   if src == nil {
			   continue
		   }
		   for step := 0; step < 10000; step++ {
			  if err := ctx.Err(); err != nil {
				  return report, err
			   }
			  cur, _, err := w.Svc.GetCursor(ctx, src.Name(), src.Stream())
			   if err != nil {
				   report.Errors = append(report.Errors, src.Name()+": "+err.Error())
				  break
			   }
			   batches, next, err := src.Pull(ctx, cur.Token)
			   if err != nil {
				   if errors.Is(err, ErrSkip) {
					  report.Skipped++
					  break
				   }
				  report.Errors = append(report.Errors, src.Name()+": "+err.Error())
				  break
			  }
			  if len(batches) == 0 && (next == "" || next == cur.Token) {
				   break
			  }
			  failed := false
			   var lastEvent time.Time
			   for _, b := range batches {
				   if b.Source == "" {
					   b.Source = src.Name()
				  }
				  if b.Stream == "" {
					   b.Stream = src.Stream()
				  }
				   res, err := w.Svc.Ingest(ctx, b)
				  if errors.Is(err, ErrSkip) {
					  report.Skipped++
					  continue
				  }
				  if errors.Is(err, ErrConflict) {
					  report.Conflicts++
					  report.Errors = append(report.Errors, src.Name()+": conflict")
					   failed = true
					   break
				   }
				  if err != nil {
					   report.Errors = append(report.Errors, src.Name()+": "+err.Error())
					   failed = true
					   break
				  }
				  report.Ingested += res.InsertedEdges + res.InsertedNodes
				  report.Replays += res.ReplayEdges + res.ReplayNodes
				   if b.EventAt.After(lastEvent) {
					   lastEvent = b.EventAt
				   }
			  }
			   if failed {
				  break
			   }
			   if len(batches) == 0 {
				  report.Skipped++
			   }
			   if next == "" || next == cur.Token {
				  break
			   }
			   if err := w.Svc.AdvanceCursor(ctx, src.Name(), src.Stream(), next, lastEvent); err != nil {
				  report.Errors = append(report.Errors, src.Name()+": cursor "+err.Error())
				   break
			  }
		  }
	  }
	   ins, err := w.Svc.Inspect(ctx)
	   if err != nil {
		  return report, err
	  }
	   report.Inspect = ins
	  return report, nil
  }

// Run 按间隔循环，直到上下文取消。
   func (w *Worker) Run(ctx context.Context) error {
	  if w == nil {
		   return ErrInvalid
	  }
	  interval := w.Interval
	  if interval <= 0 {
		  interval = 5 * time.Second
	  }
	  ticker := time.NewTicker(interval)
	   defer ticker.Stop()
	  if _, err := w.Once(ctx); err != nil && !errors.Is(err, context.Canceled) {
		  return err
	   }
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			  if _, err := w.Once(ctx); err != nil && !errors.Is(err, context.Canceled) {
				   return err
			  }
		  }
	   }
   }

// MemorySource 把内存中的批次按整数游标逐条吐出。

type MemorySource struct {
	name string
	stream string
	mu     sync.Mutex
	items  []IngestBatch
}

// NewMemorySource 构造内存源。

   func NewMemorySource(name, stream string, items []IngestBatch) *MemorySource {
	  cp := append([]IngestBatch{}, items...)
	   if stream == "" {
		  stream = "default"
	  }
	  return &MemorySource{name: name, stream: stream, items: cp}
   }

  func (m *MemorySource) Name() string {
	   if m == nil || m.name == "" {
		   return "memory"
	  }
	  return m.name
  }

   func (m *MemorySource) Stream() string {
	  if m == nil || m.stream == "" {
		  return "default"
	   }
	  return m.stream
   }

// Pull 每次返回一只批次。游标是下一条下标的十进制字符串。
  func (m *MemorySource) Pull(ctx context.Context, token string) ([]IngestBatch, string, error) {
	   if err := ctx.Err(); err != nil {
		  return nil, token, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   i := 0
	   if token != "" {
		  n, err := strconv.Atoi(token)
		   if err != nil {
			  return nil, token, invalidf("cursor")
		  }
		   i = n
	   }
	   if i >= len(m.items) {
		  return nil, strconv.Itoa(len(m.items)), nil
	   }
	   return []IngestBatch{m.items[i]}, strconv.Itoa(i + 1), nil
  }

// EnvelopeSource 从总线信封列表摄取。

type EnvelopeSource struct {
	name   string
	stream string
	mu     sync.Mutex
	items []BusEnvelope
}

// NewEnvelopeSource 构造信封源。
  func NewEnvelopeSource(name, stream string, items []BusEnvelope) *EnvelopeSource {
	   if stream == "" {
		   stream = "bus"
	  }
	   return &EnvelopeSource{name: name, stream: stream, items: append([]BusEnvelope{}, items...)}
   }

  func (e *EnvelopeSource) Name() string {
	  if e == nil || e.name == "" {
		   return "bus"
	   }
	  return e.name
  }

   func (e *EnvelopeSource) Stream() string {
	   if e == nil || e.stream == "" {
		   return "bus"
	   }
	   return e.stream
   }

// Pull 解析下一只信封。未知种类推进游标并跳过；非法载荷不推进。
  func (e *EnvelopeSource) Pull(ctx context.Context,
	   token string) ([]IngestBatch, string, error) {
	  if err := ctx.Err(); err != nil {
		  return nil, token, err
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   i := 0
	  if token != "" {
		   n, err := strconv.Atoi(token)
		   if err != nil {
			   return nil, token, invalidf("cursor")
		  }
		   i = n
	   }
	   if i >= len(e.items) {
		   return nil, strconv.Itoa(len(e.items)), nil
	   }
	   b, err := AdaptBus(e.items[i])
	   if errors.Is(err, ErrSkip) {
		   return nil, strconv.Itoa(i + 1), nil
	  }
	  if err != nil {
		   return nil, token, err
	   }
	   return []IngestBatch{b}, strconv.Itoa(i + 1), nil
  }

// LoadEnvelopesJSONL 读取一行一只的总线信封。
   func LoadEnvelopesJSONL(r io.Reader) ([]BusEnvelope, error) {
	   sc := bufio.NewScanner(r)
	   sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	  var out []BusEnvelope
	   for sc.Scan() {
		  line := stringsTrim(sc.Text())
		  if line == "" || line[0] == '#' {
			  continue
		  }
		   var env BusEnvelope
		   if err := json.Unmarshal([]byte(line), &env); err != nil {
			  return nil, invalidf("jsonl")
		  }
		  out = append(out, env)
	   }
	  if err := sc.Err(); err != nil {
		  return nil, err
	   }
	  return out, nil
   }

   func stringsTrim(s string) string {
	   for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		  s = s[1:]
	  }
	  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		  s = s[:len(s)-1]
	  }
	   return s
  }
