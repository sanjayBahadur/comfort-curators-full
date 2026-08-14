package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readFileFromRepoRoot(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "compose.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func composeServiceBlock(t *testing.T, content, name string) string {
	t.Helper()
	lines := strings.Split(content, "\n")
	var block []string
	collecting := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if collecting {
				block = append(block, line)
			}
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			if collecting {
				return strings.Join(block, "\n")
			}
			continue
		}
		if indent == 2 && !strings.HasPrefix(trimmed, "- ") {
			if collecting && strings.TrimSuffix(trimmed, ":") != name {
				return strings.Join(block, "\n")
			}
			collecting = strings.TrimSuffix(trimmed, ":") == name
			if collecting {
				block = []string{line}
			}
			continue
		}
		if collecting {
			block = append(block, line)
		}
	}
	if collecting {
		return strings.Join(block, "\n")
	}
	return ""
}

func composeDependsOn(block string) map[string]bool {
	deps := map[string]bool{}
	lines := strings.Split(block, "\n")
	inDepends := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "depends_on:") {
			inDepends = true
			continue
		}
		if !inDepends {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent < 4 {
			inDepends = false
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		key = strings.TrimSuffix(key, ":")
		if key != "" {
			deps[key] = true
		}
	}
	return deps
}

func TestComposeDefinesRequiredServices(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	for _, name := range []string{"api", "worker", "postgres", "acceptance-seed", "minio", "model-stub"} {
		if block := composeServiceBlock(t, content, name); block == "" {
			t.Errorf("compose.yaml must define service %q", name)
		}
	}
}

func TestComposeAPIPublishesLocalhostHTTPPort(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "api")
	if !strings.Contains(block, `127.0.0.1:${CC_HTTP_PORT:-8080}:8080`) {
		t.Error("api must publish 127.0.0.1:${CC_HTTP_PORT:-8080}:8080")
	}
	if !strings.Contains(block, ":8080") {
		t.Error("api must keep the container port 8080")
	}
	if strings.Contains(block, "0.0.0.0:") {
		t.Error("api must not publish on 0.0.0.0")
	}
}

func TestComposeAPIAndWorkerStartIndependently(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")

	apiDeps := composeDependsOn(composeServiceBlock(t, content, "api"))
	if apiDeps["worker"] {
		t.Error("api must not depend on worker")
	}

	workerDeps := composeDependsOn(composeServiceBlock(t, content, "worker"))
	if workerDeps["api"] {
		t.Error("worker must not depend on api")
	}
}

func TestComposeHasNoGloballyNamedRuntimeResources(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	if strings.Contains(content, "container_name:") {
		t.Error("compose.yaml must not set container_name")
	}
	if strings.Contains(content, "network_mode") {
		t.Error("compose.yaml must not use host or custom network modes")
	}
	if strings.Contains(content, "external:") {
		t.Error("compose.yaml must not reference external networks or volumes")
	}
}

func TestComposeAcceptanceSeedHonorsFixtureProtocol(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "acceptance-seed")
	if block == "" {
		t.Fatal("compose.yaml must define service acceptance-seed")
	}
	if !strings.Contains(block, "profiles:") || !strings.Contains(block, "acceptance") {
		t.Error("acceptance-seed must be gated behind the acceptance profile")
	}
	if !strings.Contains(block, "CC_ENV=acceptance") {
		t.Error("acceptance-seed must run with CC_ENV=acceptance")
	}
	if !strings.Contains(block, "CC_FIXTURE_PATH") {
		t.Error("acceptance-seed must be configured to write the fixture file")
	}
	if strings.Contains(block, "ports:") {
		t.Error("acceptance-seed must not publish any ports")
	}
}

