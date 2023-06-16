package utxograph

// TicketState 工单可接受的状态。

const (
	TicketOpen = "open"
	TicketLabeled = "labeled"
	TicketHeld    = "held"
	TicketClosed  = "closed"
)

// HoldTicket 把 open 工单改成 held。关闭不等于入账，调用方仍不能据此贷记。
  func HoldTicket(t Ticket, note string) (Ticket, bool) {
	  if t.State != TicketOpen {
		  return t, false
	  }
	   t.State = TicketHeld
	   if note != "" {
		   t.Note = note
	   }
	  return t, true
  }

// CloseTicket 结案。kind 仍保留，避免结案后看不出是未绑定还是守恒。
  func CloseTicket(t Ticket, note string) (Ticket, bool) {
	   if t.State == TicketClosed {
		  return t, false
	  }
	  t.State = TicketClosed
	   if note != "" {
		  t.Note = note
	   }
	   return t, true
   }

// BlocksCredit 这类工单挡住对应输出的入账意图。
  func BlocksCredit(t Ticket) bool {
	  switch t.Kind {
	  case "unbound", "conserve", "merge", "unknown_script", "change":
		   return t.State != TicketClosed || t.Kind == "change" || t.Kind == "conserve" || t.Kind == "merge"
	   default:
		  return true
	  }
  }
