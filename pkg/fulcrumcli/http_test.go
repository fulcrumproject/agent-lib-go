package fulcrumcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fulcrumproject/agent-lib-go/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Helper function to create a mock HTTP response
func createMockResponse(statusCode int, body interface{}) *http.Response {
	var bodyReader io.ReadCloser
	if body != nil {
		bodyJSON, _ := json.Marshal(body)
		bodyReader = io.NopCloser(bytes.NewBuffer(bodyJSON))
	} else {
		bodyReader = io.NopCloser(strings.NewReader(""))
	}

	return &http.Response{
		StatusCode: statusCode,
		Body:       bodyReader,
		Header:     make(http.Header),
	}
}

// Test properties type for generic testing
type TestProperties struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodPut {
					return false
				}
				expectedURL := "https://api.example.com/api/v1/agents/me/status"
				if req.URL.String() != expectedURL {
					return false
				}

				// Verify headers
				if req.Header.Get("Authorization") != "Bearer test-token" {
					return false
				}
				if req.Header.Get("Content-Type") != "application/json" {
					return false
				}

				// Verify request body
				var body map[string]interface{}
				bodyBytes, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset body for actual use
				json.Unmarshal(bodyBytes, &body)
				return body["status"] == string(tt.status)
			})).Return(createMockResponse(tt.mockStatusCode, nil), nil)

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
		mockResponse   map[string]interface{}
		expectError    bool
	}{
		{
			name:           "successful get agent info",
			mockStatusCode: http.StatusOK,
			mockResponse: map[string]interface{}{
				"id":     "agent-123",
				"name":   "test-agent",
				"status": "Connected",
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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodGet {
					return false
				}
				expectedURL := "https://api.example.com/api/v1/agents/me"
				return req.URL.String() == expectedURL &&
					req.Header.Get("Authorization") == "Bearer test-token"
			})).Return(createMockResponse(tt.mockStatusCode, tt.mockResponse), nil)

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
		mockJobs       []*agent.Job[TestProperties]
		expectError    bool
	}{
		{
			name:           "successful get pending jobs",
			mockStatusCode: http.StatusOK,
			mockJobs: []*agent.Job[TestProperties]{
				{
					ID:       "job-1",
					Action:   agent.JobActionServiceCreate,
					Status:   agent.JobStatusPending,
					Priority: 1,
					Service: agent.Service[TestProperties]{
						ID:   "service-1",
						Name: "test-service",
					},
				},
			},
			expectError: false,
		},
		{
			name:           "empty jobs list",
			mockStatusCode: http.StatusOK,
			mockJobs:       []*agent.Job[TestProperties]{},
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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodGet {
					return false
				}
				expectedURL := "https://api.example.com/api/v1/jobs/pending"
				return req.URL.String() == expectedURL &&
					req.Header.Get("Authorization") == "Bearer test-token"
			})).Return(createMockResponse(tt.mockStatusCode, tt.mockJobs), nil)

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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodPost {
					return false
				}
				expectedURL := fmt.Sprintf("https://api.example.com/api/v1/jobs/%s/claim", tt.jobID)
				return req.URL.String() == expectedURL &&
					req.Header.Get("Authorization") == "Bearer test-token"
			})).Return(createMockResponse(tt.mockStatusCode, nil), nil)

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
		response       interface{}
		mockStatusCode int
		expectError    bool
	}{
		{
			name:  "successful job completion with 200",
			jobID: "job-123",
			response: map[string]interface{}{
				"result": "success",
				"data":   "test-data",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "successful job completion with 204",
			jobID:          "job-123",
			response:       "simple response",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "server error",
			jobID:          "job-123",
			response:       "response",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodPost {
					return false
				}
				expectedURL := fmt.Sprintf("https://api.example.com/api/v1/jobs/%s/complete", tt.jobID)
				if req.URL.String() != expectedURL {
					return false
				}

				// Verify request body matches the response
				var body interface{}
				bodyBytes, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset body for actual use
				json.Unmarshal(bodyBytes, &body)
				expectedBody, _ := json.Marshal(tt.response)
				actualBody, _ := json.Marshal(body)
				return string(expectedBody) == string(actualBody)
			})).Return(createMockResponse(tt.mockStatusCode, nil), nil)

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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodPost {
					return false
				}
				expectedURL := fmt.Sprintf("https://api.example.com/api/v1/jobs/%s/fail", tt.jobID)
				if req.URL.String() != expectedURL {
					return false
				}

				// Verify request body
				var body map[string]interface{}
				bodyBytes, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset body for actual use
				json.Unmarshal(bodyBytes, &body)
				return body["errorMessage"] == tt.errorMessage
			})).Return(createMockResponse(tt.mockStatusCode, nil), nil)

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
				ExternalID: "ext-123",
				ResourceID: "res-456",
				Value:      42.5,
				TypeName:   "cpu_usage",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name: "successful metric report with 201",
			metric: &agent.MetricEntry{
				ExternalID: "ext-789",
				ResourceID: "res-000",
				Value:      15.2,
				TypeName:   "memory_usage",
			},
			mockStatusCode: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "server error",
			metric: &agent.MetricEntry{
				ExternalID: "ext-123",
				ResourceID: "res-456",
				Value:      42.5,
				TypeName:   "cpu_usage",
			},
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    "https://api.example.com",
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				// Verify request method and URL
				if req.Method != http.MethodPost {
					return false
				}
				expectedURL := "https://api.example.com/api/v1/metric-entries"
				if req.URL.String() != expectedURL {
					return false
				}

				// Verify request body
				var body agent.MetricEntry
				bodyBytes, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset body for actual use
				json.Unmarshal(bodyBytes, &body)
				return body == *tt.metric
			})).Return(createMockResponse(tt.mockStatusCode, nil), nil)

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
		name           string
		baseURL        string
		expectedURL    string
		endpoint       string
		expectURLError bool
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
			mockHTTP := NewMockhttpClient(t)
			client := &HTTPClient[TestProperties]{
				baseURL:    tt.baseURL,
				token:      "test-token",
				httpClient: mockHTTP,
			}

			mockHTTP.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
				return req.URL.String() == tt.expectedURL
			})).Return(createMockResponse(http.StatusOK, map[string]interface{}{"test": "data"}), nil)

			_, err := client.GetAgentInfo()
			assert.NoError(t, err)
		})
	}
}

func TestHTTPClient_HTTPError(t *testing.T) {
	mockHTTP := NewMockhttpClient(t)
	client := &HTTPClient[TestProperties]{
		baseURL:    "https://api.example.com",
		token:      "test-token",
		httpClient: mockHTTP,
	}

	// Test HTTP client error (not HTTP status error)
	expectedError := fmt.Errorf("network error")
	mockHTTP.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(nil, expectedError)

	err := client.UpdateAgentStatus(agent.AgentStatusConnected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update agent status")
}

func TestNewHTTPClient(t *testing.T) {
	baseURL := "https://api.example.com"
	token := "test-token"

	client := NewHTTPClient[TestProperties](baseURL, token)

	assert.NotNil(t, client)
	assert.Equal(t, baseURL, client.baseURL)
	assert.Equal(t, token, client.token)
	assert.NotNil(t, client.httpClient)

	// Verify that the httpClient is a real HTTP client (not our mock)
	_, ok := client.httpClient.(*http.Client)
	assert.True(t, ok, "httpClient should be a real *http.Client")
}
