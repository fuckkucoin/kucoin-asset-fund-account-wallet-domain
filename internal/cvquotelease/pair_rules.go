package cvquotelease

import "strings"

var allowedBooks = map[string]struct{}{
	"funding": {},
	"trading": {},
	"both": {},
}

   func BookAllowed(book string) bool {
	  book, err := NormalizeBook(book)
	  if err != nil {
		  return false
	   }
	   _, ok := allowedBooks[book]
	   return ok
  }

   func PairAllowed(from, to string) bool {
	   from = NormalizeAsset(from)
	   to = NormalizeAsset(to)
	   if from == "" || to == "" || from == to {
		   return false
	   }
	   return true
  }

   func SourceName(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return "mark_put"
	   }
	   return s
   }
