package model

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPTransport struct {
	Client *http.Client
}

func (t HTTPTransport) Do(ctx context.Context, request TransportRequest) (TransportResponse, error) {
	client := t.Client
	if client == nil {
		client = &http.Client{}
	}
	req, err := http.NewRequestWithContext(ctx, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return TransportResponse{}, err
	}
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}
	start := time.Now().UTC()
	resp, err := client.Do(req)
	if err != nil {
		return TransportResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TransportResponse{}, err
	}
	return TransportResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		Latency:    time.Since(start),
	}, nil
}

func resolveModelName(request ModelRequest, reasoningModel string, fastModel string) (string, error) {
	if request.Model != "" {
		return request.Model, nil
	}
	switch request.Profile {
	case ModelProfilePlannerReasoning:
		if reasoningModel == "" {
			return "", fmt.Errorf("reasoning model is not configured")
		}
		return reasoningModel, nil
	case ModelProfileCashflowFast:
		if fastModel == "" {
			return "", fmt.Errorf("fast model is not configured")
		}
		return fastModel, nil
	default:
		if fastModel != "" {
			return fastModel, nil
		}
		if reasoningModel != "" {
			return reasoningModel, nil
		}
		return "", fmt.Errorf("model is not configured for profile %q", request.Profile)
	}
}

func withRequestTimeout(ctx context.Context, policy TimeoutPolicy) (context.Context, context.CancelFunc) {
	timeout := policy.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultTimeoutPolicy().RequestTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
