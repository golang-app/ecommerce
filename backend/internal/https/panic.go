package https

import (
	"net/http"
	"runtime/debug"

	"log"
)

func WrapPanic(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				log.Printf("Panic: %v\nStack trace: %+v", r, string(debug.Stack()))
			}
		}()

		fn(w, r)
	}
}
