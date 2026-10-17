package main

import (
	"testing"

	"github.com/neith-config/neith-agent/internal/agent"
)

func TestResolveHubAndTokenUsesExplicitFlagsThenEnvironmentDefaults(t *testing.T) {
	t.Setenv("NEITH_HUB_URL", "http://env-hub")
	t.Setenv("NEITH_TOKEN", "env-token")

	if got := resolveHubURL("http://flag-hub"); got != "http://flag-hub" {
		t.Fatalf("flag Hub URL lost to environment: %q", got)
	}
	if got := resolveToken("flag-token"); got != "flag-token" {
		t.Fatalf("flag token lost to environment: %q", got)
	}
	if got := resolveHubURL(""); got != "http://env-hub" {
		t.Fatalf("environment Hub URL not used as fallback: %q", got)
	}
	if got := resolveToken(""); got != "env-token" {
		t.Fatalf("environment token not used as fallback: %q", got)
	}

	t.Setenv("NEITH_HUB_URL", "")
	if got := resolveHubURL(""); got != agent.DefaultHubURL {
		t.Fatalf("Hub URL default = %q, want %q", got, agent.DefaultHubURL)
	}
}
