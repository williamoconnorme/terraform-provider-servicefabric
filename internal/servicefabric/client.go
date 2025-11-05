package servicefabric

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const defaultAPIVersion = "6.0"

// Client provides a thin wrapper around the Service Fabric REST API.
type Client struct {
	endpoint   *url.URL
	apiVersion string
	httpClient *http.Client
	auth       Authenticator
}

// ClientConfig configures the Service Fabric client.
type ClientConfig struct {
	Endpoint      string
	APIVersion    string
	HTTPClient    *http.Client
	Authenticator Authenticator
}

// NewClient initializes a Service Fabric client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint required")
	}
	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &Client{
		endpoint:   parsed,
		apiVersion: apiVersion,
		httpClient: httpClient,
		auth:       cfg.Authenticator,
	}, nil
}

func (c *Client) buildURL(path string, query url.Values) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := *c.endpoint
	if base.Path == "" || base.Path == "/" {
		base.Path = path
	} else {
		base.Path = strings.TrimSuffix(base.Path, "/") + path
	}
	q := base.Query()
	for k, values := range query {
		for _, v := range values {
			q.Add(k, v)
		}
	}
	if q.Get("api-version") == "" {
		q.Set("api-version", c.apiVersion)
	}
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	urlStr, err := c.buildURL(path, query)
	if err != nil {
		return nil, err
	}

	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	if c.auth != nil {
		if err := c.auth.Apply(ctx, req); err != nil {
			return nil, err
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(b)),
		}
		var fabricErr struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		}
		if err := json.Unmarshal(b, &fabricErr); err == nil {
			if fabricErr.Error.Code != "" {
				apiErr.Code = fabricErr.Error.Code
			}
			if fabricErr.Error.Message != "" {
				apiErr.Message = strings.TrimSpace(fabricErr.Error.Message)
			}
		}
		return nil, apiErr
	}

	return resp, nil
}

