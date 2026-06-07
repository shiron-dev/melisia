package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
}

type Pool struct {
	Name      string
	Path      string
	Status    string
	Healthy   bool
	Size      int64
	Available int64
}

type Dataset struct {
	ID            string
	Name          string
	Type          string
	Atime         string
	Compression   string
	Deduplication string
	Exec          string
	Readonly      string
	Recordsize    string
	Sync          string
}

var (
	createDatasetParentRetryAttempts = 20
	createDatasetParentRetryDelay    = 500 * time.Millisecond
)

type apiError struct {
	Method     string
	URL        string
	Status     string
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s returned %s: %s", e.Method, e.URL, e.Status, e.Body)
}

func New(baseURL, apiKey string, tlsInsecureSkipVerify bool) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base_url must not be empty")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("api_key must not be empty")
	}

	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base_url must include scheme and host")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsInsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &Client{
		baseURL: parsed,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}, nil
}

func (c *Client) GetPool(ctx context.Context, name string) (Pool, error) {
	var pools []map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2.0/pool", nil, &pools); err != nil {
		return Pool{}, err
	}

	for _, raw := range pools {
		if stringValue(raw["name"]) == name {
			return Pool{
				Name:      stringValue(raw["name"]),
				Path:      stringValue(raw["path"]),
				Status:    stringValue(raw["status"]),
				Healthy:   poolIsHealthy(raw),
				Size:      int64Value(raw["size"]),
				Available: int64Value(raw["free"]),
			}, nil
		}
	}

	return Pool{}, fmt.Errorf("pool %q was not found", name)
}

func (c *Client) GetDataset(ctx context.Context, id string) (Dataset, error) {
	var raw map[string]any
	if err := c.doDatasetID(ctx, http.MethodGet, id, nil, &raw); err != nil {
		return Dataset{}, err
	}

	return datasetFromAPI(raw), nil
}

func (c *Client) CreateDataset(ctx context.Context, dataset Dataset) (Dataset, error) {
	body := map[string]any{
		"name":          dataset.Name,
		"type":          dataset.Type,
		"atime":         dataset.Atime,
		"compression":   dataset.Compression,
		"deduplication": dataset.Deduplication,
		"exec":          dataset.Exec,
		"readonly":      dataset.Readonly,
		"recordsize":    dataset.Recordsize,
		"sync":          dataset.Sync,
	}

	var raw map[string]any
	for attempt := 0; ; attempt++ {
		if err := c.do(ctx, http.MethodPost, "/api/v2.0/pool/dataset", body, &raw); err != nil {
			if !isParentDatasetMissing(err) || attempt >= createDatasetParentRetryAttempts {
				return Dataset{}, err
			}

			timer := time.NewTimer(createDatasetParentRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Dataset{}, ctx.Err()
			case <-timer.C:
				continue
			}
		}

		break
	}

	if len(raw) == 0 {
		return c.GetDataset(ctx, dataset.Name)
	}

	return datasetFromAPI(raw), nil
}

func (c *Client) UpdateDataset(ctx context.Context, dataset Dataset) (Dataset, error) {
	body := map[string]any{
		"atime":         dataset.Atime,
		"compression":   dataset.Compression,
		"deduplication": dataset.Deduplication,
		"exec":          dataset.Exec,
		"readonly":      dataset.Readonly,
		"recordsize":    dataset.Recordsize,
		"sync":          dataset.Sync,
	}

	var raw map[string]any
	if err := c.doDatasetID(ctx, http.MethodPut, dataset.ID, body, &raw); err != nil {
		return Dataset{}, err
	}

	if len(raw) == 0 {
		return c.GetDataset(ctx, dataset.ID)
	}

	return datasetFromAPI(raw), nil
}

func (c *Client) DeleteDataset(ctx context.Context, id string) error {
	return c.doDatasetID(ctx, http.MethodDelete, id, nil, nil)
}

func (c *Client) doDatasetID(ctx context.Context, method, id string, body any, out any) error {
	return c.doEscaped(ctx, method, "/api/v2.0/pool/dataset/id/"+id, "/api/v2.0/pool/dataset/id/"+url.PathEscape(id), body, out)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	return c.doEscaped(ctx, method, path, "", body, out)
}

func (c *Client) doEscaped(ctx context.Context, method, path, rawPath string, body any, out any) error {
	requestURL := c.requestURL(path, rawPath)

	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), requestBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, requestURL, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{
			Method:     method,
			URL:        requestURL.String(),
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(responseBody)),
		}
	}

	if out == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (c *Client) requestURL(path, rawPath string) *url.URL {
	return c.baseURL.ResolveReference(&url.URL{Path: path, RawPath: rawPath})
}

func datasetFromAPI(raw map[string]any) Dataset {
	name := stringValue(raw["name"])
	if name == "" {
		name = stringValue(raw["id"])
	}

	return Dataset{
		ID:            name,
		Name:          name,
		Type:          propertyString(raw["type"]),
		Atime:         propertyString(raw["atime"]),
		Compression:   propertyString(raw["compression"]),
		Deduplication: propertyString(raw["deduplication"]),
		Exec:          propertyString(raw["exec"]),
		Readonly:      propertyString(raw["readonly"]),
		Recordsize:    recordsizeString(raw["recordsize"]),
		Sync:          propertyString(raw["sync"]),
	}
}

func isParentDatasetMissing(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.StatusCode == http.StatusUnprocessableEntity &&
		strings.Contains(apiErr.Body, "Parent dataset") &&
		strings.Contains(apiErr.Body, "does not exist")
}

func poolIsHealthy(raw map[string]any) bool {
	if healthy, ok := raw["healthy"].(bool); ok {
		return healthy
	}

	status := strings.ToUpper(stringValue(raw["status"]))
	return status == "ONLINE" || status == "HEALTHY"
}

func propertyString(value any) string {
	if property, ok := value.(map[string]any); ok {
		if parsed := stringValue(property["parsed"]); parsed != "" {
			return strings.ToUpper(parsed)
		}
		if raw := stringValue(property["rawvalue"]); raw != "" {
			return strings.ToUpper(raw)
		}
		if value := stringValue(property["value"]); value != "" {
			return strings.ToUpper(value)
		}
	}

	return strings.ToUpper(stringValue(value))
}

func recordsizeString(value any) string {
	valueString := propertyString(value)

	switch valueString {
	case "131072":
		return "128K"
	default:
		return valueString
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return ""
	}
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
	}
}
