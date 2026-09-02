// Package observability implements the gateway's standard error envelope
// (specs/001-handler-injection/contracts/error-format.md) and structured handler-execution
// logging (FR-008), used identically regardless of which handler or phase is involved
// (Constitution Principle III: User Experience Consistency).
package observability

import (
	"encoding/json"
	"net/http"
)

// Error codes from contracts/error-format.md, with their fixed HTTP status.
const (
	CodeRouteNotFound           = "ROUTE_NOT_FOUND"
	CodeHandlerRejectedRequest  = "HANDLER_REJECTED_REQUEST"
	CodeHandlerRejectedResponse = "HANDLER_REJECTED_RESPONSE"
	CodeHandlerTimeout          = "HANDLER_TIMEOUT"
	CodeHandlerError            = "HANDLER_ERROR"
)

var statusByCode = map[string]int{
	CodeRouteNotFound:           http.StatusNotFound,
	CodeHandlerRejectedRequest:  http.StatusBadRequest,
	CodeHandlerRejectedResponse: http.StatusBadGateway,
	CodeHandlerTimeout:          http.StatusGatewayTimeout,
	CodeHandlerError:            http.StatusBadGateway,
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Route   string `json:"route,omitempty"`
	Handler string `json:"handler,omitempty"`
}

// WriteError renders the standard error envelope for the given code/message and writes it
// (with the code's fixed HTTP status) to w. Every handler rejection, handler failure, or
// routing miss in this feature MUST go through this single function so every response returns
// an identical shape (FR-007, Constitution Principle III). route and/or handlerName may be ""
// to omit them — e.g. ROUTE_NOT_FOUND has neither, since no route matched (contracts/error-
// format.md).
func WriteError(w http.ResponseWriter, code, message, route, handlerName string) {
	status, ok := statusByCode[code]
	if !ok {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{
		Code:    code,
		Message: message,
		Route:   route,
		Handler: handlerName,
	}})
}
