package monitoring

// Definition describes an inert, versioned set of items for one authorized asset.
// It contains no endpoint, credential, expression or executor assignment.
type Definition struct {
	SchemaVersion int              `json:"schema_version"`
	Revision      int64            `json:"revision"`
	ClientID      int64            `json:"client_id"`
	AssetID       int64            `json:"asset_id"`
	Items         []ItemDefinition `json:"items"`
}

type DefinitionAuthority struct {
	ClientID int64   `json:"client_id"`
	AssetID  int64   `json:"asset_id"`
	CheckIDs []int64 `json:"check_ids"`
}

type ItemDefinition struct {
	Key          string   `json:"key"`
	CheckID      int64    `json:"check_id"`
	Metric       string   `json:"metric"`
	Source       string   `json:"source"`
	Kind         string   `json:"kind"`
	ValueType    string   `json:"value_type"`
	Unit         string   `json:"unit"`
	FreshnessSec int64    `json:"freshness_sec"`
	HistoryDays  int64    `json:"history_days"`
	TrendsDays   int64    `json:"trends_days"`
	Enabled      bool     `json:"enabled"`
	DependsOn    []string `json:"depends_on"`
}

const MaxDefinitionItems = 128
const MaxDefinitionDepth = 32
const MaxDefinitionBytes = 262144