func (c *Client) pollOperation(ctx context.Context, location string) error {
	if location == "" {
		return nil
	}
	// Some locations already include api-version. Respect existing query.
	var (
		delay = 5 * time.Second
	)
	for {
		target, err := c.resolveLocation(location)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		if c.auth != nil {
			if err := c.auth.Apply(ctx, req); err != nil {
				return err
			}
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("operation polling failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
		}

		var status operationStatus
		if err := json.Unmarshal(body, &status); err != nil {
			return fmt.Errorf("decode operation status: %w: %s", err, string(body))
		}

		switch strings.ToLower(status.State()) {
		case "succeeded", "success", "completed", "complete":
			return nil
		case "failed", "faulted":
			return fmt.Errorf("operation failed: %s", status.ErrorString())
		}

		if retry := resp.Header.Get("Retry-After"); retry != "" {
			if d, err := time.ParseDuration(retry + "s"); err == nil && d > 0 {
				delay = d
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

type operationStatus struct {
	Name   string          `json:"Name"`
	ID     string          `json:"Id"`
	Status string          `json:"Status"`
	StateF string          `json:"State"`
	Error  *operationError `json:"Error"`
}

func (o operationStatus) State() string {
	if o.Status != "" {
		return o.Status
	}
	return o.StateF
}

func (o operationStatus) ErrorString() string {
	if o.Error == nil {
		return ""
	}
	return o.Error.Message
}

type operationError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

const provisionKindExternalStore = "ExternalStore"

// provisionApplicationTypeRequest matches Service Fabric JSON ordering requirements.
type provisionApplicationTypeRequest struct {
	Kind                          string `json:"Kind"`
	Async                         bool   `json:"Async"`
	ApplicationTypeName           string `json:"ApplicationTypeName"`
	ApplicationTypeVersion        string `json:"ApplicationTypeVersion"`
	ApplicationPackageDownloadURI string `json:"ApplicationPackageDownloadUri,omitempty"`
}

type unprovisionApplicationTypeRequest struct {
	Async                  bool   `json:"Async"`
	ApplicationTypeVersion string `json:"ApplicationTypeVersion"`
	ForceRemove            bool   `json:"ForceRemove,omitempty"`
}

// ProvisionApplicationType registers an application type version from an external package.
func (c *Client) ProvisionApplicationType(ctx context.Context, name, version, packageURI string) error {
	body := provisionApplicationTypeRequest{
		Kind:                          provisionKindExternalStore,
		ApplicationTypeName:           name,
		ApplicationTypeVersion:        version,
		ApplicationPackageDownloadURI: packageURI,
		Async:                         true,
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/ApplicationTypes/$/Provision", nil, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		location := resp.Header.Get("Location")
		return c.pollOperation(ctx, location)
	}
	// For synchronous completion just drain body.
	io.Copy(io.Discard, resp.Body)
	return nil
}

// UnprovisionApplicationType removes an application type version from the cluster.
func (c *Client) UnprovisionApplicationType(ctx context.Context, name, version string, force bool) error {
	body := unprovisionApplicationTypeRequest{
		ApplicationTypeVersion: version,
		Async:                  true,
		ForceRemove:            force,
	}
	path := fmt.Sprintf("/ApplicationTypes/%s/$/Unprovision", url.PathEscape(name))
	resp, err := c.doRequest(ctx, http.MethodPost, path, nil, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return c.pollOperation(ctx, resp.Header.Get("Location"))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// GetApplicationTypeVersion retrieves metadata for a specific application type version.
func (c *Client) GetApplicationTypeVersion(ctx context.Context, name, version string) (*ApplicationTypeInfo, error) {
	items, err := c.ListApplicationTypeVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if strings.EqualFold(item.TypeName(), name) && strings.EqualFold(item.TypeVersion(), version) {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("application type %s/%s not found", name, version)
}

// ListApplicationTypeVersions retrieves application type versions optionally filtered by name.
func (c *Client) ListApplicationTypeVersions(ctx context.Context, name string) ([]ApplicationTypeInfo, error) {
	var (
		path  string
		query = url.Values{}
	)
	query.Set("api-version", "6.0")
	query.Set("ExcludeApplicationParameters", "false")
	path = "/ApplicationTypes"
	resp, err := c.doRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var list applicationTypeInfoList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	if name == "" {
		return list.Items, nil
	}

	filtered := make([]ApplicationTypeInfo, 0, len(list.Items))
	for _, item := range list.Items {
		if strings.EqualFold(item.TypeName(), name) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// CreateApplication deploys an application using the provided description.
func (c *Client) CreateApplication(ctx context.Context, app ApplicationDescription) error {
	if app.Name == "" {
		return fmt.Errorf("application name required")
	}
	app.prepare()
	endpoint := "/Applications/$/Create"
	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, nil, app)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return c.pollOperation(ctx, resp.Header.Get("Location"))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// DeleteApplication removes an application.
func (c *Client) DeleteApplication(ctx context.Context, name string, force bool) error {
	appID := url.PathEscape(applicationIDFromName(name))
	endpoint := fmt.Sprintf("/Applications/%s/$/Delete", appID)
	query := url.Values{}
	if force {
		query.Set("ForceRemove", "true")
	}
	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, query, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return c.pollOperation(ctx, resp.Header.Get("Location"))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

const (
	upgradeKindRolling              = "Rolling"
	rollingUpgradeModeUnmonitored   = "UnmonitoredAuto"
	upgradeStateRollingForwardDone  = "RollingForwardCompleted"
	upgradeStateRollingBackDone     = "RollingBackCompleted"
	upgradeStateRollingBackProgress = "RollingBackInProgress"
	upgradeStateFailed              = "Failed"
)

// ApplicationUpgradeDescription describes an application upgrade request.
type ApplicationUpgradeDescription struct {
	Name                         string               `json:"Name"`
	TargetApplicationTypeVersion string               `json:"TargetApplicationTypeVersion"`
	ParameterMap                 map[string]string    `json:"-"`
	Parameters                   []NameValueParameter `json:"Parameters,omitempty"`
	UpgradeKind                  string               `json:"UpgradeKind"`
	RollingUpgradeMode           string               `json:"RollingUpgradeMode,omitempty"`
	ForceRestart                 bool                 `json:"ForceRestart,omitempty"`
}

func (d *ApplicationUpgradeDescription) prepare() {
	if len(d.Parameters) == 0 && len(d.ParameterMap) > 0 {
		d.Parameters = mapToParameterList(d.ParameterMap)
	}
}

type applicationUpgradeProgress struct {
	UpgradeState         string `json:"UpgradeState"`
	FailureReason        string `json:"FailureReason"`
	UpgradeStatusDetails string `json:"UpgradeStatusDetails"`
}

// UpgradeApplication triggers a rolling upgrade and waits for completion.
func (c *Client) UpgradeApplication(ctx context.Context, desc ApplicationUpgradeDescription) error {
	if desc.Name == "" {
		return fmt.Errorf("application name required")
	}
	desc.prepare()
	if desc.UpgradeKind == "" {
		desc.UpgradeKind = upgradeKindRolling
	}
	if desc.RollingUpgradeMode == "" {
		desc.RollingUpgradeMode = rollingUpgradeModeUnmonitored
	}

	if err := c.startApplicationUpgrade(ctx, desc); err != nil {
		if IsApplicationUpgradeInProgressError(err) {
			if waitErr := c.waitForApplicationUpgrade(ctx, desc.Name); waitErr != nil {
				return waitErr
			}
			if err := c.startApplicationUpgrade(ctx, desc); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return c.waitForApplicationUpgrade(ctx, desc.Name)
}

func (c *Client) startApplicationUpgrade(ctx context.Context, desc ApplicationUpgradeDescription) error {
	appID := url.PathEscape(applicationIDFromName(desc.Name))
	endpoint := fmt.Sprintf("/Applications/%s/$/Upgrade", appID)
	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, nil, desc)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) waitForApplicationUpgrade(ctx context.Context, name string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		progress, err := c.getApplicationUpgradeProgress(ctx, name)
		if err != nil {
			if IsNotFoundError(err) {
				return nil
			}
			return err
		}

		switch progress.UpgradeState {
		case upgradeStateRollingForwardDone, "":
			return nil
		case upgradeStateRollingBackDone, upgradeStateFailed:
			return fmt.Errorf("application upgrade failed: state=%s details=%s", progress.UpgradeState, progress.UpgradeStatusDetails)
		case upgradeStateRollingBackProgress, "RollingForwardPending", "RollingForwardInProgress", "Invalid":
			// continue polling
		default:
			// Unknown state, continue polling but guard against hangs.
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) getApplicationUpgradeProgress(ctx context.Context, name string) (*applicationUpgradeProgress, error) {
	appID := url.PathEscape(applicationIDFromName(name))
	endpoint := fmt.Sprintf("/Applications/%s/$/GetUpgradeProgress", appID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var progress applicationUpgradeProgress
	if err := json.NewDecoder(resp.Body).Decode(&progress); err != nil {
		return nil, err
	}
	return &progress, nil
}

// GetApplication retrieves application information.
func (c *Client) GetApplication(ctx context.Context, name string) (*ApplicationInfo, error) {
	appID := url.PathEscape(applicationIDFromName(name))
	endpoint := fmt.Sprintf("/Applications/%s", appID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Some cluster versions respond with 202 Accepted while the application is
	// still materializing. Respect the async response, poll the provided
	// location (if any), then retry the read.
	if resp.StatusCode == http.StatusAccepted {
		location := resp.Header.Get("Location")
		io.Copy(io.Discard, resp.Body)
		if location != "" {
			if err := c.pollOperation(ctx, location); err != nil {
				return nil, err
			}
		}
		// Backoff briefly to give the cluster time to update, then retry.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		return c.GetApplication(ctx, name)
	}

	if resp.StatusCode == http.StatusNoContent {
		// Treat empty payload as not found.
		return nil, &APIError{
			Method:     http.MethodGet,
			Path:       endpoint,
			StatusCode: http.StatusNotFound,
			Message:    "application not found",
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty response retrieving application %s", name)
	}

	var info ApplicationInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode application response: %w: %s", err, string(body))
	}
	return &info, nil
}

// ListApplications returns all applications optionally filtered by type name.
func (c *Client) ListApplications(ctx context.Context, typeName string) ([]ApplicationInfo, error) {
	query := url.Values{}
	if typeName != "" {
		query.Set("ApplicationTypeName", typeName)
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/Applications/$/GetApplications", query, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var list applicationInfoList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ApplicationTypeInfo describes an application type version registered in the cluster.
type ApplicationTypeInfo struct {
	Name                   string               `json:"Name"`
	Version                string               `json:"Version"`
	ApplicationTypeName    string               `json:"ApplicationTypeName"`
	ApplicationTypeVersion string               `json:"ApplicationTypeVersion"`
	Status                 string               `json:"Status"`
	DefaultParameterList   []NameValueParameter `json:"DefaultParameterList"`
}

type applicationTypeInfoList struct {
	Items []ApplicationTypeInfo `json:"Items"`
}

func (a ApplicationTypeInfo) TypeName() string {
	if a.ApplicationTypeName != "" {
		return a.ApplicationTypeName
	}
	return a.Name
}

func (a ApplicationTypeInfo) TypeVersion() string {
	if a.ApplicationTypeVersion != "" {
		return a.ApplicationTypeVersion
	}
	return a.Version
}

// ApplicationDescription is the payload for creating/updating applications.
type ApplicationDescription struct {
	Name          string               `json:"Name"`
	TypeName      string               `json:"TypeName"`
	TypeVersion   string               `json:"TypeVersion"`
	ParameterMap  map[string]string    `json:"-"`
	ParameterList []NameValueParameter `json:"ParameterList,omitempty"`
	ApplicationCapacity         *ApplicationCapacityDescription         `json:"ApplicationCapacity,omitempty"`
	ManagedApplicationIdentity  *ManagedApplicationIdentityDescription  `json:"ManagedApplicationIdentity,omitempty"`
}

func (a *ApplicationDescription) prepare() {
	if len(a.ParameterList) == 0 && len(a.ParameterMap) > 0 {
		a.ParameterList = mapToParameterList(a.ParameterMap)
	}
}

// ApplicationCapacityDescription captures Service Fabric application capacity settings.
type ApplicationCapacityDescription struct {
	MinimumNodes       *int64                         `json:"MinimumNodes,omitempty"`
	MaximumNodes       *int64                         `json:"MaximumNodes,omitempty"`
	ApplicationMetrics []ApplicationMetricDescription `json:"ApplicationMetrics,omitempty"`
}

// ApplicationMetricDescription configures capacity metrics for an application.
type ApplicationMetricDescription struct {
	Name                   string `json:"Name,omitempty"`
	MaximumCapacity        *int64 `json:"MaximumCapacity,omitempty"`
	ReservationCapacity    *int64 `json:"ReservationCapacity,omitempty"`
	TotalApplicationCapacity *int64 `json:"TotalApplicationCapacity,omitempty"`
}

type ManagedApplicationIdentityDescription struct {
	TokenServiceEndpoint string                       `json:"TokenServiceEndpoint,omitempty"`
	IdentityRefs         []ManagedApplicationIdentity `json:"ManagedIdentities,omitempty"`
}

type ManagedApplicationIdentity struct {
	Name        string `json:"Name,omitempty"`
	PrincipalID string `json:"PrincipalId,omitempty"`
}

// ApplicationInfo represents an application instance.
type ApplicationInfo struct {
	ID            string               `json:"Id"`
	Name          string               `json:"Name"`
	TypeName      string               `json:"TypeName"`
	TypeVersion   string               `json:"TypeVersion"`
	Parameters    []NameValueParameter `json:"Parameters"`
	ParameterList []NameValueParameter `json:"ParameterList"`
	Status        string               `json:"Status"`
	HealthState   string               `json:"HealthState"`
	ManagedApplicationIdentity *ManagedApplicationIdentityDescription `json:"ManagedApplicationIdentity,omitempty"`
	ApplicationCapacity        *ApplicationCapacityDescription        `json:"ApplicationCapacity,omitempty"`
}

type applicationInfoList struct {
	Items []ApplicationInfo `json:"Items"`
}

// NameValueParameter is the common structure used by the Service Fabric API.
type NameValueParameter struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// ParameterListToMap converts parameter lists to maps for Terraform state consumption.
func ParameterListToMap(list []NameValueParameter) map[string]string {
	if len(list) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(list))
	for _, item := range list {
		out[item.Key] = item.Value
	}
	return out
}

func mapToParameterList(m map[string]string) []NameValueParameter {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]NameValueParameter, 0, len(keys))
	for _, k := range keys {
		result = append(result, NameValueParameter{Key: k, Value: m[k]})
	}
	return result
}

func (a ApplicationInfo) ParameterEntries() []NameValueParameter {
	if len(a.Parameters) > 0 {
		return a.Parameters
	}
	return a.ParameterList
}

func applicationIDFromName(name string) string {
	n := strings.TrimPrefix(name, "fabric:/")
	n = strings.TrimPrefix(n, "fabric:\\")
	n = strings.TrimPrefix(n, "/")
	n = strings.ReplaceAll(n, "/", "~")
	return n
}

func (c *Client) resolveLocation(location string) (string, error) {
	loc, err := url.Parse(location)
	if err != nil {
		return "", err
	}

	if !loc.IsAbs() {
		base := *c.endpoint
		if loc.Path != "" {
			if base.Path == "" || base.Path == "/" {
				base.Path = loc.Path
			} else {
				base.Path = strings.TrimSuffix(base.Path, "/") + "/" + strings.TrimPrefix(loc.Path, "/")
			}
		}
		query := base.Query()
		for k, values := range loc.Query() {
			for _, v := range values {
				query.Add(k, v)
			}
		}
		if query.Get("api-version") == "" {
			query.Set("api-version", c.apiVersion)
		}
		base.RawQuery = query.Encode()
		return base.String(), nil
	}

	query := loc.Query()
	if query.Get("api-version") == "" {
		query.Set("api-version", c.apiVersion)
		loc.RawQuery = query.Encode()
	}
	return loc.String(), nil
}