func TestDockerfileBuildsBinariesWithoutHostGo(t *testing.T) {
	lines := strings.Split(readFileFromRepoRoot(t, "Dockerfile"), "\n")

	builderLine := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "FROM golang:") {
			builderLine = i
			break
		}
	}
	if builderLine == -1 {
		t.Fatal("Dockerfile must build from a golang builder image")
	}

	firstRuntimeLine := -1
	for i := builderLine + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "FROM ") && strings.Contains(lines[i], " AS ") {
			firstRuntimeLine = i
			break
		}
	}
	if firstRuntimeLine == -1 {
		t.Fatal("Dockerfile must define at least one runtime stage after the builder")
	}

	builderStage := strings.Join(lines[builderLine:firstRuntimeLine], "\n")
	for _, binary := range []string{"/bin/api", "/bin/worker", "/bin/acceptance-seed", "/bin/model-stub"} {
		// api/worker take an optional BUILD_TAGS arg between "go build" and
		// "-o" (empty by default -- see the Dockerfile comment -- non-empty
		// only for the acceptance-only session-fixture route), so this
		// checks each half is present on the same line rather than
		// requiring them adjacent.
		found := false
		for _, line := range strings.Split(builderStage, "\n") {
			if strings.Contains(line, "go build") && strings.Contains(line, "-o "+binary) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Dockerfile builder stage must compile %s", binary)
		}
	}

	runtimeStages := strings.Join(lines[firstRuntimeLine:], "\n")
	if strings.Contains(runtimeStages, "go build") {
		t.Error("runtime stages must not compile Go; binaries must be copied from the builder")
	}
	for _, target := range []string{"AS api", "AS worker", "AS seed", "AS model-stub"} {
		if !strings.Contains(runtimeStages, target) {
			t.Errorf("Dockerfile must define the %q runtime target", target)
		}
		if !strings.Contains(runtimeStages, "COPY --from=builder") {
			t.Errorf("runtime target %q must copy binaries from the builder", target)
		}
	}
}

func TestDockerComposeConfigValid(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	cmd := exec.Command("docker", "compose", "-f", "compose.yaml", "config", "-q")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, out)
	}
}

func TestComposeModelStubPublishesLocalhostBinding(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "model-stub")
	if block == "" {
		t.Fatal("compose.yaml must define service model-stub")
	}
	if !strings.Contains(block, `127.0.0.1:${CC_MODEL_PORT:-8081}:8080`) {
		t.Error("model-stub must publish 127.0.0.1:${CC_MODEL_PORT:-8081}:8080")
	}
	if !strings.Contains(block, ":8080") {
		t.Error("model-stub must keep the container port 8080")
	}
	if strings.Contains(block, "0.0.0.0:") {
		t.Error("model-stub must not publish on 0.0.0.0")
	}
}

func TestComposeModelStubHasHealthcheck(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "model-stub")
	if !strings.Contains(block, "healthcheck") {
		t.Error("model-stub must define a healthcheck")
	}
	if !strings.Contains(block, "/health/live") {
		t.Error("model-stub healthcheck must probe /health/live")
	}
}

func TestComposeModelStubStartsIndependently(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	deps := composeDependsOn(composeServiceBlock(t, content, "model-stub"))
	if len(deps) > 0 {
		t.Error("model-stub must not depend on other services")
	}
}

func TestComposeMinioIsPinnedImage(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "minio")
	if block == "" {
		t.Fatal("compose.yaml must define service minio")
	}
	if !strings.Contains(block, "RELEASE.") || !strings.Contains(block, "minio/minio:") {
		t.Error("minio must use a pinned release image tag")
	}
}

func TestComposeMinioPublishesLocalhostBindings(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "minio")
	if !strings.Contains(block, `127.0.0.1:${CC_S3_PORT:-9000}:9000`) {
		t.Error("minio must publish 127.0.0.1 bound S3 port")
	}
	if !strings.Contains(block, `127.0.0.1:${CC_MINIO_CONSOLE_PORT:-9001}:9001`) {
		t.Error("minio must publish 127.0.0.1 bound console port")
	}
	if strings.Contains(block, "0.0.0.0:") {
		t.Error("minio must not publish on 0.0.0.0")
	}
}

func TestComposeMinioHasHealthcheck(t *testing.T) {
	content := readFileFromRepoRoot(t, "compose.yaml")
	block := composeServiceBlock(t, content, "minio")
	if !strings.Contains(block, "healthcheck") {
		t.Error("minio must define a healthcheck")
	}
	// The minio image has no curl/wget, only mc (its own client) and
	// busybox utilities -- a curl-based check against /minio/health/live
	// always fails with "executable file not found", permanently marking
	// the container unhealthy regardless of whether minio is actually
	// serving traffic. "mc ready" is the client the image actually ships.
	if !strings.Contains(block, "mc") || !strings.Contains(block, "ready") {
		t.Error("minio healthcheck must use the mc client (curl/wget are not present in this image)")
	}
}
