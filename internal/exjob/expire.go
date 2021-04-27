package exjob

import (
	"context"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ErrExpired 任务已过期或正文已回收。
var ErrExpired = errors.New("export job expired")

const (
	defaultJobTTL = 24 * time.Hour
	defaultBodyRetain = 72 * time.Hour
)

// ExpireAndReclaim 扫描内存任务：超时跃迁 expired，超保留清空正文。
   func (b *Book) ExpireAndReclaim(now time.Time, jobTTL, bodyRetain time.Duration) (expired, reclaimed int, err error) {
	  if b == nil {
		   return 0, 0, nil
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  if jobTTL <= 0 {
		   jobTTL = defaultJobTTL
	  }
	   if bodyRetain <= 0 {
		   bodyRetain = defaultBodyRetain
	  }
	   b.mu.Lock()
	  ids := make([]string, 0, len(b.jobs))
	   for id := range b.jobs {
		  ids = append(ids, id)
	   }
	   b.mu.Unlock()
	  for _, id := range ids {
		   j, gerr := b.Get(id)
		  if gerr != nil {
			   continue
		   }
		  age := now.Sub(j.CreatedAt.UTC())
		  switch j.Status {
		   case StatusQueued, StatusRunning:
			  if age < jobTTL {
				   continue
			  }
			   st,
				   aerr := AdvanceStatus(j.Status, StatusExpired)
			   if aerr != nil {
				  return expired, reclaimed, aerr
			   }
			  j.Status = st
			  j.Body = ""
			   j.ExpiredAt = now
			   if serr := b.save(j, persist.Fence{}); serr != nil {
				  return expired, reclaimed, serr
			  }
			  expired++
		  case StatusDone, StatusFailed:
			  if age < bodyRetain || j.Body == "" {
				   continue
			   }
			  j.Body = ""
			  if serr := b.save(j, persist.Fence{}); serr != nil {
				  return expired, reclaimed, serr
			  }
			   reclaimed++
		  }
	   }
	  if b.sql != nil {
		  if _, merr := b.sql.MarkExpired(context.Background(), now.Add(-jobTTL)); merr != nil {
			   return expired, reclaimed, merr
		   }
	   }
	  return expired, reclaimed, nil
  }

// EnsureFresh 读取前检查：已 expired 或正文已回收则拒绝交付。
   func (b *Book) EnsureFresh(j Job, now time.Time) error {
	  if j.Status == StatusExpired {
		   return ErrExpired
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  if (j.Status == StatusQueued || j.Status == StatusRunning) && !j.CreatedAt.IsZero() && now.Sub(j.CreatedAt.UTC()) > defaultJobTTL {
		   return ErrExpired
	  }
	   if j.Status == StatusDone && j.Body == "" && j.Checksum != "" {
		  return ErrExpired
	   }
	   return nil
   }
