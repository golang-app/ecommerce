package https

import (
	"net/http"

	"log"
)

func WrapPanic(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				log.Printf("Panic: %v", r)
			}
		}()

		fn(w, r)
	}
}
