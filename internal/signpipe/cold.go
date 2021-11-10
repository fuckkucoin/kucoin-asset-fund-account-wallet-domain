package signpipe

import "sync/atomic"

// ColdRef 是签播闸能看到的单据事实。BatchID 为空时由幂等键或单据号反查批次。

type ColdRef struct {
	BatchID    string
	MovementID string
	Idem       string
	Chain      string
	To string
	Amount string
}

// ColdAllow 由冷签绑定引擎挂上。nil 表示本进程未启用批次仪式闸。
type ColdAllow func(ColdRef) error

type coldBox struct{ fn ColdAllow }

var coldAllow atomic.Pointer[coldBox]

// BindColdAllow 安装或卸下进程级签播闸。
  func BindColdAllow(fn ColdAllow) {
	   if fn == nil {
		  coldAllow.Store(nil)
		   return
	   }
	  coldAllow.Store(&coldBox{fn: fn})
  }

// AllowCold 执行进程级闸门。未挂载时返回 nil。
   func AllowCold(ref ColdRef) error {
	   box := coldAllow.Load()
	  if box == nil || box.fn == nil {
		   return nil
	  }
	  return box.fn(ref)
   }

// SetColdAllow 只作用于这一条管线，先于进程级闸门。
   func (p *Pipe) SetColdAllow(fn ColdAllow) {
	   if p == nil {
		   return
	   }
	   p.mu.Lock()
	  p.cold = fn
	  p.mu.Unlock()
  }

// AttachBatch 把出金批次号记到签章单上。不改变规范串，也不推进围栏。
   func (p *Pipe) AttachBatch(id, batchID string) error {
	   if p == nil {
		   return errPipe()
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  j, err := p.getLocked(id)
	  if err != nil {
		  return err
	   }
	   if j.State == StateSigned || j.State == StateCancelled || j.State == StateExpired {
		   return errPipeClosed()
	   }
	   j.BatchID = trim(batchID)
	   j.UpdatedAt = p.clock()
	  return nil
  }

   func (p *Pipe) rejectCold(j *Job) error {
	   if j == nil {
		  return nil
	   }
	   ref := ColdRef{
		   BatchID: j.BatchID, MovementID: j.MovementID, Chain: j.Chain, To: j.To, Amount: j.Amount,
	  }
	  if p != nil && p.cold != nil {
		  return p.cold(ref)
	   }
	  return AllowCold(ref)
   }

  func errPipe() error { return errText("sign pipeline unavailable") }

  func errPipeClosed() error { return errText("sign job closed") }

   func trim(s string) string {
	   i, j := 0, len(s)
	   for i < j && (s[i] == ' ' || s[i] == '\t') {
		   i++
	  }
	   for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		  j--
	   }
	   return s[i:j]
  }
