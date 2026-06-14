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
	"strconv"
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
	ACLMode       string
	ACLType       string
	Compression   string
	Copies        int64
	Deduplication string
	Exec          string
	Readonly      string
	Recordsize    string
	Snapdir       string
	Sync          string
}

type AppsConfig struct {
	ID                 string
	EnableImageUpdates bool
	Pool               string
	Nvidia             bool
	AddressPools       []AppsAddressPool
	PreferredTrains    []string
}

type AppsAddressPool struct {
	Base string
	Size int64
}

type AppConfig struct {
	ID     string
	Name   string
	Values json.RawMessage
}

type FilesystemStat struct {
	Path string
	Mode string
	UID  int64
	GID  int64
}

type FilesystemACL struct {
	Path      string
	UID       int64
	GID       int64
	ACLType   string
	ACL       json.RawMessage
	Recursive bool
}

type SMBShare struct {
	ID      int64
	Name    string
	Path    string
	Purpose string
	Enabled bool
	Comment string
}

var (
	createDatasetParentRetryAttempts = 20
	createDatasetParentRetryDelay    = 500 * time.Millisecond
	jobWaitPollInterval              = 2 * time.Second
	jobWaitTimeout                   = 30 * time.Minute
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
		"copies":        dataset.Copies,
		"deduplication": dataset.Deduplication,
		"exec":          dataset.Exec,
		"readonly":      dataset.Readonly,
		"recordsize":    dataset.Recordsize,
		"snapdir":       dataset.Snapdir,
		"sync":          dataset.Sync,
	}
	addOptionalString(body, "aclmode", dataset.ACLMode)
	addOptionalString(body, "acltype", dataset.ACLType)

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
		"copies":        dataset.Copies,
		"deduplication": dataset.Deduplication,
		"exec":          dataset.Exec,
		"readonly":      dataset.Readonly,
		"recordsize":    dataset.Recordsize,
		"snapdir":       dataset.Snapdir,
		"sync":          dataset.Sync,
	}
	addOptionalString(body, "aclmode", dataset.ACLMode)
	addOptionalString(body, "acltype", dataset.ACLType)

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

func (c *Client) GetFilesystemStat(ctx context.Context, path string) (FilesystemStat, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/filesystem/stat", path, &raw); err != nil {
		return FilesystemStat{}, err
	}

	return FilesystemStat{
		Path: stringValue(raw["path"]),
		Mode: permissionMode(stringValue(raw["mode"])),
		UID:  int64Value(raw["uid"]),
		GID:  int64Value(raw["gid"]),
	}, nil
}

func (c *Client) SetFilesystemPermission(ctx context.Context, stat FilesystemStat) error {
	body := map[string]any{
		"path": stat.Path,
		"mode": stat.Mode,
		"uid":  stat.UID,
		"gid":  stat.GID,
		"options": map[string]any{
			"recursive": false,
			"stripacl":  false,
			"traverse":  false,
		},
	}

	var job any
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/filesystem/setperm", body, &job); err != nil {
		return err
	}

	return c.waitForJobResponse(ctx, "filesystem.setperm", job)
}

func (c *Client) GetFilesystemACL(ctx context.Context, path string) (FilesystemACL, error) {
	body := map[string]any{
		"path":       path,
		"simplified": false,
	}

	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/filesystem/getacl", body, &raw); err != nil {
		return FilesystemACL{}, err
	}

	acl, err := canonicalJSONFromValue(raw["acl"])
	if err != nil {
		return FilesystemACL{}, fmt.Errorf("canonicalize filesystem ACL: %w", err)
	}

	return FilesystemACL{
		Path:    stringValue(raw["path"]),
		UID:     int64Value(raw["uid"]),
		GID:     int64Value(raw["gid"]),
		ACLType: stringValue(raw["acltype"]),
		ACL:     acl,
	}, nil
}

func (c *Client) SetFilesystemACL(ctx context.Context, acl FilesystemACL) error {
	var dacl any
	if err := json.Unmarshal(acl.ACL, &dacl); err != nil {
		return fmt.Errorf("decode filesystem ACL: %w", err)
	}

	body := map[string]any{
		"path":    acl.Path,
		"uid":     acl.UID,
		"gid":     acl.GID,
		"acltype": acl.ACLType,
		"dacl":    dacl,
		"options": map[string]any{
			"recursive": acl.Recursive,
			"traverse":  false,
		},
	}

	var job any
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/filesystem/setacl", body, &job); err != nil {
		return err
	}

	return c.waitForJobResponse(ctx, "filesystem.setacl", job)
}

func (c *Client) GetSMBShareByPath(ctx context.Context, path string) (SMBShare, error) {
	shares, err := c.ListSMBShares(ctx)
	if err != nil {
		return SMBShare{}, err
	}

	for _, share := range shares {
		if share.Path == path {
			return share, nil
		}
	}

	return SMBShare{}, fmt.Errorf("SMB share for path %q was not found", path)
}

