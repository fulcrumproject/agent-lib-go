package stdagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSecrets(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                string
		data                any
		mockSecretResponses map[string]any
		mockSecretErrors    map[string]error
		treatNotFoundAsNil  bool
		expected            any
		expectError         bool
	}{
		{
			name: "simple string secret",
			data: "vault://secret123",
			mockSecretResponses: map[string]any{
				"secret123": "resolved-secret-value",
			},
			treatNotFoundAsNil: false,
			expected:           "resolved-secret-value",
			expectError:         false,
		},
		{
			name: "non-vault string unchanged",
			data: "regular-string-value",
			mockSecretResponses: map[string]any{},
			treatNotFoundAsNil: false,
			expected:           "regular-string-value",
			expectError:         false,
		},
		{
			name: "object with secret values",
			data: map[string]any{
				"apiKey":  "vault://key123",
				"region":  "us-east-1",
				"timeout": 30,
			},
			mockSecretResponses: map[string]any{
				"key123": "secret-api-key",
			},
			treatNotFoundAsNil: false,
			expected: map[string]any{
				"apiKey":  "secret-api-key",
				"region":  "us-east-1",
				"timeout": 30,
			},
			expectError: false,
		},
		{
			name: "nested object with secrets",
			data: map[string]any{
				"database": map[string]any{
					"host":     "localhost",
					"password": "vault://dbpass",
				},
				"apiKey": "vault://apikey",
			},
			mockSecretResponses: map[string]any{
				"dbpass": "db-secret-password",
				"apikey": "api-secret-key",
			},
			treatNotFoundAsNil: false,
			expected: map[string]any{
				"database": map[string]any{
					"host":     "localhost",
					"password": "db-secret-password",
				},
				"apiKey": "api-secret-key",
			},
			expectError: false,
		},
		{
			name: "array with secrets",
			data: []any{
				"vault://secret1",
				"regular-value",
				"vault://secret2",
			},
			mockSecretResponses: map[string]any{
				"secret1": "resolved1",
				"secret2": "resolved2",
			},
			treatNotFoundAsNil: false,
			expected: []any{
				"resolved1",
				"regular-value",
				"resolved2",
			},
			expectError: false,
		},
		{
			name: "ephemeral secret not found - set to nil",
			data: "vault://ephemeral123",
			mockSecretErrors: map[string]error{
				"ephemeral123": ErrSecretNotFound,
			},
			treatNotFoundAsNil: true,
			expected:           nil,
			expectError:         false,
		},
		{
			name: "ephemeral secret not found in object - set to nil",
			data: map[string]any{
				"password": "vault://ephemeral123",
				"username": "admin",
			},
			mockSecretErrors: map[string]error{
				"ephemeral123": ErrSecretNotFound,
			},
			treatNotFoundAsNil: true,
			expected: map[string]any{
				"password": nil,
				"username": "admin",
			},
			expectError: false,
		},
		{
			name: "secret not found - error when treatNotFoundAsNil is false",
			data: "vault://missing123",
			mockSecretErrors: map[string]error{
				"missing123": ErrSecretNotFound,
			},
			treatNotFoundAsNil: false,
			expectError:         true,
		},
		{
			name: "secret retrieval error",
			data: "vault://error123",
			mockSecretErrors: map[string]error{
				"error123": assert.AnError,
			},
			treatNotFoundAsNil: false,
			expectError:         true,
		},
		{
			name: "non-string types unchanged",
			data: map[string]any{
				"number":  42,
				"boolean": true,
				"null":    nil,
			},
			mockSecretResponses: map[string]any{},
			treatNotFoundAsNil: false,
			expected: map[string]any{
				"number":  42,
				"boolean": true,
				"null":    nil,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := NewMockFulcrumClient(t)

			// Setup mock responses
			for ref, value := range tt.mockSecretResponses {
				mockClient.On("GetSecret", ref).Return(value, nil)
			}

			// Setup mock errors
			for ref, err := range tt.mockSecretErrors {
				mockClient.On("GetSecret", ref).Return(nil, err)
			}

			result, err := resolveSecrets(ctx, mockClient, tt.data, tt.treatNotFoundAsNil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestResolveSecretsInJSON(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                string
		rawJSON             *json.RawMessage
		mockSecretResponses map[string]any
		mockSecretErrors    map[string]error
		treatNotFoundAsNil  bool
		expectedJSON        string
		expectError         bool
	}{
		{
			name: "resolve secrets in JSON object",
			rawJSON: func() *json.RawMessage {
				jsonStr := `{"apiKey":"vault://key123","region":"us-east-1"}`
				raw := json.RawMessage(jsonStr)
				return &raw
			}(),
			mockSecretResponses: map[string]any{
				"key123": "secret-api-key",
			},
			treatNotFoundAsNil: false,
			expectedJSON:       `{"apiKey":"secret-api-key","region":"us-east-1"}`,
			expectError:         false,
		},
		{
			name: "ephemeral secret not found - set to null",
			rawJSON: func() *json.RawMessage {
				jsonStr := `{"password":"vault://ephemeral123"}`
				raw := json.RawMessage(jsonStr)
				return &raw
			}(),
			mockSecretErrors: map[string]error{
				"ephemeral123": ErrSecretNotFound,
			},
			treatNotFoundAsNil: true,
			expectedJSON:       `{"password":null}`,
			expectError:         false,
		},
		{
			name: "nil JSON",
			rawJSON: nil,
			mockSecretResponses: map[string]any{},
			treatNotFoundAsNil: false,
			expectError:         false,
		},
		{
			name: "invalid JSON",
			rawJSON: func() *json.RawMessage {
				raw := json.RawMessage(`{invalid json}`)
				return &raw
			}(),
			mockSecretResponses: map[string]any{},
			treatNotFoundAsNil: false,
			expectError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := NewMockFulcrumClient(t)

			// Setup mock responses
			for ref, value := range tt.mockSecretResponses {
				mockClient.On("GetSecret", ref).Return(value, nil)
			}

			// Setup mock errors
			for ref, err := range tt.mockSecretErrors {
				mockClient.On("GetSecret", ref).Return(nil, err)
			}

			var originalJSON json.RawMessage
			if tt.rawJSON != nil {
				originalJSON = make(json.RawMessage, len(*tt.rawJSON))
				copy(originalJSON, *tt.rawJSON)
			}

			err := resolveSecretsInJSON(ctx, mockClient, tt.rawJSON, tt.treatNotFoundAsNil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedJSON != "" && tt.rawJSON != nil {
					var expected, actual map[string]any
					assert.NoError(t, json.Unmarshal([]byte(tt.expectedJSON), &expected))
					assert.NoError(t, json.Unmarshal(*tt.rawJSON, &actual))
					assert.Equal(t, expected, actual)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

