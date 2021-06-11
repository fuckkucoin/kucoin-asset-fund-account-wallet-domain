package fundmat

// CanStep 判断重建状态迁移。queued 只能进入 running；running 只能进入 done 或 failed。
   func CanStep(from, to JobStatus) error {
	  if !from.Valid() || !to.Valid() {
		   return ErrTransition
	   }
	  switch from {
	   case JobQueued:
		   if to == JobRunning {
			   return nil
		  }
	  case JobRunning:
		   if to == JobDone || to == JobFailed {
			   return nil
		   }
	   }
	   return ErrTransition
   }

// Step 在迁移合法时返回目标状态。调用方仍须用仓储的条件更新落盘。
  func Step(from, to JobStatus) (JobStatus, error) {
	   if err := CanStep(from, to); err != nil {
		   return from, err
	   }
	   return to, nil
   }
