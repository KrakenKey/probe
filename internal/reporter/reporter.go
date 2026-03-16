package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Reporter struct {
	apiURL     string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

func New(apiURL, apiKey, version, goos, goarch string) *Reporter {
	return &Reporter{
		apiURL: apiURL,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		userAgent: fmt.Sprintf("krakenkey-probe/%s (%s/%s)", version, goos, goarch),
	}
}

func (r *Reporter) Register(ctx context.Context, probe ProbeInfo) error {
	return r.post(ctx, "/probes/register", probe)
}

func (r *Reporter) Report(ctx context.Context, report ScanReport) error {
	return r.post(ctx, "/probes/report", report)
}

// FetchHostedConfig retrieves endpoint configuration for a hosted probe.
func (r *Reporter) FetchHostedConfig(ctx context.Context, probeID string) (*HostedConfig, error) {
	url := r.apiURL + "/probes/" + probeID + "/config"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	r.setHeaders(req)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching hosted config: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, fmt.Errorf("fetching hosted config: %w", err)
	}

	var cfg HostedConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding hosted config: %w", err)
	}
	return &cfg, nil
}

func (r *Reporter) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(5 * time.Second)
		}

		lastErr = r.doPost(ctx, path, body)
		if lastErr == nil {
			return nil
		}

		// Only retry on 5xx errors
		if apiErr, ok := lastErr.(*APIError); ok {
			if apiErr.StatusCode >= 500 {
				continue
			}
			return lastErr
		}
		// Network errors: retry once
	}
	return lastErr
}

func (r *Reporter) doPost(ctx context.Context, path string, body []byte) error {
	url := r.apiURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	r.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	return checkResponse(resp)
}

func (r *Reporter) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("User-Agent", r.userAgent)
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
	}

	if resp.StatusCode == 429 {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				apiErr.RetryAfter = time.Duration(secs) * time.Second
			}
		}
	}

	return apiErr
}

// APIError represents a non-2xx response from the KrakenKey API.
type APIError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.StatusCode {
	case 401:
		return "API authentication failed (HTTP 401): check your API key"
	case 429:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("API rate limited (HTTP 429): retry after %s", e.RetryAfter)
		}
		return "API rate limited (HTTP 429)"
	default:
		return fmt.Sprintf("API error (HTTP %d): %s", e.StatusCode, e.Body)
	}
}
