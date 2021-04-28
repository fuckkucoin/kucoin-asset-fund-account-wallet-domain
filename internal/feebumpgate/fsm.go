package feebumpgate

// AllowGenTransition 世代迁移是否合法。
   func AllowGenTransition(from, to GenState) bool {
	   if from == to {
		  return true
	  }
	  switch from {
	   case GenSigned:
		  return to == GenBroadcast || to == GenSuperseded || to == GenBlocked
	  case GenBroadcast:
		  return to == GenFinal || to == GenSuperseded || to == GenBlocked
	   case GenSuperseded, GenFinal, GenBlocked:
		  return false
	   default:
		   return false
	  }
  }

// AllowOccTransition 占用迁移是否合法。
  func AllowOccTransition(from, to OccState) bool {
	   if from == to {
		   return true
	  }
	  if from == OccHeld {
		   return to == OccRetired || to == OccOrphaned
	  }
	  return false
  }

// NextOnBroadcast signed → broadcast。
  func NextOnBroadcast(from GenState) (GenState, error) {
	  if !from.CanBroadcast() {
		  if from == GenSuperseded {
			  return from, ErrGenSuperseded
		   }
		   if from == GenBlocked {
			  return from, ErrGenBlocked
		   }
		   if from == GenFinal {
			   return from, ErrGenFinal
		  }
		  return from, ErrOldBroadcast
	   }
	  return GenBroadcast, nil
   }

// NextOnSupersede 新世代生效时旧世代去向。
   func NextOnSupersede(from GenState) (GenState, error) {
	   if !from.CanSupersede() {
		   if from.Terminal() {
			  return from, nil
		   }
		  return from, ErrInvalidTransition
	  }
	   return GenSuperseded, nil
   }

// NextOnRetire held → retired。

   func NextOnRetire(from OccState) (OccState, error) {
	  if from == OccRetired {
		   return OccRetired, nil
	   }
	  if from != OccHeld {
		  if from == OccOrphaned {
			   return from, ErrOccOrphaned
		   }
		   return from, ErrOccNotHeld
	  }
	   return OccRetired, nil
   }

// NextOnOrphan held → orphaned。
   func NextOnOrphan(from OccState) (OccState, error) {
	   if from == OccOrphaned {
		   return OccOrphaned, nil
	  }
	   if from != OccHeld {
		  return from, ErrOccNotHeld
	   }
	  return OccOrphaned, nil
  }

// TransitionTable 文档化合法边，供夹具。
   func TransitionTable() [][2]string {
	   return [][2]string{
		   {"signed", "broadcast"},
		  {"signed", "superseded"},
		   {"signed", "blocked"},
		  {"broadcast", "final"},
		  {"broadcast", "superseded"},
		  {"broadcast", "blocked"},
		   {"held", "retired"},
		   {"held", "orphaned"},
	  }
   }

// IllegalGenEdges 非法回退边。

   func IllegalGenEdges() [][2]GenState {
	   return [][2]GenState{
		  {GenFinal, GenSigned},
		  {GenFinal, GenBroadcast},
		   {GenSuperseded, GenSigned},
		   {GenSuperseded, GenBroadcast},
		  {GenBlocked, GenSigned},
		  {GenBlocked, GenBroadcast},
		  {GenBroadcast, GenSigned},
	   }
   }

// IllegalOccEdges 非法占用回退。

   func IllegalOccEdges() [][2]OccState {
	  return [][2]OccState{
		  {OccRetired, OccHeld},
		   {OccOrphaned, OccHeld},
		  {OccRetired, OccOrphaned},
		  {OccOrphaned, OccRetired},
	  }
  }
