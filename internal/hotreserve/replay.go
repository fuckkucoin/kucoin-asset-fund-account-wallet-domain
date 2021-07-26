package hotreserve

// Replay 按序号重放分录，得到应有状态。
// 做什么：确认开立、圈存、兑付、释放的科目顺序没有断。
// 冻结备忘必须跟在圈存或释放之后，不能单独出现。
// 不变量：回放状态等于预留当前状态，且每笔金额等于预留本金。
   func Replay(amount string, rows []ReservationJournal) (State, error) {
	   if _, err := parsePos(amount); err != nil {
		  return "", err
	  }
	  var st State
	  needOff := false
	  for _, j := range rows {
		  if j.Amount != amount {
			  return "", ErrState
		   }
		   if err := subjects(j.Kind, j.Debit, j.Credit); err != nil {
			   return "", err
		  }
		   switch j.Kind {
		  case KindOpen:
			  if st != "" {
				   return "", ErrState
			   }
			  st = StateOpen
		  case KindEarmark:
			  if st != StateOpen {
				   return "", ErrState
			  }
			  st = StateEarmarked
		  case KindEarmarkFreeze:
			   if st != StateEarmarked {
				  return "", ErrState
			  }
		   case KindConsume:
			  if st != StateEarmarked {
				  return "", ErrState
			   }
			   st = StateConsumed
		   case KindConsumeFreeze:
			  if st != StateConsumed {
				  return "", ErrState
			   }
			  needOff = false
		  case KindExpire:
			   if st != StateOpen {
				   return "", ErrState
			   }
			  st = StateExpired
		  case KindCancel, KindForce:
			  if st != StateOpen && st != StateEarmarked {
				   return "", ErrState
			  }
			   needOff = st == StateEarmarked
			  if j.Kind == KindCancel || j.Kind == KindForce {
				  if st == StateEarmarked && j.Credit != SubEarmarked {
					  return "", ErrState
				  }
				  if st == StateOpen && j.Credit != SubOpen {
					   return "", ErrState
				  }
			  }
			   st = StateCancelled
		  case KindCancelFreeze, KindForceFreeze:
			   if st != StateCancelled || !needOff {
				  return "", ErrState
			   }
			   needOff = false
		  default:
			  return "", ErrState
		   }
	   }
	   if needOff {
		  return "", ErrState
	   }
	  if st == StateEarmarked && !sawKind(rows, KindEarmarkFreeze) {
		  return "", ErrState
	  }
	   if st == StateConsumed && (!sawKind(rows, KindEarmarkFreeze) || !sawKind(rows, KindConsumeFreeze)) {
		  return "", ErrState
	   }
	  return st, nil
  }

  func sawKind(rows []ReservationJournal, kind string) bool {
	   for _, j := range rows {
		  if j.Kind == kind {
			   return true
		   }
	   }
	   return false
  }
