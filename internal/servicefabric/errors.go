package servicefabric

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError represents a Service Fabric API error response.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s %s failed with status %d: %s", e.Method, e.Path, e.StatusCode, e.Message)
}

// IsNotFoundError returns true when the given error represents a 404 response.
func IsNotFoundError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsApplicationTypeInUseError reports whether the error corresponds to a conflict
// because an application type version is still in use.
func IsApplicationTypeInUseError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FABRIC_E_APPLICATION_TYPE_IN_USE"
	}
	return false
}

// IsApplicationTypeAlreadyExistsError reports whether the error corresponds to an
// attempt to provision an application type version that already exists.
func IsApplicationTypeAlreadyExistsError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FABRIC_E_APPLICATION_TYPE_ALREADY_EXISTS"
	}
	return false
}

// IsApplicationUpgradeInProgressError reports whether an upgrade is already in progress.
func IsApplicationUpgradeInProgressError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FABRIC_E_APPLICATION_UPGRADE_IN_PROGRESS"
	}
	return false
}

// IsApplicationAlreadyExistsError reports whether an application already exists.
func IsApplicationAlreadyExistsError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FABRIC_E_APPLICATION_ALREADY_EXISTS"
	}
	return false
}

// IsServiceAlreadyExistsError reports whether a service already exists.
func IsServiceAlreadyExistsError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FABRIC_E_SERVICE_ALREADY_EXISTS"
	}
	return false
}
