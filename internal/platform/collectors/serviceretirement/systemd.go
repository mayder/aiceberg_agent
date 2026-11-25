package serviceretirement

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

type systemdServiceInventory struct {
	run func(context.Context, ...string) (string, int, error)
}

var systemdServiceName = regexp.MustCompile(`^[a-zA-Z0-9:_@.\\-]+\.service$`)

func (s systemdServiceInventory) list(ctx context.Context) ([]string, error) {
	output, code, err := s.run(ctx, "list-unit-files", "--type=service", "--no-legend", "--plain", "--full")
	if err != nil || code != 0 {
		return nil, errors.New("systemd inventory unavailable")
	}
	var names []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 2 || !systemdServiceName.MatchString(fields[0]) {
			return nil, errors.New("unexpected systemd inventory format")
		}
		if strings.HasSuffix(fields[0], "@.service") || fields[1] == "transient" || fields[1] == "generated" || fields[1] == "not-found" {
			continue
		}
		names = append(names, fields[0])
	}
	return names, nil
}

func (s systemdServiceInventory) missing(ctx context.Context, name string) (bool, error) {
	if !systemdServiceName.MatchString(name) {
		return false, errors.New("invalid systemd service name")
	}
	output, code, err := s.run(ctx, "show", "--property=LoadState,ActiveState", "--", name)
	if err != nil || (code != 0 && code != 4) {
		return false, errors.New("systemd service state unavailable")
	}
	load, active := "", ""
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return false, errors.New("unexpected systemd service state format")
		}
		switch {
		case key == "LoadState" && load == "":
			load = value
		case key == "ActiveState" && active == "":
			active = value
		default:
			return false, errors.New("unexpected systemd service state property")
		}
	}
	return load == "not-found" && active == "inactive", nil
}
