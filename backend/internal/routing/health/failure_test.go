package health

import "testing"

func TestClassifyFailure(t *testing.T) {
	for _, test := range []struct {
		name               string
		status             int
		body               string
		failover, cooldown bool
	}{
		{"success", 200, "", false, false},
		{"rate limit", 429, `{"error":{"code":"insufficient_quota"}}`, true, false},
		{"authentication", 401, "", true, true},
		{"quota", 402, "", true, true},
		{"permission", 403, "permission denied", true, true},
		{"safety", 403, `{"error":{"code":"content_policy_violation"}}`, false, false},
		{"missing model", 404, "", true, true},
		{"endpoint", 405, "", true, true},
		{"retired model", 410, "", true, true},
		{"timeout", 408, "", true, true},
		{"conflict", 409, "", true, false},
		{"server", 500, "", true, true},
		{"overloaded", 529, "", true, true},
		{"network gateway", 502, "", true, true},
		{"bad model", 400, `{"error":{"code":"model_not_found"}}`, true, true},
		{"unsupported model", 422, `{"error":{"code":"unsupported_model"}}`, true, true},
		{"unsupported feature", 400, `{"error":{"code":"unsupported_parameter"}}`, true, false},
		{"context limit", 400, `{"error":{"code":"context_length_exceeded"}}`, false, false},
		{"invalid messages", 422, `{"error":{"message":"messages must be an array"}}`, false, false},
		{"payload size", 413, "", false, false},
		{"canceled", 499, "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyFailure(test.status, []byte(test.body))
			if got.Failover != test.failover || got.Cooldown != test.cooldown {
				t.Fatalf("classification = %#v, want failover=%t cooldown=%t", got, test.failover, test.cooldown)
			}
		})
	}
}
