package browserprobe

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/you/aiceberg_agent/internal/domain/browserjourney"
)

const (
	Contract  = browserjourney.Contract
	MaxRecipe = 32 << 10
)

var (
	testIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	dnsNamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	errRecipe      = errors.New("browser_invalid_recipe")
)

type Journey struct {
	Contract   string `json:"contract"`
	ClientID   int64  `json:"client_id"`
	AssetID    int64  `json:"asset_id"`
	CheckID    int64  `json:"check_id"`
	EndpointID int64  `json:"endpoint_id"`
	Origin     string `json:"origin"`
	Address    string `json:"address"`
	BudgetMS   int    `json:"budget_ms"`
	Steps      []Step `json:"steps"`
}

func (j Journey) scope() browserjourney.Scope {
	return browserjourney.Scope{ClientID: j.ClientID, AssetID: j.AssetID, CheckID: j.CheckID, EndpointID: j.EndpointID}
}

type Step struct {
	Action string `json:"action"`
	Path   string `json:"path,omitempty"`
	TestID string `json:"test_id,omitempty"`
	Text   string `json:"text,omitempty"`
}

type Receipt = browserjourney.Receipt

type validatedJourney struct {
	Journey
	origin *url.URL
	pin    netip.Addr
}

func Decode(recipe []byte) (Journey, error) {
	var journey Journey
	if len(recipe) > MaxRecipe {
		return journey, errRecipe
	}
	decoder := json.NewDecoder(bytes.NewReader(recipe))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&journey) != nil || decoder.Decode(new(any)) != io.EOF {
		return Journey{}, errRecipe
	}
	_, err := validate(journey)
	return journey, err
}

func validate(j Journey) (validatedJourney, error) {
	if j.Contract != Contract || j.BudgetMS < browserjourney.MinBudget || j.BudgetMS > browserjourney.MaxBudget || len(j.Steps) < 3 || len(j.Steps) > browserjourney.MaxSteps {
		return validatedJourney{}, errRecipe
	}
	if !j.scope().Valid() {
		return validatedJourney{}, errRecipe
	}
	origin, pin, err := validateOrigin(j.Origin, j.Address)
	if err != nil {
		return validatedJourney{}, err
	}
	if j.Steps[0].Action != "navigate" || j.Steps[len(j.Steps)-1].Action != "expect_text" {
		return validatedJourney{}, errRecipe
	}
	for _, step := range j.Steps {
		if !validStep(step) {
			return validatedJourney{}, errRecipe
		}
	}
	return validatedJourney{Journey: j, origin: origin, pin: pin}, nil
}

func validateOrigin(raw, address string) (*url.URL, netip.Addr, error) {
	origin, err := url.Parse(raw)
	ip, ipErr := netip.ParseAddr(address)
	if err != nil || ipErr != nil || len(raw) > 300 || ip.Zone() != "" ||
		origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" || origin.Opaque != "" || (origin.Path != "" && origin.Path != "/") {
		return nil, netip.Addr{}, errRecipe
	}
	ip = ip.Unmap()
	host := origin.Hostname()
	literal, literalErr := netip.ParseAddr(host)
	if literalErr == nil {
		if literal.Zone() != "" || literal.Unmap() != ip {
			return nil, netip.Addr{}, errRecipe
		}
	} else if !dnsNamePattern.MatchString(host) || len(host) > 253 || ip.IsLoopback() {
		return nil, netip.Addr{}, errRecipe
	}
	if (!ip.IsGlobalUnicast() && !ip.IsLoopback()) || ip == netip.MustParseAddr("100.100.100.200") || ip == netip.MustParseAddr("fd00:ec2::254") {
		return nil, netip.Addr{}, errRecipe
	}
	if origin.Scheme != "https" && (origin.Scheme != "http" || !ip.IsLoopback()) {
		return nil, netip.Addr{}, errRecipe
	}
	if port := origin.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 || port != strconv.FormatUint(number, 10) {
			return nil, netip.Addr{}, errRecipe
		}
	}
	origin.Path = ""
	return origin, ip, nil
}

func validStep(step Step) bool {
	switch step.Action {
	case "navigate":
		startsWithPath := strings.HasPrefix(step.Path, "/") && !strings.HasPrefix(step.Path, "//")
		return startsWithPath && len(step.Path) <= 200 && step.TestID == "" && step.Text == "" && !strings.ContainsAny(step.Path, "\\%?# \t\r\n")
	case "click":
		return step.Path == "" && testIDPattern.MatchString(step.TestID) && step.Text == ""
	case "expect_text":
		return step.Path == "" && testIDPattern.MatchString(step.TestID) &&
			len(step.Text) >= 1 && len(step.Text) <= 128 && !strings.ContainsAny(step.Text, "\x00\r\n")
	default:
		return false
	}
}

func (j validatedJourney) sameOrigin(candidate *url.URL) bool {
	return candidate.User == nil && candidate.Scheme == j.origin.Scheme &&
		sameOriginHostname(candidate.Hostname(), j.origin.Hostname()) && originPort(candidate) == originPort(j.origin)
}

func sameOriginHostname(left, right string) bool {
	first, leftErr := netip.ParseAddr(left)
	second, rightErr := netip.ParseAddr(right)
	if leftErr == nil && rightErr == nil {
		return first.Unmap() == second.Unmap()
	}
	return strings.EqualFold(left, right)
}

func originPort(origin *url.URL) string {
	if value := origin.Port(); value != "" {
		return value
	}
	if origin.Scheme == "http" {
		return "80"
	}
	return "443"
}
