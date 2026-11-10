package onboarding

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/metorial/cli/internal/auth"
	"github.com/metorial/cli/internal/config"
)

const (
	regionLatencyTimeout  = 3 * time.Second
	regionLatencyAttempts = 2
)

type regionOption struct {
	auth.SignupRegion
	Latency time.Duration
}

type regionChoice struct {
	Options     []regionOption
	Recommended string
	// "location" when the server picked it from the caller's country, "latency" when measured here.
	RecommendedBy string
}

var fetchSignupRegions = func(apiHost *url.URL) (*auth.SignupRegionsResponse, error) {
	return auth.NewClient(apiHost).CLISignupRegions()
}

var measureRegionLatency = func(traceURL string) (time.Duration, error) {
	client := &http.Client{Timeout: regionLatencyTimeout}

	var best time.Duration
	for attempt := 0; attempt < regionLatencyAttempts; attempt++ {
		started := time.Now()
		response, err := client.Get(traceURL)
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()

		elapsed := time.Since(started)
		if best == 0 || elapsed < best {
			best = elapsed
		}
	}

	return best, nil
}

func discoverRegions(apiHost *url.URL) *regionChoice {
	response, err := fetchSignupRegions(apiHost)
	if err != nil || len(response.Regions) == 0 {
		if apiHost.Hostname() != config.DefaultAPIHost {
			return nil
		}
		// Older Metorial Cloud deployments may not expose region discovery yet.
		response = &auth.SignupRegionsResponse{Regions: []auth.SignupRegion{
			{ID: "us1", Name: "United States", TraceURL: "https://api-us1.metorial.com/.metorial/trace-region"},
			{ID: "eu1", Name: "European Union", TraceURL: "https://api-eu1.metorial.com/.metorial/trace-region"},
		}}
	}

	choice := &regionChoice{}
	for _, region := range response.Regions {
		choice.Options = append(choice.Options, regionOption{SignupRegion: region})
	}

	if choice.hasRegion(response.RecommendedRegion) {
		choice.Recommended = response.RecommendedRegion
		choice.RecommendedBy = "location"
		return choice
	}

	choice.measureLatencies()
	if fastest := choice.fastest(); fastest != "" {
		choice.Recommended = fastest
		choice.RecommendedBy = "latency"
	}

	return choice
}

func (c *regionChoice) hasRegion(id string) bool {
	if id == "" {
		return false
	}
	for _, option := range c.Options {
		if option.ID == id {
			return true
		}
	}
	return false
}

func (c *regionChoice) measureLatencies() {
	var wait sync.WaitGroup
	for index := range c.Options {
		wait.Add(1)
		go func(option *regionOption) {
			defer wait.Done()
			if latency, err := measureRegionLatency(option.TraceURL); err == nil {
				option.Latency = latency
			}
		}(&c.Options[index])
	}
	wait.Wait()
}

func (c *regionChoice) fastest() string {
	fastest := ""
	var best time.Duration
	for _, option := range c.Options {
		if option.Latency <= 0 {
			continue
		}
		if fastest == "" || option.Latency < best {
			fastest = option.ID
			best = option.Latency
		}
	}
	return fastest
}

func (c *regionChoice) recommendedFirst() []regionOption {
	options := append([]regionOption(nil), c.Options...)
	sort.SliceStable(options, func(i, j int) bool {
		return options[i].ID == c.Recommended && options[j].ID != c.Recommended
	})
	return options
}

func (c *regionChoice) label(option regionOption) string {
	label := fmt.Sprintf("%s (%s)", option.Name, option.ID)
	if option.Latency > 0 {
		label += fmt.Sprintf(" - %dms", option.Latency.Milliseconds())
	}
	if option.ID == c.Recommended {
		label += " - recommended"
	}
	return label
}

func (c *regionChoice) regionInstructions(rootName string) string {
	var builder strings.Builder

	builder.WriteString("metorial: --region is required to create a new organization. Available regions:\n")
	for _, option := range c.recommendedFirst() {
		builder.WriteString("- " + c.label(option) + "\n")
	}

	switch c.RecommendedBy {
	case "location":
		builder.WriteString(fmt.Sprintf("The recommended region is %s, based on this machine's location. ", c.Recommended))
	case "latency":
		builder.WriteString(fmt.Sprintf("The recommended region is %s, which has the lowest latency from this machine. ", c.Recommended))
	default:
		builder.WriteString("No region could be recommended automatically. ")
	}

	if c.Recommended != "" {
		builder.WriteString("Pick the recommended region unless the user asked for a different one (for example for data residency), ")
	} else {
		builder.WriteString("Choose the region that meets the user’s location or data residency needs, ")
	}
	builder.WriteString(fmt.Sprintf("then run `%s signup --region <id>`.", rootName))

	return builder.String()
}
