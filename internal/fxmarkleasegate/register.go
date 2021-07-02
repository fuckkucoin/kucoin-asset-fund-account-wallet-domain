package fxmarkleasegate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var book *fxmark.Book
	   if rt != nil {
		   if marks, ok := rt.FXMarks.(*fxmark.Book); ok {
			  book = marks
		  }
	  }
	   eng := New(Config{Store: NewMemStore(), Book: book, AutoBind: false})
	  RegisterRoutes(v1, eng)
  }
