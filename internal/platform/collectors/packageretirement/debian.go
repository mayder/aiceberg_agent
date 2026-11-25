package packageretirement

import (
	"errors"
	"regexp"
	"strings"
)

const (
	packageLimit     = 4096
	publicationLimit = 16
	freshnessSeconds = 300
)

var debianPackagePart = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)

type packageAddress struct{ name, architecture string }

type packageRecord struct {
	name  string
	want  string
	state string
}

func parseDebianInventory(output string) (map[packageAddress]packageRecord, error) {
	packages := make(map[packageAddress]packageRecord)
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 || !debianPackagePart.MatchString(fields[1]) || !debianPackagePart.MatchString(fields[2]) ||
			len(fields[0]) > 255 || (fields[0] != fields[1] && fields[0] != fields[1]+":"+fields[2]) || !debianPackageState(fields[4]) {
			return nil, errors.New("invalid dpkg inventory row")
		}
		address := packageAddress{fields[1], fields[2]}
		if _, exists := packages[address]; exists || len(packages) >= packageLimit {
			return nil, errors.New("ambiguous or oversized dpkg inventory")
		}
		packages[address] = packageRecord{name: fields[0], want: fields[3], state: fields[4]}
	}
	return packages, nil
}

func debianPackageState(state string) bool {
	switch state {
	case "installed", "not-installed", "config-files", "half-installed", "unpacked", "half-configured", "triggers-awaited", "triggers-pending":
		return true
	default:
		return false
	}
}
