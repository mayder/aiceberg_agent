package usecase

import "strings"

// Compare the numeric core and SemVer prerelease identifiers separately. Build
// metadata does not change precedence; a final release supersedes its candidate.
func compareVersionStrings(a, b string) int {
	acore, apre := updateVersionComponents(a)
	bcore, bpre := updateVersionComponents(b)
	as, bs := strings.Split(acore, "."), strings.Split(bcore, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		av, bv := "0", "0"
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if cmp := compareNumericVersion(av, bv); cmp != 0 {
			return cmp
		}
	}
	if apre == "" && bpre == "" {
		return 0
	}
	if apre == "" {
		return 1
	}
	if bpre == "" {
		return -1
	}
	return comparePrerelease(apre, bpre)
}

func updateVersionComponents(value string) (string, string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	withoutBuild, _, _ := strings.Cut(value, "+")
	core, pre, _ := strings.Cut(withoutBuild, "-")
	return core, pre
}

func compareNumericVersion(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < min(len(as), len(bs)); i++ {
		av, bv := as[i], bs[i]
		an, bn := numericVersionIdentifier(av), numericVersionIdentifier(bv)
		cmp := 0
		switch {
		case an && bn:
			cmp = compareNumericVersion(av, bv)
		case an:
			cmp = -1
		case bn:
			cmp = 1
		default:
			cmp = strings.Compare(av, bv)
		}
		if cmp != 0 {
			return cmp
		}
	}
	if len(as) < len(bs) {
		return -1
	}
	if len(as) > len(bs) {
		return 1
	}
	return 0
}

func numericVersionIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
