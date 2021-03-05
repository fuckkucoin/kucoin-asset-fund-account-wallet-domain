package breachclosegate

// breachEvent BreachLease 状态机事件。
type breachEvent string

const (
	// EventExpireCase 关联案件已被强制 Expire。
	EventExpireCase breachEvent = "expire_case"
	// EventApproveClr 四眼清障批准。
	EventApproveClr breachEvent = "approve_clearance"
)

// fsmGraph 合法状态转换表。
var fsmGraph = map[BreachState]map[breachEvent]BreachState{
	BreachStateBlocking: {
		EventExpireCase: BreachStateExpiredCase,
		EventApproveClr: BreachStateCleared,
	},
	// 终态无出弧。
}

// applyBreachEvent 执行状态转换，返回新状态。非法转换返回 ErrIllegalTransition。

   func applyBreachEvent(cur BreachState, ev breachEvent) (BreachState, error) {
	   edges, ok := fsmGraph[cur]
	   if !ok {
		  return cur, ErrIllegalTransition
	  }
	  next, ok := edges[ev]
	   if !ok {
		   return cur, ErrIllegalTransition
	   }
	   return next, nil
   }
