package keepgo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

var (
	sandboxICCIDPattern = regexp.MustCompile(`^[0-9]{18,22}$`)
	lpaPattern          = regexp.MustCompile(`^LPA:1\$([a-zA-Z0-9.-]+)\$([a-zA-Z0-9_-]+)(\$[a-zA-Z0-9_.-]*)*$`)
)

func (c *Client) IssueSandboxLine(ctx context.Context, quote sim.Quote) (sim.SandboxLine, error) {
	selection := quote.Selection
	if c.baseURL.String() != SandboxBaseURL || quote.ProviderID != sandboxProviderID ||
		quote.ProviderBundleID <= 0 || selection.PackageID == "" || !isMVPPackage(selection.TotalDataMB, selection.DurationDays) {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	body, err := json.Marshal(map[string]int64{
		"bundle_id": quote.ProviderBundleID, "refill_mb": int64(selection.TotalDataMB),
		"refill_days": int64(selection.DurationDays), "count": 1,
	})
	if err != nil {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, SandboxBaseURL+"/line/create", bytes.NewReader(body))
	if err != nil {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("apiKey", c.apiKey)
	request.Header.Set("accessToken", c.accessToken)
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	var result struct {
		Ack     string          `json:"ack"`
		SimCard json.RawMessage `json:"sim_card"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil || result.Ack != "success" {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	return parseSandboxCard(result.SimCard)
}

type issuedSandboxCard struct {
	ICCID string `json:"iccid"`
	LPA   string `json:"lpa_code"`
}

func parseSandboxCard(raw json.RawMessage) (sim.SandboxLine, error) {
	var card issuedSandboxCard
	if len(raw) > 0 && raw[0] == '[' {
		var cards []issuedSandboxCard
		if err := json.Unmarshal(raw, &cards); err != nil || len(cards) != 1 {
			return sim.SandboxLine{}, sim.ErrSandboxIssuance
		}
		card = cards[0]
	} else if err := json.Unmarshal(raw, &card); err != nil {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	lpa := strings.TrimSpace(card.LPA)
	parts := lpaPattern.FindStringSubmatch(lpa)
	if !sandboxICCIDPattern.MatchString(card.ICCID) || len(lpa) > 191 || len(parts) == 0 {
		return sim.SandboxLine{}, sim.ErrSandboxIssuance
	}
	return sim.SandboxLine{ICCID: card.ICCID, Activation: sim.ActivationData{
		SMAddress: parts[1], ActivationCode: parts[2], ManualCode: lpa, IsSandbox: true,
	}}, nil
}
