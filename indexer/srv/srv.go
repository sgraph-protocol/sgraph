// package srv implements basic JSONRPC 2.0 server
package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type RpcRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

type RpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RpcError       `json:"error,omitempty"`
	ID      any             `json:"id,omitempty"`
}

type RpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type HandlerFunc func(context.Context, json.RawMessage) (json.RawMessage, error)

func WrapH[REQ, RES any](h func(ctx context.Context, r REQ) (RES, error)) HandlerFunc {
	return func(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
		req := new(REQ)
		if err := json.Unmarshal(in, req); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		resp, err := h(ctx, *req)
		if err != nil {
			return nil, fmt.Errorf("err: %w", err)
		}
		return json.Marshal(resp)
	}
}

type Server struct {
	handlers map[string]HandlerFunc
}

func NewServer() Server {
	return Server{handlers: make(map[string]HandlerFunc)}
}

func (s Server) Register(method string, h HandlerFunc) {
	s.handlers[strings.ToLower(method)] = h
}

func (s Server) Run(ctx context.Context) error {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req RpcRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.renderError(w, http.StatusBadRequest, RpcError{
				Code:    -32700,
				Message: "Parse error",
			})
			return
		}

		resp := s.process(r.Context(), req)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			s.renderError(w, http.StatusInternalServerError, RpcError{
				Code:    -32000,
				Message: "internal server error",
			})
			return
		}
	})

	return http.ListenAndServe(":8080", h)
}

func (s Server) process(ctx context.Context, req RpcRequest) RpcResponse {
	handler, found := s.handlers[strings.ToLower(req.Method)]
	if !found {
		return RpcResponse{
			Jsonrpc: "2.0",
			Error: &RpcError{
				Code:    -32601,
				Message: "method not found",
			},
		}
	}

	response, err := handler(ctx, req.Params)
	if err != nil {
		return RpcResponse{
			Jsonrpc: "2.0",
			Error: &RpcError{
				Code:    -32000,
				Message: err.Error(),
			},
		}
	}

	return RpcResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result:  response,
	}
}

func (s Server) renderError(w http.ResponseWriter, status int, err RpcError) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(err); err != nil {
		w.Write([]byte("internal error"))
	}
}
