package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/keepgo"
)

var defaultCountries = []string{
	"Turkey",
	"Egypt",
	"United Arab Emirates",
	"India",
	"Thailand",
	"Vietnam",
	"China",
}

type catalogReport struct {
	Environment string          `json:"environment"`
	FetchedAt   time.Time       `json:"fetched_at"`
	BalanceUSD  string          `json:"balance_usd"`
	Countries   []string        `json:"countries"`
	Bundles     []keepgo.Bundle `json:"bundles"`
	Statistics  map[string]int  `json:"statistics"`
}

func main() {
	client, err := keepgo.NewClient(keepgo.Config{
		BaseURL:     envOrDefault("KEEPGO_BASE_URL", keepgo.SandboxBaseURL),
		APIKey:      os.Getenv("KEEPGO_API_KEY"),
		AccessToken: os.Getenv("KEEPGO_ACCESS_TOKEN"),
	})
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	balance, err := client.AccountBalance(ctx)
	if err != nil {
		fail(fmt.Errorf("read sandbox balance: %w", err))
	}
	bundles, err := client.Bundles(ctx, true)
	if err != nil {
		fail(fmt.Errorf("read sandbox catalogue: %w", err))
	}
	countries := configuredCountries()
	selected := filterBundles(bundles.Bundles, countries)
	report := catalogReport{
		Environment: "keepgo-sandbox",
		FetchedAt:   time.Now().UTC(),
		BalanceUSD:  string(balance.AccountBalance),
		Countries:   countries,
		Bundles:     selected,
		Statistics: map[string]int{
			"all_bundles":      len(bundles.Bundles),
			"selected_bundles": len(selected),
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		fail(fmt.Errorf("write catalogue report: %w", err))
	}
}

func configuredCountries() []string {
	value := strings.TrimSpace(os.Getenv("KEEPGO_COUNTRIES"))
	if value == "" {
		return append([]string(nil), defaultCountries...)
	}
	parts := strings.Split(value, ",")
	countries := make([]string, 0, len(parts))
	for _, part := range parts {
		if country := strings.TrimSpace(part); country != "" {
			countries = append(countries, country)
		}
	}
	return countries
}

func filterBundles(bundles []keepgo.Bundle, countries []string) []keepgo.Bundle {
	wanted := make(map[string]struct{}, len(countries))
	for _, country := range countries {
		wanted[strings.ToLower(country)] = struct{}{}
	}
	selected := make([]keepgo.Bundle, 0)
	for _, bundle := range bundles {
		for _, coverage := range bundle.Coverage {
			if _, ok := wanted[strings.ToLower(coverage)]; ok {
				selected = append(selected, bundle)
				break
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return selected
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "keepgo-catalog:", err)
	os.Exit(1)
}
