package handler

import (
	"errors"

	domainstorage "yunxia/internal/domain/storage"
)

func cloudProviderErrorDetails(err error) map[string]any {
	var providerErr *domainstorage.ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	details := make(map[string]any)
	if providerErr.VerificationURL != "" {
		details["verification_url"] = providerErr.VerificationURL
		details["requires_manual_verification"] = true
	}
	if providerErr.RetryAfterSeconds > 0 {
		details["retry_after_seconds"] = providerErr.RetryAfterSeconds
	}
	if providerErr.ProviderCode != "" {
		details["provider_code"] = providerErr.ProviderCode
	}
	if len(details) == 0 {
		return nil
	}
	return details
}
