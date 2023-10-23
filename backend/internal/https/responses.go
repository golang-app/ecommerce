package https

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func CORS(w http.ResponseWriter) {
	var allowedHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization,X-CSRF-Token, hx-request, hx-current-url"
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
		InternalError(w, "serialization-error", "cannot marshal response: "+err.Error())
		return
	}

	CORS(w)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func NoContent(w http.ResponseWriter) {
	CORS(w)
	w.WriteHeader(http.StatusNoContent)
}

type ErrorResponse struct {
	Type   string `json:"type,omitempty"`
	Detail string `json:"detail,omitempty"`
	Title  string `json:"title,omitempty"`
}

const errTypePrefix = "https://example.com/errors/"

func NotFound(w http.ResponseWriter, errType string, title string) {
	Error(w, ErrorResponse{
		Type:  errTypePrefix + errType,
		Title: title,
	}, http.StatusNotFound)
}

func InternalError(w http.ResponseWriter, errType string, title string) {
	Error(w, ErrorResponse{
		Type:  errTypePrefix + errType,
		Title: title,
	}, http.StatusInternalServerError)
}

func Unauthorized(w http.ResponseWriter, errType string, title string) {
	Error(w, ErrorResponse{
		Type:  errTypePrefix + errType,
		Title: title,
	}, http.StatusUnauthorized)
}

func BadRequest(w http.ResponseWriter, errType string, title string) {
	Error(w, ErrorResponse{
		Type:  errTypePrefix + errType,
		Title: title,
	}, http.StatusBadRequest)
}

func Error(w http.ResponseWriter, resp ErrorResponse, code int) {
	CORS(w)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(code)
	body, err := json.Marshal(resp)
	if err != nil {
		body = []byte(fmt.Sprintf("cannot marshal response: %s", err))
	}

	_, _ = w.Write(body)
}

func EmptyHandler(w http.ResponseWriter, r *http.Request) {
	CORS(w)
	w.WriteHeader(http.StatusOK)
}
