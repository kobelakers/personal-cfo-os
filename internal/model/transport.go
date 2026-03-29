package model

import (
	"context"
	"net/http"
	"time"
)

// Transport is the replaceable provider transport seam. Chat models depend on
// this abstraction instead of binding business code to a single HTTP endpoint
// shape.
type Transport interface {
	Do(ctx context.Context, request TransportRequest) (TransportResponse, error)
}

type TransportRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

type TransportResponse struct {
	StatusCode int           `json:"status_code"`
	Headers    http.Header   `json:"headers,omitempty"`
	Body       []byte        `json:"body,omitempty"`
	Latency    time.Duration `json:"latency"`
}
