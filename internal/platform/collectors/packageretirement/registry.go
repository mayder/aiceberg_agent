package packageretirement

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type registryApplication struct {
	view, key, displayName string
}

func normalizeRegistryApplications(applications []registryApplication) (map[packageAddress]packageRecord, error) {
	if len(applications) > packageLimit {
		return nil, errors.New("application registry inventory exceeds limit")
	}
	inventory := make(map[packageAddress]packageRecord, len(applications))
	names := make(map[string]int, len(applications))
	for _, app := range applications {
		address := packageAddress{name: strings.ToLower(app.key), architecture: app.view}
		if _, exists := inventory[address]; exists || app.key == "" || (app.view != "32" && app.view != "64") {
			return nil, errors.New("invalid or repeated application registry key")
		}
		name := strings.Trim(app.displayName, " \t\n\r\x00\v")
		record := packageRecord{state: "present"}
		if name != "" && len(name) <= 255 && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0 {
			record.name = name
			record.state = "installed"
			names[name]++
		}
		inventory[address] = record
	}
	for address, record := range inventory {
		if names[record.name] > 1 {
			record.state = "ambiguous"
			inventory[address] = record
		}
	}
	return inventory, nil
}
