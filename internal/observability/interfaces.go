package observability

type TraceRef struct {
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id"`
}

type MetricKey struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}
