package https

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func cors(w http.ResponseWriter) {
	var allowedHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization,X-CSRF-Token"
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
	w.Header().Set("Access-Control-Expose-Headers", "Authorization")
}

func OK(w http.ResponseWriter, msg interface{}) {
	resp := struct {
		Data interface{} `json:"data"`
	}{
		Data: msg,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		Error(w, "cannot marshal request object", http.StatusInternalServerError)
		return
	}

	cors(w)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func NoContent(w http.ResponseWriter) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func NotFound(w http.ResponseWriter, msg string) {
	Error(w, msg, http.StatusNotFound)
}

func InternalError(w http.ResponseWriter, msg string) {
	Error(w, msg, http.StatusInternalServerError)
}

func BadRequest(w http.ResponseWriter, msg string) {
	Error(w, msg, http.StatusBadRequest)
}

func Error(w http.ResponseWriter, msg string, code int) {
	cors(w)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(code)
	resp := struct {
		Message string `json:"message"`
	}{
		Message: msg,
	}
	body, err := json.Marshal(resp)
	if err != nil {
		body = []byte(fmt.Sprintf("cannot marshal response: %s", err))
	}

	_, _ = w.Write(body)
}