func (c *Client) ListSMBShares(ctx context.Context) ([]SMBShare, error) {
	var rawShares []map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2.0/sharing/smb", nil, &rawShares); err != nil {
		return nil, err
	}

	shares := make([]SMBShare, 0, len(rawShares))
	for _, raw := range rawShares {
		shares = append(shares, smbShareFromAPI(raw))
	}

	return shares, nil
}

func (c *Client) CreateSMBShare(ctx context.Context, share SMBShare) (SMBShare, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/sharing/smb", smbShareToAPI(share), &raw); err != nil {
		return SMBShare{}, err
	}

	if len(raw) == 0 {
		return c.GetSMBShareByPath(ctx, share.Path)
	}

	return smbShareFromAPI(raw), nil
}

func (c *Client) UpdateSMBShare(ctx context.Context, share SMBShare) (SMBShare, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v2.0/sharing/smb/id/%d", share.ID), smbShareToAPI(share), &raw); err != nil {
		return SMBShare{}, err
	}

	if len(raw) == 0 {
		return c.GetSMBShareByPath(ctx, share.Path)
	}

	return smbShareFromAPI(raw), nil
}

func (c *Client) DeleteSMBShare(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v2.0/sharing/smb/id/%d", id), nil, nil)
}

func (c *Client) GetAppsConfig(ctx context.Context) (AppsConfig, error) {
	var dockerRaw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2.0/docker", nil, &dockerRaw); err != nil {
		return AppsConfig{}, err
	}

	var catalogRaw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v2.0/catalog", nil, &catalogRaw); err != nil {
		return AppsConfig{}, err
	}

	return appsConfigFromAPI(dockerRaw, catalogRaw), nil
}

func (c *Client) UpdateAppsConfig(ctx context.Context, config AppsConfig) (AppsConfig, error) {
	dockerBody := map[string]any{
		"enable_image_updates": config.EnableImageUpdates,
		"pool":                 config.Pool,
		"nvidia":               config.Nvidia,
		"address_pools":        addressPoolsToAPI(config.AddressPools),
	}
	var dockerJob any
	if err := c.do(ctx, http.MethodPut, "/api/v2.0/docker", dockerBody, &dockerJob); err != nil {
		return AppsConfig{}, err
	}
	if err := c.waitForJobResponse(ctx, "docker.update", dockerJob); err != nil {
		return AppsConfig{}, err
	}

	catalogBody := map[string]any{
		"preferred_trains": config.PreferredTrains,
	}
	var catalogJob any
	if err := c.do(ctx, http.MethodPut, "/api/v2.0/catalog", catalogBody, &catalogJob); err != nil {
		return AppsConfig{}, err
	}
	if err := c.waitForJobResponse(ctx, "catalog.update", catalogJob); err != nil {
		return AppsConfig{}, err
	}

	return c.GetAppsConfig(ctx)
}

func (c *Client) GetAppConfig(ctx context.Context, name string) (AppConfig, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/api/v2.0/app/config", name, &raw); err != nil {
		return AppConfig{}, err
	}

	canonical, err := canonicalJSON(raw)
	if err != nil {
		return AppConfig{}, err
	}

	return AppConfig{
		ID:     name,
		Name:   name,
		Values: canonical,
	}, nil
}

func (c *Client) UpdateAppConfig(ctx context.Context, config AppConfig) (AppConfig, error) {
	var values any
	if err := json.Unmarshal(config.Values, &values); err != nil {
		return AppConfig{}, fmt.Errorf("decode app config values: %w", err)
	}

	body := map[string]any{
		"values": values,
	}
	var appJob any
	if err := c.doEscaped(ctx, http.MethodPut, "/api/v2.0/app/id/"+config.Name, "/api/v2.0/app/id/"+url.PathEscape(config.Name), body, &appJob); err != nil {
		return AppConfig{}, err
	}
	if err := c.waitForJobResponse(ctx, "app.update", appJob); err != nil {
		return AppConfig{}, err
	}

	return c.GetAppConfig(ctx, config.Name)
}

func (c *Client) waitForJobResponse(ctx context.Context, method string, rawJob any) error {
	jobID, ok := jobID(rawJob)
	if !ok {
		return nil
	}

	return c.waitForJob(ctx, method, jobID)
}

