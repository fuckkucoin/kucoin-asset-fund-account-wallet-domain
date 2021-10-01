package portfolio

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/account"
)

type Book = account.Book

const (
	BookFunding = account.BookFunding
	BookTrading = account.BookTrading
	BookMargin   = account.BookMargin
	BookIsolated = account.BookIsolated
	BookFutures = account.BookFutures
	BookEarn = account.BookEarn
)

  func ParseBook(s string) (Book, error) {
	   b, err := account.ParseBook(s)
	   if err != nil {
		  return "", ErrBook
	  }
	   return b, nil
   }

   func AllBooks() []Book { return account.AllBooks() }

   func SubLedgers() []Book {
	   return []Book{BookFunding, BookTrading, BookEarn, BookMargin, BookIsolated, BookFutures}
  }

   func bookKey(userID string, book Book, asset string) string {
	   return strings.TrimSpace(userID) + "|" + string(book) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }
