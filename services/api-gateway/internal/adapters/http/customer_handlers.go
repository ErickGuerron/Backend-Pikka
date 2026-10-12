package http

import (
	"log"
	"net/http"
)

// CustomerHandlers ...
type CustomerHandlers struct {
}

// NewCustomerHandlers ...
func NewCustomerHandlers() *CustomerHandlers {
	return &CustomerHandlers{}
}

// GetMe .
func (h *CustomerHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	log.Println("GetMe called")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"msg":"GetMe"}`))
}
