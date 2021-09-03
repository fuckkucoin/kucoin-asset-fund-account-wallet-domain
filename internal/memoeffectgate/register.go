package memoeffectgate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   var book *fundsurf.Book
	   if rt != nil {
		   book = rt.FundSurf
	  }
	  var tickets TicketPort = NewMemTicketBook()
	  var credit CreditPort = NewMemCreditPort()
	  if book != nil {
		  tickets = AdaptFundSurfTickets(book)
		  credit = AdaptFundSurfCredit(book, nil)
	   }
	   eng := New(Config{
		  Store: NewMemStore(),
		   Mutex:   NewMemClaimMutex(),
		  Tickets: tickets,
		   Credit:  credit,
	   })
	  RegisterRoutes(v1, eng)
   }
