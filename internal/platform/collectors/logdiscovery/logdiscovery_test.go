package logdiscovery

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestCollectEmitsContractWithDiscoveredLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "error.log")
	if err := os.WriteFile(logPath, []byte("2026-06-20 error failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newCollector(config.Config{
		LogDiscoveryEnabled:          true,
		LogDiscoveryInterval:         time.Minute,
		LogDiscoveryMaxCandidates:    20,
		LogDiscoveryMaxEvidenceBytes: 512,
	}, func() config.CollectPrefs {
		return config.CollectPrefs{LogDiscoveryEnabled: true}
	}, []knownPath{{
		Path:                logPath,
		Kind:                "log_file",
		Product:             "nginx",
		ServiceName:         "nginx",
		RecommendedCategory: "observability",
		SOCSourceType:       "none",
		SOCEligible:         "conditional",
		Permissions:         []string{"read:test"},
	}})
	c.now = func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }
	c.hostname = func() (string, error) { return "test-host", nil }

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	body := decoded[schemaVersion]
	if body.SchemaVersion != schemaVersion {
		t.Fatalf("schema version mismatch: %#v", body.SchemaVersion)
	}
	if body.Host != "test-host" || body.ScanPolicy["default_min_severity"] != "error" {
		t.Fatalf("unexpected host/policy: %#v", body)
	}
	if len(body.Candidates) == 0 {
		t.Fatalf("expected candidates: %#v", body)
	}
	got := body.Candidates[0]
	if got.Fingerprint == "" || got.Path != logPath || got.MinSeverity != "error" {
		t.Fatalf("invalid candidate: %#v", got)
	}
	if got.SOCEligible != "conditional" || got.RedactionPolicy == "" {
		t.Fatalf("missing governance fields: %#v", got)
	}
}

func TestCollectDisabledReturnsNil(t *testing.T) {
	c := newCollector(config.Config{LogDiscoveryEnabled: true}, func() config.CollectPrefs {
		return config.CollectPrefs{Version: "remote", LogDiscoveryEnabled: false}
	}, nil)
	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if raw != nil {
		t.Fatalf("expected nil payload when disabled, got %s", string(raw))
	}
}

func TestCollectDetectsContainerKubernetesAndOTLPRuntimeSignals(t *testing.T) {
	dir := t.TempDir()
	dockerSock := filepath.Join(dir, "docker.sock")
	containerdSock := filepath.Join(dir, "containerd.sock")
	kubeToken := filepath.Join(dir, "token")
	for _, path := range []string{dockerSock, containerdSock, kubeToken} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	c := newCollector(config.Config{
		LogDiscoveryEnabled:       true,
		ContainerDockerSocket:     dockerSock,
		ContainerContainerdSocket: containerdSock,
		KubernetesTokenPath:       kubeToken,
		OTLPEnabled:               true,
		OTLPHTTPAddr:              "127.0.0.1:4318",
	}, nil, nil)
	c.now = func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }
	c.hostname = func() (string, error) { return "runtime-host", nil }

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	body := decoded[schemaVersion]
	assertCandidateProduct(t, body.Candidates, "docker")
	assertCandidateProduct(t, body.Candidates, "containerd")
	assertCandidateProduct(t, body.Candidates, "kubernetes")
	assertCandidateProduct(t, body.Candidates, "opentelemetry")
	if body.Capabilities["docker_socket"] != true || body.Capabilities["kubernetes_token"] != true || body.Capabilities["otlp_enabled"] != true {
		t.Fatalf("unexpected capabilities: %#v", body.Capabilities)
	}
}

func TestCollectReportsPermissionGapForInaccessiblePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission fixture is POSIX-only")
	}
	dir := t.TempDir()
	blockedDir := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blockedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(blockedDir, "error.log")
	if err := os.WriteFile(logPath, []byte("error\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(blockedDir, 0o700)
	}()

	c := newCollector(config.Config{LogDiscoveryEnabled: true}, nil, []knownPath{{
		Path:                logPath,
		Kind:                "log_file",
		Product:             "nginx",
		ServiceName:         "nginx",
		RecommendedCategory: "observability",
		SOCSourceType:       "none",
		SOCEligible:         "conditional",
	}})
	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	body := decoded[schemaVersion]
	if len(body.Gaps) == 0 {
		t.Skip("filesystem allowed stat despite blocked directory")
	}
	if body.Gaps[0].Code != "path_permission_denied" || body.Gaps[0].Scope != logPath {
		t.Fatalf("unexpected permission gap: %#v", body.Gaps)
	}
}

