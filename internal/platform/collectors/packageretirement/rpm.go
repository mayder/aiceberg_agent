package packageretirement

import (
	"errors"
	"regexp"
	"strings"
)

var rpmPackagePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]*$`)

func parseRPMInventory(output string) (map[packageAddress]packageRecord, error) {
	packages := make(map[packageAddress]packageRecord)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) > packageLimit {
		return nil, errors.New("oversized RPM inventory")
	}
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || !rpmPackagePart.MatchString(fields[0]) || len(fields[0]) > 255 {
			return nil, errors.New("invalid RPM inventory row")
		}
		if fields[0] == "gpg-pubkey" && fields[1] == "(none)" {
			continue
		}
		if !rpmPackagePart.MatchString(fields[1]) || len(fields[1]) > 64 {
			return nil, errors.New("invalid RPM package architecture")
		}
		// Several installed versions of a package share the inventory identity.
		packages[packageAddress{fields[0], fields[1]}] = packageRecord{name: fields[0], state: "installed"}
	}
	if len(packages) == 0 {
		return nil, errors.New("empty RPM inventory")
	}
	return packages, nil
}
