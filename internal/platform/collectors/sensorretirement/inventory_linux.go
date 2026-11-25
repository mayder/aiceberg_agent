package sensorretirement

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/shirou/gopsutil/v3/host"
)

var hwmonName = regexp.MustCompile(`^hwmon[0-9]+$`)

type sensorInventory struct {
	byDevice map[string][]string
	counts   map[string]int
}

// readInventory correlates direct hwmon entries with the existing collector's
// SensorKey. A mismatch leaves the inventory unqualified for retirement.
func readInventory(ctx context.Context, root string) (sensorInventory, error) {
	var result sensorInventory
	class := filepath.Join(root, "class", "hwmon")
	devices, err := os.ReadDir(class)
	if err != nil {
		return result, err
	}
	result.byDevice = make(map[string][]string)
	result.counts = make(map[string]int)
	for _, device := range devices {
		if !hwmonName.MatchString(device.Name()) {
			continue
		}
		base := filepath.Join(class, device.Name())
		resolved, err := filepath.EvalSymlinks(base)
		if err != nil {
			return sensorInventory{}, err
		}
		devicePath := strings.TrimPrefix(resolved, root)
		if !strings.HasPrefix(devicePath, "/devices/") || filepath.Base(devicePath) != device.Name() {
			return sensorInventory{}, errors.New("unexpected hwmon path")
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return sensorInventory{}, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "temp") || !strings.HasSuffix(name, "_input") {
				continue
			}
			key, err := readDirectTemperatureKey(base, name)
			if err != nil {
				return sensorInventory{}, err
			}
			result.byDevice[devicePath] = append(result.byDevice[devicePath], key)
			result.counts[key]++
		}
	}
	temperatures, err := host.SensorsTemperaturesWithContext(ctx)
	if err != nil {
		return sensorInventory{}, err
	}
	actual := make(map[string]int)
	for _, sensor := range temperatures {
		key := sensor.SensorKey
		// The ingestion schema persists at most 100 characters. Discard a
		// scan if a key could be transformed before reaching the catalog.
		if key == "" || strings.TrimSpace(key) != key || len(key) > 100 ||
			!utf8.ValidString(key) || strings.ContainsAny(key, "\x00\v\r\n") {
			return sensorInventory{}, errors.New("hwmon key cannot be matched to the catalog")
		}
		actual[key]++
	}
	// The fallback may report valid temperatures but has no direct hwmon
	// provenance. Retain its keys to prevent retiring a still-visible identity.
	if len(result.counts) == 0 {
		result.counts = actual
		return result, nil
	}
	for key, count := range actual {
		if result.counts[key] != count {
			return sensorInventory{}, errors.New("hwmon identity differs from temperature collector")
		}
	}
	if len(actual) != len(result.counts) {
		return sensorInventory{}, errors.New("hwmon inventory differs from temperature collector")
	}
	return result, nil
}

func readDirectTemperatureKey(base, input string) (string, error) {
	chip, err := os.ReadFile(filepath.Join(base, "name"))
	if err != nil {
		return "", err
	}
	value, err := os.ReadFile(filepath.Join(base, input))
	if err != nil {
		return "", err
	}
	temperature, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 64)
	if err != nil || math.IsNaN(temperature) || math.IsInf(temperature, 0) {
		return "", errors.New("invalid hwmon temperature")
	}
	prefix, _, _ := strings.Cut(input, "_")
	label, err := os.ReadFile(filepath.Join(base, prefix+"_label"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	key := strings.TrimSpace(string(chip))
	if suffix := strings.ReplaceAll(strings.TrimSpace(strings.ToLower(string(label))), " ", "_"); suffix != "" {
		key += "_" + suffix
	}
	return key, nil
}
