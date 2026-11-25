package browserjourney

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const (
	Contract    = "monitoring.browser-journey.v1"
	RunContract = "monitoring.browser-run.v1"
	RunPath     = "/v1/run"
	BrowserKind = "browser"
	MaxWire     = 4096
	MaxSteps    = 10
	MinBudget   = 1000
	MaxBudget   = 30000
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Scope struct {
	ClientID   int64 `json:"client_id"`
	AssetID    int64 `json:"asset_id"`
	CheckID    int64 `json:"check_id"`
	EndpointID int64 `json:"endpoint_id"`
}

func (s Scope) Valid() bool {
	for _, id := range []int64{s.ClientID, s.AssetID, s.CheckID, s.EndpointID} {
		if id < 1 || id > 9007199254740991 {
			return false
		}
	}
	return true
}

type Request struct {
	Contract string `json:"contract"`
	Scope
	Origin   string `json:"origin"`
	Digest   string `json:"recipe_sha256"`
	BudgetMS int    `json:"budget_ms"`
}

func (r Request) Valid() bool {
	return r.Contract == RunContract && r.Scope.Valid() && ValidDigest(r.Digest) &&
		len(r.Origin) > 0 && len(r.Origin) <= 300 && r.BudgetMS >= MinBudget && r.BudgetMS <= MaxBudget
}

func ValidDigest(digest string) bool { return digestPattern.MatchString(digest) }

type Receipt struct {
	Contract string `json:"contract"`
	Scope
	Digest    string `json:"recipe_sha256,omitempty"`
	Outcome   string `json:"outcome"`
	Completed int    `json:"completed_steps"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

func (r Receipt) Matches(request Request) bool {
	if r.Contract != Contract || r.Scope != request.Scope || r.Digest != request.Digest ||
		r.ElapsedMS < 0 || r.ElapsedMS > int64(request.BudgetMS)+5000 || r.Completed < 0 || r.Completed > MaxSteps {
		return false
	}
	switch r.Outcome {
	case "ok":
		return r.Completed >= 3
	case "step_failed", "resource_limit", "cancelled", "timeout":
		return true
	case "invalid_recipe", "not_allowed", "runner_unavailable", "busy":
		return r.Completed == 0
	default:
		return false
	}
}

func Decode[T any](body io.Reader) (T, error) {
	var value T
	data, err := io.ReadAll(io.LimitReader(body, MaxWire+1))
	if err != nil {
		return value, err
	}
	if len(data) > MaxWire {
		return value, errors.New("browser_run_wire_limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return value, errors.New("browser_run_invalid_wire")
	}
	return value, nil
}
