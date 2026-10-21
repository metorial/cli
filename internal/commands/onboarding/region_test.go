package onboarding

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/commandutil"
	"github.com/metorial/cli/internal/config"
	"github.com/metorial/cli/internal/terminal"
	"github.com/spf13/cobra"
)

var testRegions = []auth.SignupRegion{
	{ID: "us1", Name: "United States", TraceURL: "https://us.example/trace"},
	{ID: "eu1", Name: "European Union", TraceURL: "https://eu.example/trace"},
}

func stubRegions(t *testing.T, response *auth.SignupRegionsResponse, latencies map[string]time.Duration) *[]string {
	t.Helper()
	var measured []string

	originalFetch := fetchSignupRegions
	originalMeasure := measureRegionLatency
	fetchSignupRegions = func(*url.URL) (*auth.SignupRegionsResponse, error) {
		if response == nil {
			return nil, errors.New("not found")
		}
		return response, nil
	}
	measureRegionLatency = func(traceURL string) (time.Duration, error) {
		measured = append(measured, traceURL)
		if latency, ok := latencies[traceURL]; ok {
			return latency, nil
		}
		return 0, errors.New("unreachable")
	}
	t.Cleanup(func() {
		fetchSignupRegions = originalFetch
		measureRegionLatency = originalMeasure
	})

	return &measured
}

func hostURL(t *testing.T) *url.URL {
	t.Helper()
	parsed, err := url.Parse("https://api.metorial.test")
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestDiscoverRegionsUsesServerRecommendationWithoutMeasuring(t *testing.T) {
	measured := stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions, RecommendedRegion: "eu1"}, nil)

	choice := discoverRegions(hostURL(t))
	if choice == nil || choice.Recommended != "eu1" || choice.RecommendedBy != "location" {
		t.Fatalf("choice = %+v", choice)
	}
	if len(*measured) != 0 {
		t.Fatalf("latency must not be measured when the server recommends a region: %v", *measured)
	}
}

func TestDiscoverRegionsFallsBackToLowestLatency(t *testing.T) {
	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions}, map[string]time.Duration{
		"https://us.example/trace": 180 * time.Millisecond,
		"https://eu.example/trace": 40 * time.Millisecond,
	})

	choice := discoverRegions(hostURL(t))
	if choice == nil || choice.Recommended != "eu1" || choice.RecommendedBy != "latency" {
		t.Fatalf("choice = %+v", choice)
	}
}

func TestDiscoverRegionsIgnoresUnknownServerRecommendation(t *testing.T) {
	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions, RecommendedRegion: "mars1"}, map[string]time.Duration{
		"https://us.example/trace": 20 * time.Millisecond,
		"https://eu.example/trace": 90 * time.Millisecond,
	})

	choice := discoverRegions(hostURL(t))
	if choice == nil || choice.Recommended != "us1" || choice.RecommendedBy != "latency" {
		t.Fatalf("choice = %+v", choice)
	}
}

func TestDiscoverRegionsWithoutAnyRecommendation(t *testing.T) {
	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions}, nil)

	choice := discoverRegions(hostURL(t))
	if choice == nil || choice.Recommended != "" || len(choice.Options) != 2 {
		t.Fatalf("choice = %+v", choice)
	}
}

func TestDiscoverRegionsSkipsHostsWithoutAChoice(t *testing.T) {
	stubRegions(t, nil, nil)
	if choice := discoverRegions(hostURL(t)); choice != nil {
		t.Fatalf("expected no choice when the lookup fails, got %+v", choice)
	}

	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions[:1]}, nil)
	if choice := discoverRegions(hostURL(t)); choice != nil {
		t.Fatalf("expected no choice for a single region, got %+v", choice)
	}
}

func TestAgentInstructionsListRecommendedRegionFirst(t *testing.T) {
	choice := &regionChoice{
		Options: []regionOption{
			{SignupRegion: testRegions[0]},
			{SignupRegion: testRegions[1], Latency: 35 * time.Millisecond},
		},
		Recommended:   "eu1",
		RecommendedBy: "latency",
	}

	text := choice.agentInstructions("metorial-admin")

	if strings.Index(text, "European Union (eu1) - 35ms - recommended") > strings.Index(text, "United States (us1)") {
		t.Fatalf("recommended region should be listed first:\n%s", text)
	}
	for _, expected := range []string{"lowest latency", "unless the user asked for a different one", "metorial-admin signup --region <id>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("instructions missing %q:\n%s", expected, text)
		}
	}
}

func TestAgentInstructionsWithoutRecommendationAskToPick(t *testing.T) {
	choice := &regionChoice{Options: []regionOption{{SignupRegion: testRegions[0]}, {SignupRegion: testRegions[1]}}}

	text := choice.agentInstructions("metorial-admin")
	if !strings.Contains(text, "No region could be recommended") || !strings.Contains(text, "closest to the user") {
		t.Fatalf("unexpected instructions:\n%s", text)
	}
}

func signupContext(t *testing.T, agentEnv string) (*cobra.Command, commandutil.Context) {
	t.Helper()
	setupStore(t)
	if agentEnv != "" {
		t.Setenv(agentEnv, "1")
	}

	application := app.NewWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, terminal.Features{}, terminal.Features{})
	application.SetSubjectType(config.SubjectTypeOrganizationMember)

	root := &cobra.Command{Use: "metorial-admin"}
	child := &cobra.Command{Use: "signup"}
	root.AddCommand(child)

	return child, commandutil.NewContext(application, &commandutil.RootOptions{})
}

func TestResolveSignupRegionPrefersTheFlag(t *testing.T) {
	stubRegions(t, nil, nil)
	command, ctx := signupContext(t, "CLAUDECODE")

	region, err := resolveSignupRegion(command, ctx, hostURL(t), " us1 ", true)
	if err != nil || region != "us1" {
		t.Fatalf("region = %q, err = %v", region, err)
	}
}

func TestResolveSignupRegionAgentMustChoose(t *testing.T) {
	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions, RecommendedRegion: "eu1"}, nil)
	command, ctx := signupContext(t, "CURSOR_AGENT")

	_, err := resolveSignupRegion(command, ctx, hostURL(t), "", true)
	if err == nil || !strings.Contains(err.Error(), "signup --region <id>") || !strings.Contains(err.Error(), "recommended") {
		t.Fatalf("expected region instructions, got %v", err)
	}
}

func TestResolveSignupRegionNonInteractiveUsesRecommendation(t *testing.T) {
	stubRegions(t, &auth.SignupRegionsResponse{Regions: testRegions, RecommendedRegion: "us1"}, nil)
	command, ctx := signupContext(t, "")

	region, err := resolveSignupRegion(command, ctx, hostURL(t), "", false)
	if err != nil || region != "us1" {
		t.Fatalf("region = %q, err = %v", region, err)
	}
}

func TestResolveSignupRegionWithoutRegionsLetsTheServerDecide(t *testing.T) {
	stubRegions(t, nil, nil)
	command, ctx := signupContext(t, "CLAUDECODE")

	region, err := resolveSignupRegion(command, ctx, hostURL(t), "", true)
	if err != nil || region != "" {
		t.Fatalf("region = %q, err = %v", region, err)
	}
}
