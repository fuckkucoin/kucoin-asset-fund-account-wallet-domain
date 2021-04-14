package dualctl

import "strings"

// Distinct 申请人、投票人和执行人不得收成同一人闭环。

   func Distinct(actors ...string) error {
	   seen := map[string]struct{}{}
	  n := 0
	   for _, a := range actors {
		  a = strings.TrimSpace(a)
		  if a == "" {
			  continue
		   }
		   if strings.EqualFold(a, "system") {
			   return ErrActor
		   }
		   k := strings.ToLower(a)
		   if _, ok := seen[k]; ok {
			  continue
		  }
		   seen[k] = struct{}{}
		   n++
	   }
	   if n < 2 {
		   return ErrActor
	  }
	  return nil
   }

// Gap 检查已就绪单：开单人不得单独执行；票数必须达到 Need。

  func (b *Book) Gap(id, execActor string) error {
	   r, err := b.Get(id)
	  if err != nil {
		   return err
	  }
	   if r.State != StateReady {
		  return ErrNotReady
	   }
	   if err := Distinct(r.OpenedBy, execActor); err != nil {
		  return err
	  }
	   if allowCount(r) < r.Need {
		  return ErrNotReady
	   }
	   return nil
   }