func TestPKG74ControlledDiscoveryEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled chmod fixture is POSIX-only")
	}
	dir := t.TempDir()
	paths, blockedDir := pkg74ControlledPaths(t, dir)
	defer func() { _ = os.Chmod(blockedDir, 0o700) }()

	c := newCollector(config.Config{
		LogDiscoveryEnabled:          true,
		LogDiscoveryMaxCandidates:    300,
		LogDiscoveryMaxEvidenceBytes: 1024,
		ContainerDockerSocket:        filepath.Join(dir, "runtime", "docker.sock"),
		ContainerContainerdSocket:    filepath.Join(dir, "runtime", "containerd.sock"),
		KubernetesTokenPath:          filepath.Join(dir, "k8s", "token"),
		OTLPEnabled:                  true,
		OTLPHTTPAddr:                 "127.0.0.1:4318",
	}, nil, paths)
	c.now = func() time.Time { return time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC) }
	c.hostname = func() (string, error) { return "pkg74-controlled-host", nil }

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body := decodePKG74Body(t, raw)
	for _, product := range []string{"nginx", "apache", "iis", "application", "postgresql", "linux_auth", "docker", "containerd", "kubernetes", "opentelemetry"} {
		assertCandidateProduct(t, body.Candidates, product)
	}
	if !hasGap(body.Gaps, "path_permission_denied") {
		t.Fatalf("expected permission gap, got %#v", body.Gaps)
	}
	if strings.Contains(string(raw), "SHOULD_NOT_LEAK") || strings.Contains(string(raw), "super-secret") {
		t.Fatalf("raw evidence leaked secret: %s", string(raw))
	}
	if web, _ := classifyPort(8080); web != "web_server" {
		t.Fatalf("expected web_server port classification, got %q", web)
	}
	if db, _ := classifyPort(5432); db != "postgresql" {
		t.Fatalf("expected postgresql port classification, got %q", db)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG74_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG74Evidence(t, evidenceDir, raw, body)
	}
}

func TestFingerprintDeduplicatesSuperficialDuplicates(t *testing.T) {
	row := baseCandidate("log_file", "nginx")
	row.Path = "/var/log/nginx/error.log"
	row.ServiceName = "nginx"
	row.Fingerprint = fingerprint(row)

	dup := row
	dup.Evidence = []string{"size:10", "mtime:changed"}
	dup.Fingerprint = fingerprint(dup)

	c := newCollector(config.Config{}, nil, nil)
	rows := c.deduplicate([]Candidate{row, dup}, 10)
	if len(rows) != 1 {
		t.Fatalf("expected one deduped candidate, got %d", len(rows))
	}
}

func TestSanitizeTextRedactsSecretLikeArguments(t *testing.T) {
	got := sanitizeText("worker --token abc --password=secret --safe ok", 200)
	if got == "" || got == "worker --token abc --password=secret --safe ok" {
		t.Fatalf("expected redaction, got %q", got)
	}
	if got != "worker [redacted] [redacted] [redacted] --safe ok" {
		t.Fatalf("unexpected redacted value: %q", got)
	}
}

func assertCandidateProduct(t *testing.T, rows []Candidate, product string) {
	t.Helper()
	for _, row := range rows {
		if row.Product == product {
			if row.Fingerprint == "" || row.MinSeverity != "error" || row.RedactionPolicy == "" {
				t.Fatalf("candidate %s missing governance fields: %#v", product, row)
			}
			return
		}
	}
	t.Fatalf("candidate product %q not found in %#v", product, rows)
}

func pkg74ControlledPaths(t *testing.T, dir string) ([]knownPath, string) {
	t.Helper()
	files := map[string]string{
		"nginx/error.log":         "2026/06/20 18:00:00 [error] upstream timed out\n",
		"apache/error.log":        "[Sat Jun 20 18:00:00] [error] proxy timeout\n",
		"iis/W3SVC1/u_ex.log":     "2026-06-20 18:00:00 500 /lg/ponto\n",
		"lg-app/error.log":        "ERROR LG folha ponto slow request db timeout token=SHOULD_NOT_LEAK\n",
		"postgres/postgresql.log": "ERROR: canceling statement due to statement timeout\n",
		"auth.log":                "sshd: Failed password for invalid user\n",
		"runtime/docker.sock":     "docker",
		"runtime/containerd.sock": "containerd",
		"k8s/token":               "kubernetes-token",
	}
	for name, content := range files {
		writePKG74File(t, filepath.Join(dir, name), content)
	}
	blockedDir := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(blockedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	blockedPath := filepath.Join(blockedDir, "secret.log")
	writePKG74File(t, blockedPath, "ERROR blocked\n")
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatal(err)
	}
	return []knownPath{
		pkg74Path(dir, "nginx/error.log", "log_file", "nginx", "nginx", "observability", "none", "conditional"),
		pkg74Path(dir, "apache/error.log", "log_file", "apache", "apache", "observability", "none", "conditional"),
		pkg74Path(dir, "iis/W3SVC1/*.log", "log_glob", "iis", "iis", "observability", "waf", "conditional"),
		pkg74Path(dir, "lg-app/error.log", "log_file", "application", "lg-folha-ponto", "observability", "none", "conditional"),
		pkg74Path(dir, "postgres/*.log", "log_glob", "postgresql", "postgresql", "observability", "none", "conditional"),
		pkg74Path(dir, "auth.log", "log_file", "linux_auth", "auth", "soc", "linux_security", "yes"),
		{Path: blockedPath, Kind: "log_file", Product: "application", ServiceName: "blocked", RecommendedCategory: "observability", SOCSourceType: "none", SOCEligible: "conditional"},
	}, blockedDir
}

