package fulcrumcli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestProperties struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type TestConfig struct {
	TestProperty string `json:"testProperty"`
}

func TestHTTPClient_UpdateAgentStatus(t *testing.T) {
	tests := []struct {
		name           string
		status         agent.AgentStatus
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "successful status update",
			status:         agent.AgentStatusConnected,
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "server error",
			status:         agent.AgentStatusError,
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/api/v1/agents/me/status", r.URL.Path)

				// Verify headers
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify request body
				var body map[string]any
				bodyBytes, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &body)
				require.NoError(t, err)
				assert.Equal(t, string(tt.status), body["status"])

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			err := client.UpdateAgentStatus(tt.status)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHTTPClient_GetAgentInfo(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockResponse   *agent.AgentInfo[json.RawMessage]
		expectError    bool
	}{
		{
			name:           "successful get agent info",
			mockStatusCode: http.StatusOK,
			mockResponse: &agent.AgentInfo[json.RawMessage]{
				ID:     "agent-123",
				Name:   "test-agent",
				Status: agent.AgentStatusConnected,
			},
			expectError: false,
		},
		{
			name:           "server error",
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/agents/me", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					json.NewEncoder(w).Encode(tt.mockResponse)
				}
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			result, err := client.GetAgentInfo()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.mockResponse, result)
			}
		})
	}
}

func TestHTTPClient_GetPendingJobs(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockJobs       []*agent.RawJob
		expectError    bool
	}{
		{
			name:           "successful get pending jobs",
			mockStatusCode: http.StatusOK,
			mockJobs: func() []*agent.RawJob {
				params := json.RawMessage(`{"name":"test-service","value":42}`)
				return []*agent.RawJob{
					{
						ID:       "job-1",
						Action:   "create",
						Status:   agent.JobStatusPending,
						Priority: 1,
						Params:   &params,
					},
				}
			}(),
			expectError: false,
		},
		{
			name:           "empty jobs list",
			mockStatusCode: http.StatusOK,
			mockJobs:       []*agent.RawJob{},
			expectError:    false,
		},
		{
			name:           "server error",
			mockStatusCode: http.StatusInternalServerError,
			mockJobs:       nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/jobs/pending", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockJobs != nil {
					json.NewEncoder(w).Encode(tt.mockJobs)
				}
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			result, err := client.GetPendingJobs()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.mockJobs, result)
			}
		})
	}
}

func TestHTTPClient_ClaimJob(t *testing.T) {
	tests := []struct {
		name           string
		jobID          string
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "successful job claim with 200",
			jobID:          "job-123",
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "successful job claim with 204",
			jobID:          "job-123",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "job not found",
			jobID:          "job-123",
			mockStatusCode: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodPost, r.Method)
				expectedPath := fmt.Sprintf("/api/v1/jobs/%s/claim", tt.jobID)
				assert.Equal(t, expectedPath, r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			err := client.ClaimJob(tt.jobID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHTTPClient_CompleteJob(t *testing.T) {
	tests := []struct {
		name           string
		jobID          string
		response       any
		mockStatusCode int
		expectError    bool
	}{
		{
			name:  "successful job completion with 200",
			jobID: "job-123",
			response: map[string]any{
				"result": "success",
				"data":   "test-data",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "successful job completion with 204",
			jobID:          "job-123",
			response:       map[string]any{"message": "simple response"},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "server error",
			jobID:          "job-123",
			response:       map[string]any{"error": "response"},
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodPost, r.Method)
				expectedPath := fmt.Sprintf("/api/v1/jobs/%s/complete", tt.jobID)
				assert.Equal(t, expectedPath, r.URL.Path)

				// Verify request body matches the response
				bodyBytes, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				expectedBody, _ := json.Marshal(tt.response)
				assert.JSONEq(t, string(expectedBody), string(bodyBytes))

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			err := client.CompleteJob(tt.jobID, tt.response)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHTTPClient_FailJob(t *testing.T) {
	tests := []struct {
		name           string
		jobID          string
		errorMessage   string
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "successful job failure with 200",
			jobID:          "job-123",
			errorMessage:   "Something went wrong",
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "successful job failure with 204",
			jobID:          "job-123",
			errorMessage:   "Another error",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "server error",
			jobID:          "job-123",
			errorMessage:   "Error message",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodPost, r.Method)
				expectedPath := fmt.Sprintf("/api/v1/jobs/%s/fail", tt.jobID)
				assert.Equal(t, expectedPath, r.URL.Path)

				// Verify request body
				var body map[string]any
				bodyBytes, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &body)
				require.NoError(t, err)
				assert.Equal(t, tt.errorMessage, body["errorMessage"])

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			err := client.FailJob(tt.jobID, tt.errorMessage)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHTTPClient_ReportMetric(t *testing.T) {
	tests := []struct {
		name           string
		metric         *agent.MetricEntry
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful metric report with 200",
			metric: &agent.MetricEntry{
				AgentInstanceID: "ext-123",
				ResourceID:      "res-456",
				Value:           42.5,
				TypeName:        "cpu_usage",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name: "successful metric report with 201",
			metric: &agent.MetricEntry{
				AgentInstanceID: "ext-789",
				ResourceID:      "res-000",
				Value:           15.2,
				TypeName:        "memory_usage",
			},
			mockStatusCode: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "server error",
			metric: &agent.MetricEntry{
				AgentInstanceID: "ext-123",
				ResourceID:      "res-456",
				Value:           42.5,
				TypeName:        "cpu_usage",
			},
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and URL
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/metric-entries", r.URL.Path)

				// Verify request body
				var body agent.MetricEntry
				bodyBytes, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &body)
				require.NoError(t, err)
				assert.Equal(t, *tt.metric, body)

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "test-token")
			err := client.ReportMetric(tt.metric)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHTTPClient_URLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
		endpoint    string
	}{
		{
			name:        "standard URL construction",
			baseURL:     "https://api.example.com",
			endpoint:    "/api/v1/agents/me",
			expectedURL: "https://api.example.com/api/v1/agents/me",
		},
		{
			name:        "URL with trailing slash",
			baseURL:     "https://api.example.com/",
			endpoint:    "/api/v1/agents/me",
			expectedURL: "https://api.example.com/api/v1/agents/me",
		},
		{
			name:        "URL with path",
			baseURL:     "https://api.example.com/base",
			endpoint:    "/api/v1/agents/me",
			expectedURL: "https://api.example.com/base/api/v1/agents/me",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{"test": "data"})
			}))
			defer server.Close()

			// Test that the client can be created with different base URLs
			client := NewHTTPClient(server.URL, "test-token")
			_, err := client.GetAgentInfo()
			assert.NoError(t, err)
		})
	}
}

func TestNewHTTPClient(t *testing.T) {
	baseURL := "https://api.example.com"
	token := "test-token"

	client := NewHTTPClient(baseURL, token)

	assert.NotNil(t, client)
	assert.Equal(t, token, client.token)
	assert.NotNil(t, client.client)

	// Verify that the client is properly configured
	assert.Equal(t, baseURL, client.client.BaseURL())
}