func (c *Client) waitForJob(ctx context.Context, method string, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, jobWaitTimeout)
	defer cancel()

	for {
		job, err := c.getJob(ctx, id)
		if err != nil {
			return err
		}

		state := strings.ToUpper(stringValue(job["state"]))
		switch state {
		case "SUCCESS":
			return nil
		case "FAILED", "ABORTED":
			message := firstNonEmpty(stringValue(job["error"]), stringValue(job["exception"]))
			if message == "" {
				message = "job did not complete successfully"
			}
			return fmt.Errorf("%s job %d %s: %s", method, id, state, message)
		}

		timer := time.NewTimer(jobWaitPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) getJob(ctx context.Context, id int64) (map[string]any, error) {
	var jobs []map[string]any
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v2.0/core/get_jobs?limit=1&id=%d", id), nil, &jobs); err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return map[string]any{}, nil
	}

	return jobs[0], nil
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
	requestPath, rawQuery, _ := strings.Cut(path, "?")
	requestRawPath, _, _ := strings.Cut(rawPath, "?")
	if rawPath == "" {
		requestRawPath = ""
	}

	return c.baseURL.ResolveReference(&url.URL{
		Path:     requestPath,
		RawPath:  requestRawPath,
		RawQuery: rawQuery,
	})
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
		ACLMode:       propertyString(raw["aclmode"]),
		ACLType:       propertyString(raw["acltype"]),
		Compression:   propertyString(raw["compression"]),
		Copies:        propertyInt64(raw["copies"]),
		Deduplication: propertyString(raw["deduplication"]),
		Exec:          propertyString(raw["exec"]),
		Readonly:      propertyString(raw["readonly"]),
		Recordsize:    recordsizeString(raw["recordsize"]),
		Snapdir:       propertyString(raw["snapdir"]),
		Sync:          propertyString(raw["sync"]),
	}
}

func addOptionalString(body map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		body[key] = value
	}
}

func appsConfigFromAPI(dockerRaw map[string]any, catalogRaw map[string]any) AppsConfig {
	return AppsConfig{
		ID:                 "apps",
		EnableImageUpdates: boolValue(dockerRaw["enable_image_updates"]),
		Pool:               stringValue(dockerRaw["pool"]),
		Nvidia:             boolValue(dockerRaw["nvidia"]),
		AddressPools:       addressPoolsFromAPI(dockerRaw["address_pools"]),
		PreferredTrains:    stringSliceValue(catalogRaw["preferred_trains"]),
	}
}

func addressPoolsFromAPI(value any) []AppsAddressPool {
	rawPools, ok := value.([]any)
	if !ok {
		return nil
	}

	pools := make([]AppsAddressPool, 0, len(rawPools))
	for _, rawPool := range rawPools {
		pool, ok := rawPool.(map[string]any)
		if !ok {
			continue
		}
		pools = append(pools, AppsAddressPool{
			Base: stringValue(pool["base"]),
			Size: int64Value(pool["size"]),
		})
	}

	return pools
}

func addressPoolsToAPI(pools []AppsAddressPool) []map[string]any {
	rawPools := make([]map[string]any, 0, len(pools))
	for _, pool := range pools {
		rawPools = append(rawPools, map[string]any{
			"base": pool.Base,
			"size": pool.Size,
		})
	}

	return rawPools
}

func smbShareFromAPI(raw map[string]any) SMBShare {
	return SMBShare{
		ID:      int64Value(raw["id"]),
		Name:    stringValue(raw["name"]),
		Path:    stringValue(raw["path"]),
		Purpose: stringValue(raw["purpose"]),
		Enabled: boolValue(raw["enabled"]),
		Comment: stringValue(raw["comment"]),
	}
}

func smbShareToAPI(share SMBShare) map[string]any {
	return map[string]any{
		"name":    share.Name,
		"path":    share.Path,
		"purpose": share.Purpose,
		"enabled": share.Enabled,
		"comment": share.Comment,
	}
}

func stringSliceValue(value any) []string {
	rawValues, ok := value.([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		values = append(values, stringValue(rawValue))
	}

	return values
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return canonicalJSONFromValue(value)
}

func canonicalJSONFromValue(value any) (json.RawMessage, error) {
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode canonical json: %w", err)
	}

	return canonical, nil
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

func propertyInt64(value any) int64 {
	if property, ok := value.(map[string]any); ok {
		if parsed := int64Value(property["parsed"]); parsed != 0 {
			return parsed
		}
		if raw := int64Value(property["rawvalue"]); raw != 0 {
			return raw
		}
		if value := int64Value(property["value"]); value != 0 {
			return value
		}
	}

	return int64Value(value)
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

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
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

func permissionMode(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "0o") || strings.HasPrefix(value, "0O") {
		value = value[2:]
		if len(value) <= 3 {
			return value
		}
		return value[len(value)-3:]
	}

	if strings.HasPrefix(value, "40") || strings.HasPrefix(value, "10") {
		if len(value) <= 3 {
			return value
		}
		return value[len(value)-3:]
	}

	mode, err := strconv.ParseInt(value, 10, 64)
	if err != nil || mode <= 0 {
		return value
	}
	if mode <= 777 {
		trimmed := strings.TrimLeft(value, "0")
		if trimmed == "" {
			return "0"
		}
		return trimmed
	}

	return fmt.Sprintf("%03o", mode&0777)
}

func jobID(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed > 0
	case int64:
		return typed, typed > 0
	case int:
		return int64(typed), typed > 0
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil && parsed > 0
	case string:
		var parsed json.Number = json.Number(typed)
		id, err := parsed.Int64()
		return id, err == nil && id > 0
	case map[string]any:
		return jobID(typed["id"])
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
