package health

import (
	"net/http"
	"strings"
)

type Failure struct {
	Kind     string
	Failover bool
	Cooldown bool
}

// ClassifyFailure distinguishes target health from request-specific rejection.
// Rate limiting affects only the current request and never opens the circuit.
func ClassifyFailure(status int, body []byte) Failure {
	if status >= 200 && status < 300 {
		return Failure{}
	}
	switch status {
	case http.StatusTooManyRequests:
		return Failure{Kind: "rate_limit", Failover: true}
	case http.StatusUnauthorized, http.StatusPaymentRequired:
		return Failure{Kind: "authentication_or_quota", Failover: true, Cooldown: true}
	case http.StatusForbidden:
		if containsFailureMarker(body, "content_policy", "safety", "moderation", "content_filter") {
			return Failure{Kind: "content_policy"}
		}
		return Failure{Kind: "permission", Failover: true, Cooldown: true}
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusGone:
		return Failure{Kind: "model_or_endpoint", Failover: true, Cooldown: true}
	case http.StatusRequestTimeout:
		return Failure{Kind: "timeout", Failover: true, Cooldown: true}
	case http.StatusConflict, http.StatusTooEarly:
		return Failure{Kind: "temporary_rejection", Failover: true}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		if containsFailureMarker(body, "model_not_found", "model_not_available", "model_unavailable", "model_decommissioned", "unsupported_model", "invalid_model", "model does not exist", "model is not available", "model is not supported", "model not found", "模型不存在", "模型不可用") {
			return Failure{Kind: "model_unavailable", Failover: true, Cooldown: true}
		}
		if containsFailureMarker(body, "invalid_api_key", "insufficient_quota", "account_deactivated") {
			return Failure{Kind: "authentication_or_quota", Failover: true, Cooldown: true}
		}
		if containsFailureMarker(body, "unsupported_parameter", "unsupported_value", "unsupported_feature", "bridge_incompatible") {
			return Failure{Kind: "capability", Failover: true}
		}
	}
	if status >= 500 && status <= 599 {
		return Failure{Kind: "upstream_failure", Failover: true, Cooldown: true}
	}
	return Failure{Kind: "request_error"}
}

func containsFailureMarker(body []byte, markers ...string) bool {
	message := strings.ToLower(string(body))
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
