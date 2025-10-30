package stdagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// resolveSecretsInJSON resolves all vault:// references in a JSON structure
// It modifies the JSON in-place by resolving secrets from the vault.
// If a secret is not found and treatNotFoundAsNil is true, sets the value to nil.
func resolveSecretsInJSON(ctx context.Context, client FulcrumClient, rawJSON *json.RawMessage, treatNotFoundAsNil bool) error {
	if rawJSON == nil {
		return nil
	}

	var data any
	if err := json.Unmarshal(*rawJSON, &data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON for secret resolution: %w", err)
	}

	resolved, err := resolveSecrets(ctx, client, data, treatNotFoundAsNil)
	if err != nil {
		return err
	}

	resolvedJSON, err := json.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("failed to marshal resolved JSON: %w", err)
	}

	*rawJSON = json.RawMessage(resolvedJSON)
	return nil
}

// resolveSecrets recursively resolves vault:// references in a JSON structure
// Returns the resolved structure with secrets replaced by their actual values
// If treatNotFoundAsNil is true and a secret is not found, sets it to nil
func resolveSecrets(ctx context.Context, client FulcrumClient, data any, treatNotFoundAsNil bool) (any, error) {
	switch v := data.(type) {
	case string:
		if strings.HasPrefix(v, "vault://") {
			reference := strings.TrimPrefix(v, "vault://")
			secretValue, err := client.GetSecret(reference)
			if err != nil {
				if treatNotFoundAsNil && errors.Is(err, ErrSecretNotFound) {
					// For ephemeral secrets that are not found, set to nil
					return nil, nil
				}
				return nil, fmt.Errorf("failed to resolve secret %s: %w", reference, err)
			}
			return secretValue, nil
		}
		return v, nil

	case map[string]any:
		resolved := make(map[string]any)
		for key, value := range v {
			resolvedValue, err := resolveSecrets(ctx, client, value, treatNotFoundAsNil)
			if err != nil {
				return nil, err
			}
			resolved[key] = resolvedValue
		}
		return resolved, nil

	case []any:
		resolved := make([]any, len(v))
		for i, item := range v {
			resolvedItem, err := resolveSecrets(ctx, client, item, treatNotFoundAsNil)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedItem
		}
		return resolved, nil

	default:
		// For other types (numbers, booleans, null), no resolution needed
		return v, nil
	}
}

