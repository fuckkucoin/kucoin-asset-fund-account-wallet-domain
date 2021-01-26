package accrual

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindAccrualBatch = "accrual_batch"
	kindAccrualLines = "accrual_lines"
	kindAccrualDump  = "accrual_dump"
)

// Dump 引擎完整状态快照（用于序列化）。

type Dump struct {
	Batches []AccrualBatch `json:"batches"`
	Lines   []batchLines   `json:"lines"`
}

// batchLines 单批次分录集合。

type batchLines struct {
	BatchID string        `json:"batch_id"`
	Lines   []AccrualLine `json:"lines"`
}

// Save 将引擎状态序列化到 KV 存储。key 为全局唯一键（如 "main"）。
  func (e *Engine) Save(ctx context.Context, kv persist.KV, key string) error {
	   if e == nil || kv == nil {
		  return ErrEngineNil
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrInvalidDay
	  }
	   e.mu.Lock()
	   dump := Dump{}
	   for _, bat := range e.batches {
		  dump.Batches = append(dump.Batches, *bat)
	   }
	   for batchID, lineList := range e.lines {
		  bl := batchLines{BatchID: batchID}
		   for _, l := range lineList {
			   bl.Lines = append(bl.Lines, *l)
		   }
		  dump.Lines = append(dump.Lines, bl)
	  }
	  e.mu.Unlock()
	  body, err := json.Marshal(dump)
	  if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindAccrualDump, key, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		  return persist.Replace(ctx, kv, kindAccrualDump, key, body)
	   }
	   return nil
   }

// LoadEngine 从 KV 存储恢复引擎状态。
   func LoadEngine(ctx context.Context, kv persist.KV, key string) (*Engine, error) {
	   if kv == nil {
		  return nil, ErrEngineNil
	   }
	   row,
		  err := kv.Get(ctx, kindAccrualDump, strings.TrimSpace(key))
	   if err != nil {
		  if err == persist.ErrNotFound {
			  return NewEngine(), nil
		   }
		  return nil, err
	  }
	  var dump Dump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	   }
	  eng := NewEngine()
	   eng.mu.Lock()
	  for i := range dump.Batches {
		   cp := dump.Batches[i]
		  eng.batches[cp.ID] = &cp
	  }
	  for _, bl := range dump.Lines {
		  lineList := make([]*AccrualLine,
			  0, len(bl.Lines))
		   maxSeq := 0
		   for i := range bl.Lines {
			   cp := bl.Lines[i]
			  lineList = append(lineList,
				  &cp)
			  if cp.Seq > maxSeq {
				   maxSeq = cp.Seq
			   }
		  }
		  eng.lines[bl.BatchID] = lineList
		  eng.seqMap[bl.BatchID] = maxSeq + 1
	   }
	   eng.mu.Unlock()
	   return eng, nil
  }

// SaveBatch 仅持久化单个批次（增量写，避免全量覆盖）。
  func (e *Engine) SaveBatch(ctx context.Context, kv persist.KV, batchID string) error {
	  if e == nil || kv == nil {
		  return ErrEngineNil
	   }
	   e.mu.Lock()
	   bat, ok := e.batches[strings.TrimSpace(batchID)]
	  if !ok {
		  e.mu.Unlock()
		  return ErrBatchNotFound
	  }
	   batCopy := *bat
	   lineList := e.lines[batchID]
	   lineCopy := make([]AccrualLine, 0, len(lineList))
	   for _, l := range lineList {
		  lineCopy = append(lineCopy, *l)
	   }
	  e.mu.Unlock()
	   // 写批次记录
	   batBody, err := json.Marshal(batCopy)
	  if err != nil {
		  return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindAccrualBatch, batchID, batBody)
	   if err != nil {
		   return err
	  }
	   if existed {
		   if err := persist.Replace(ctx, kv, kindAccrualBatch, batchID, batBody); err != nil {
			   return err
		  }
	   }
	   // 写分录列表
	   linesBody, err := json.Marshal(lineCopy)
	  if err != nil {
		   return err
	  }
	   _, existed, err = persist.GetOrInit(ctx, kv, kindAccrualLines, batchID, linesBody)
	   if err != nil {
		  return err
	  }
	  if existed {
		   return persist.Replace(ctx, kv, kindAccrualLines, batchID, linesBody)
	   }
	  return nil
  }
