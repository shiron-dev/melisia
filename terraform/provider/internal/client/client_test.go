package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		wantErr string
	}{
		{
			name:    "missing base url",
			baseURL: "",
			apiKey:  "test-key",
			wantErr: "base_url must not be empty",
		},
		{
			name:    "missing api key",
			baseURL: "https://truenas.example.test",
			apiKey:  "",
			wantErr: "api_key must not be empty",
		},
		{
			name:    "relative base url",
			baseURL: "truenas.example.test",
			apiKey:  "test-key",
			wantErr: "base_url must include scheme and host",
		},
		{
			name:    "invalid base url",
			baseURL: "http://[::1",
			apiKey:  "test-key",
			wantErr: "parse base_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.baseURL, tt.apiKey, false)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewConfiguresTLSInsecureSkipVerify(t *testing.T) {
	client, err := New("https://truenas.example.test", "test-key", true)
	if err != nil {
		t.Fatal(err)
	}

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("got transport %T, want *http.Transport", client.httpClient.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected TLS InsecureSkipVerify to be enabled")
	}
}

func TestRequestsSetAuthAndContentHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("got Accept %q, want application/json", r.Header.Get("Accept"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("got Authorization %q, want bearer API key", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("got Content-Type %q, want application/json", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"apps/apps","type":{"parsed":"FILESYSTEM"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.CreateDataset(context.Background(), Dataset{
		Name: "apps/apps",
		Type: "FILESYSTEM",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetPoolFindsPoolAndNormalizesHealth(t *testing.T) {
	tests := []struct {
		name    string
		rawPool string
		want    Pool
	}{
		{
			name: "healthy boolean wins",
			rawPool: `{
				"name":"apps",
				"path":"/mnt/apps",
				"status":"DEGRADED",
				"healthy":true,
				"size":8053063680,
				"free":4026531840
			}`,
			want: Pool{
				Name:      "apps",
				Path:      "/mnt/apps",
				Status:    "DEGRADED",
				Healthy:   true,
				Size:      8053063680,
				Available: 4026531840,
			},
		},
		{
			name: "online status is healthy fallback",
			rawPool: `{
				"name":"tank",
				"path":"/mnt/tank",
				"status":"ONLINE",
				"size":8053063680,
				"free":1024
			}`,
			want: Pool{
				Name:      "tank",
				Path:      "/mnt/tank",
				Status:    "ONLINE",
				Healthy:   true,
				Size:      8053063680,
				Available: 1024,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requireMethodPath(t, r, http.MethodGet, "/api/v2.0/pool")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("[" + tt.rawPool + `,{"name":"other"}]`))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			got, err := client.GetPool(context.Background(), tt.want.Name)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestGetPoolReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"apps"}]`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetPool(context.Background(), "tank")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `pool "tank" was not found`) {
		t.Fatalf("got error %q", err.Error())
	}
}

func TestDatasetIDRequestsEscapeSlash(t *testing.T) {
	client, err := New("https://truenas.example.test", "test-key", false)
	if err != nil {
		t.Fatal(err)
	}

	requestURL := client.requestURL(
		"/api/v2.0/pool/dataset/id/apps/apps",
		"/api/v2.0/pool/dataset/id/apps%2Fapps",
	)

	wantPath := "/api/v2.0/pool/dataset/id/apps%2Fapps"
	if requestURL.EscapedPath() != wantPath {
		t.Fatalf("got path %q, want %q", requestURL.EscapedPath(), wantPath)
	}
}

func TestGetDatasetEscapesIDAndNormalizesProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireMethodPath(t, r, http.MethodGet, "/api/v2.0/pool/dataset/id/apps/apps/nextcloud")
		if r.URL.EscapedPath() != "/api/v2.0/pool/dataset/id/apps%2Fapps%2Fnextcloud" {
			t.Fatalf("got escaped path %q", r.URL.EscapedPath())
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(datasetJSON("apps/apps/nextcloud")))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.GetDataset(context.Background(), "apps/apps/nextcloud")
	if err != nil {
		t.Fatal(err)
	}

	want := Dataset{
		ID:            "apps/apps/nextcloud",
		Name:          "apps/apps/nextcloud",
		Type:          "FILESYSTEM",
		Atime:         "ON",
		Compression:   "LZ4",
		Copies:        1,
		Deduplication: "OFF",
		Exec:          "ON",
		Readonly:      "OFF",
		Recordsize:    "128K",
		Snapdir:       "HIDDEN",
		Sync:          "STANDARD",
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCreateDatasetSendsExpectedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireMethodPath(t, r, http.MethodPost, "/api/v2.0/pool/dataset")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		assertBodyValue(t, body, "name", "apps/apps")
		assertBodyValue(t, body, "type", "FILESYSTEM")
		assertBodyValue(t, body, "atime", "ON")
		assertBodyValue(t, body, "compression", "LZ4")
		assertBodyValue(t, body, "copies", float64(1))
		assertBodyValue(t, body, "deduplication", "OFF")
		assertBodyValue(t, body, "exec", "ON")
		assertBodyValue(t, body, "readonly", "OFF")
		assertBodyValue(t, body, "recordsize", "128K")
		assertBodyValue(t, body, "snapdir", "HIDDEN")
		assertBodyValue(t, body, "sync", "STANDARD")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(datasetJSON("apps/apps")))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.CreateDataset(context.Background(), testDataset("apps/apps"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "apps/apps" {
		t.Fatalf("got name %q, want apps/apps", got.Name)
	}
}

func TestCreateDatasetFallsBackToReadWhenResponseIsEmpty(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			requireMethodPath(t, r, http.MethodPost, "/api/v2.0/pool/dataset")
			w.WriteHeader(http.StatusNoContent)
		case 2:
			requireMethodPath(t, r, http.MethodGet, "/api/v2.0/pool/dataset/id/apps/apps")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(datasetJSON("apps/apps")))
		default:
			t.Fatalf("unexpected request %d %s", requests, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.CreateDataset(context.Background(), testDataset("apps/apps"))
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
	if got.ID != "apps/apps" {
		t.Fatalf("got ID %q, want apps/apps", got.ID)
	}
}

func TestCreateDatasetRetriesWhenParentDatasetIsNotReady(t *testing.T) {
	restoreRetrySettings := setRetrySettings(t, 2, time.Millisecond)
	defer restoreRetrySettings()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireMethodPath(t, r, http.MethodPost, "/api/v2.0/pool/dataset")

		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"pool_dataset_create.name":[{"message":"Parent dataset (apps/apps) does not exist.","errno":22}]}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(datasetJSON("apps/apps/nextcloud")))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	dataset, err := client.CreateDataset(context.Background(), testDataset("apps/apps/nextcloud"))
	if err != nil {
		t.Fatal(err)
	}

	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
	if dataset.Name != "apps/apps/nextcloud" {
		t.Fatalf("got dataset name %q, want apps/apps/nextcloud", dataset.Name)
	}
}

func TestCreateDatasetStopsRetryingAfterAttempts(t *testing.T) {
	restoreRetrySettings := setRetrySettings(t, 1, time.Millisecond)
	defer restoreRetrySettings()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"pool_dataset_create.name":[{"message":"Parent dataset (apps/apps) does not exist.","errno":22}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateDataset(context.Background(), testDataset("apps/apps/nextcloud"))
	if err == nil {
		t.Fatal("expected error")
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want initial request plus one retry", requests)
	}
	if !strings.Contains(err.Error(), "Parent dataset") {
		t.Fatalf("got error %q", err.Error())
	}
}

func TestCreateDatasetDoesNotRetryUnrelatedUnprocessableEntity(t *testing.T) {
	restoreRetrySettings := setRetrySettings(t, 10, time.Millisecond)
	defer restoreRetrySettings()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"pool_dataset_create.name":[{"message":"Dataset name is invalid.","errno":22}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateDataset(context.Background(), testDataset("apps/apps"))
	if err == nil {
		t.Fatal("expected error")
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
}

func TestCreateDatasetReturnsContextCancellationDuringRetry(t *testing.T) {
	restoreRetrySettings := setRetrySettings(t, 10, time.Hour)
	defer restoreRetrySettings()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"pool_dataset_create.name":[{"message":"Parent dataset (apps/apps) does not exist.","errno":22}]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newTestClient(t, server.URL)
	_, err := client.CreateDataset(ctx, testDataset("apps/apps/nextcloud"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want context.Canceled", err)
	}
}

func TestUpdateDatasetSendsOnlyMutablePropertiesAndFallsBackToRead(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			requireMethodPath(t, r, http.MethodPut, "/api/v2.0/pool/dataset/id/apps/apps")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, ok := body["name"]; ok {
				t.Fatal("update body must not include name")
			}
			if _, ok := body["type"]; ok {
				t.Fatal("update body must not include type")
			}
			assertBodyValue(t, body, "compression", "LZ4")
			assertBodyValue(t, body, "copies", float64(1))
			assertBodyValue(t, body, "snapdir", "HIDDEN")
			w.WriteHeader(http.StatusNoContent)
		case 2:
			requireMethodPath(t, r, http.MethodGet, "/api/v2.0/pool/dataset/id/apps/apps")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(datasetJSON("apps/apps")))
		default:
			t.Fatalf("unexpected request %d %s", requests, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.UpdateDataset(context.Background(), testDataset("apps/apps"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "apps/apps" {
		t.Fatalf("got ID %q, want apps/apps", got.ID)
	}
}

func TestDeleteDatasetUsesEscapedID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireMethodPath(t, r, http.MethodDelete, "/api/v2.0/pool/dataset/id/tank/users/shiron")
		if r.URL.EscapedPath() != "/api/v2.0/pool/dataset/id/tank%2Fusers%2Fshiron" {
			t.Fatalf("got escaped path %q", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.DeleteDataset(context.Background(), "tank/users/shiron"); err != nil {
		t.Fatal(err)
	}
}

func TestDoReturnsAPIErrorWithStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Invalid API key"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetPool(context.Background(), "apps")
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got error %T, want *apiError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Body != "Invalid API key" {
		t.Fatalf("got body %q, want Invalid API key", apiErr.Body)
	}
}

func TestDoReturnsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetPool(context.Background(), "apps")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decode response body") {
		t.Fatalf("got error %q", err.Error())
	}
}

func TestDatasetFromAPIFallsBackToID(t *testing.T) {
	dataset := datasetFromAPI(map[string]any{
		"id":         "tank/users",
		"recordsize": map[string]any{"parsed": "131072"},
		"copies":     map[string]any{"parsed": float64(1)},
	})

	if dataset.ID != "tank/users" {
		t.Fatalf("got ID %q, want tank/users", dataset.ID)
	}
	if dataset.Name != "tank/users" {
		t.Fatalf("got name %q, want tank/users", dataset.Name)
	}
	if dataset.Recordsize != "128K" {
		t.Fatalf("got recordsize %q, want 128K", dataset.Recordsize)
	}
	if dataset.Copies != 1 {
		t.Fatalf("got copies %d, want 1", dataset.Copies)
	}
}

func TestPropertyStringPriorityAndNormalization(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "parsed wins",
			value: map[string]any{"parsed": "lz4", "rawvalue": "gzip", "value": "off"},
			want:  "LZ4",
		},
		{
			name:  "rawvalue fallback",
			value: map[string]any{"rawvalue": "standard", "value": "always"},
			want:  "STANDARD",
		},
		{
			name:  "value fallback",
			value: map[string]any{"value": "on"},
			want:  "ON",
		},
		{
			name:  "plain string",
			value: "off",
			want:  "OFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := propertyString(tt.value)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParentDatasetMissingDetection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matching truenas error",
			err: &apiError{
				StatusCode: http.StatusUnprocessableEntity,
				Body:       `{"message":"Parent dataset (apps/apps) does not exist."}`,
			},
			want: true,
		},
		{
			name: "wrong status",
			err: &apiError{
				StatusCode: http.StatusBadRequest,
				Body:       `{"message":"Parent dataset (apps/apps) does not exist."}`,
			},
			want: false,
		},
		{
			name: "wrong body",
			err: &apiError{
				StatusCode: http.StatusUnprocessableEntity,
				Body:       `{"message":"Dataset already exists."}`,
			},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("network failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isParentDatasetMissing(tt.err)
			if got != tt.want {
				t.Fatalf("got %t, want %t", got, tt.want)
			}
		})
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := New(baseURL, "test-key", false)
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func requireMethodPath(t *testing.T, r *http.Request, method, path string) {
	t.Helper()

	if r.Method != method {
		t.Fatalf("got method %q, want %q", r.Method, method)
	}
	if r.URL.Path != path {
		t.Fatalf("got path %q, want %q", r.URL.Path, path)
	}
}

func assertBodyValue(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()

	if got := body[key]; got != want {
		t.Fatalf("got body[%q] %q, want %q", key, got, want)
	}
}

func testDataset(name string) Dataset {
	return Dataset{
		ID:            name,
		Name:          name,
		Type:          "FILESYSTEM",
		Atime:         "ON",
		Compression:   "LZ4",
		Copies:        1,
		Deduplication: "OFF",
		Exec:          "ON",
		Readonly:      "OFF",
		Recordsize:    "128K",
		Snapdir:       "HIDDEN",
		Sync:          "STANDARD",
	}
}

func datasetJSON(name string) string {
	return `{
		"name":` + quote(name) + `,
		"type":{"parsed":"filesystem"},
		"atime":{"parsed":"on"},
		"compression":{"parsed":"lz4"},
		"copies":{"parsed":1},
		"deduplication":{"parsed":"off"},
		"exec":{"parsed":"on"},
		"readonly":{"parsed":"off"},
		"recordsize":{"parsed":"131072"},
		"snapdir":{"parsed":"hidden"},
		"sync":{"parsed":"standard"}
	}`
}

func quote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func setRetrySettings(t *testing.T, attempts int, delay time.Duration) func() {
	t.Helper()

	previousAttempts := createDatasetParentRetryAttempts
	previousDelay := createDatasetParentRetryDelay
	createDatasetParentRetryAttempts = attempts
	createDatasetParentRetryDelay = delay

	return func() {
		createDatasetParentRetryAttempts = previousAttempts
		createDatasetParentRetryDelay = previousDelay
	}
}
