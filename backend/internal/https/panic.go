package https

import (
	"net/http"
	"runtime/debug"

	"github.com/sirupsen/logrus"
)

func WrapPanic(fn http.HandlerFunc, logger logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				logger.WithFields(logrus.Fields{
					"panic": rec,
					"stack": string(debug.Stack()),
				}).Error("panic recovered in HTTP handler")
			}
		}()

		fn(w, r)
	}
}
