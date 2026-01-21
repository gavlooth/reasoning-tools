package main

import (
	"context"
	"fmt"
	"strings"
)

// FallbackProvider tries multiple providers in order until one succeeds.
type FallbackProvider struct {
	providers []Provider
}

func NewFallbackProvider(providers []Provider) Provider {
	if len(providers) == 0 {
		return nil
	}
	if len(providers) == 1 {
		return providers[0]
	}
	return &FallbackProvider{providers: providers}
}

func (f *FallbackProvider) Name() string {
	if len(f.providers) == 0 {
		return "fallback"
	}
	return f.providers[0].Name()
}

func (f *FallbackProvider) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	var errs []string
	for _, p := range f.providers {
		resp, err := p.Chat(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
	}
	return "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}

func (f *FallbackProvider) SupportsStreaming() bool {
	return true
}

func (f *FallbackProvider) ChatStream(ctx context.Context, messages []ChatMessage, opts ChatOptions, onToken TokenCallback) (string, error) {
	var errs []string
	for _, p := range f.providers {
		if sp, ok := p.(StreamingProvider); ok && sp.SupportsStreaming() {
			resp, err := sp.ChatStream(ctx, messages, opts, onToken)
			if err == nil {
				return resp, nil
			}
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
			continue
		}

		resp, err := p.Chat(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
	}
	return "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}