func pkg74Path(root, rel, kind, product, service, category, socType, socEligible string) knownPath {
	return knownPath{Path: filepath.Join(root, rel), Kind: kind, Product: product, ServiceName: service, RecommendedCategory: category, SOCSourceType: socType, SOCEligible: socEligible, Permissions: []string{"read:" + service}}
}

func writePKG74File(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func decodePKG74Body(t *testing.T, raw []byte) payload {
	t.Helper()
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded[schemaVersion]
}

func hasGap(items []gap, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func writePKG74Evidence(t *testing.T, dir string, raw []byte, body payload) {
	t.Helper()
	rawDir := filepath.Join(dir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(rawDir, "pkg74-discovery-controlled-raw.tgz")
	if err := writePKG74TarGZ(archivePath, map[string][]byte{"pkg74-discovery-controlled.json": raw}); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "evidence.md")
	evidence := pkg74EvidenceMarkdown(body)
	if err := os.WriteFile(evidencePath, []byte(evidence), 0o644); err != nil {
		t.Fatal(err)
	}
	writePKG74Provenance(t, dir)
	writePKG74Manifest(t, dir, evidencePath, archivePath)
}

func pkg74EvidenceMarkdown(body payload) string {
	products := map[string]bool{}
	for _, candidate := range body.Candidates {
		products[candidate.Product] = true
	}
	return strings.Join([]string{
		"# Evidencia PKG-74 - Discovery controlado",
		"",
		"- Data UTC: 2026-06-20T18:00:00Z",
		"- Cenario: descoberta de fontes para aplicacao lenta com web/app/banco/rede, runtime e seguranca",
		"- Ambiente: teste controlado Go " + runtime.GOOS + "/" + runtime.GOARCH,
		"- Host controlado: " + body.Host,
		fmt.Sprintf("- Candidatos descobertos: %d", len(body.Candidates)),
		fmt.Sprintf("- Lacunas registradas: %d", len(body.Gaps)),
		"- Produtos cobertos: " + strings.Join(pkg74ProductList(products), ", "),
		"- Politica: bounded=true, read_only=true, min_severity=error, activates_collection=false",
		"- Redaction: token/senha sensivel ausente do payload bruto",
		"- Rede: portas 8080 e 5432 classificadas como web_server e postgresql por contrato",
		"- Evidencia bruta anexada: raw/pkg74-discovery-controlled-raw.tgz",
		"",
		"Esta evidencia cobre o cenario controlado de troubleshooting de aplicacao lenta sem declarar causa raiz. IIS e Kubernetes reais continuam dependentes de ambiente com esses componentes presentes.",
		"",
	}, "\n")
}

func pkg74ProductList(products map[string]bool) []string {
	out := []string{}
	for product := range products {
		out = append(out, product)
	}
	sort.Strings(out)
	return out
}

func writePKG74Provenance(t *testing.T, dir string) {
	t.Helper()
	provenance := strings.Join([]string{
		"key\tvalue",
		"scenario\tpkg74-discovery-controlled",
		"created_at_utc\t20260620T180000Z",
		"command\tPKG74_EVIDENCE_DIR=" + dir + " go test ./internal/platform/collectors/logdiscovery -run TestPKG74ControlledDiscoveryEvidence -count=1 -v",
		"goos\t" + runtime.GOOS,
		"goarch\t" + runtime.GOARCH,
		"raw_archive\traw/pkg74-discovery-controlled-raw.tgz",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "PROVENANCE.tsv"), []byte(provenance), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePKG74Manifest(t *testing.T, dir, evidencePath, rawPath string) {
	t.Helper()
	manifest := fmt.Sprintf(
		"scenario\tevidence_path\tevidence_sha256\tevidence_bytes\traw_path\traw_sha256\traw_bytes\tcreated_at_utc\n%s\t%s\t%s\t%d\t%s\t%s\t%d\t%s\n",
		"pkg74-discovery-controlled",
		"docs/evidence/pkg74/discovery-controlled-20260620T180000Z/evidence.md",
		pkg74FileSHA256(t, evidencePath), pkg74FileSize(t, evidencePath),
		"docs/evidence/pkg74/discovery-controlled-20260620T180000Z/raw/pkg74-discovery-controlled-raw.tgz",
		pkg74FileSHA256(t, rawPath), pkg74FileSize(t, rawPath),
		"20260620T180000Z",
	)
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST.tsv"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePKG74TarGZ(path string, files map[string][]byte) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for name, content := range files {
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), ModTime: time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC)}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(content); err != nil {
			return err
		}
	}
	return nil
}

func pkg74FileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func pkg74FileSize(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return int(info.Size())
}
