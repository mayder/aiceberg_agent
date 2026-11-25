package agentless

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const (
	jmxMaxJAR   = 256 << 10
	jmxMaxReply = 2048
)

var errJMXWorker = errors.New("jmx_worker_receipt_invalid")

type jmxWorkerReceipt struct {
	Status  string           `json:"status"`
	Code    string           `json:"code,omitempty"`
	Metrics map[string]int64 `json:"metrics,omitempty"`
}

func invokeJMXWorker(ctx context.Context, settings config.JMXProbeSettings, profile jmxProfile, budget int64) (jmxWorkerReceipt, error) {
	payload, err := safefile.ReadPrivate(settings.JAR, jmxMaxJAR)
	if err != nil || sha256.Sum256(payload) != settings.Digest {
		return jmxWorkerReceipt{}, errors.New("jmx_worker_integrity_failed")
	}
	privateDir, err := os.MkdirTemp("", "aiceberg-jmx-worker-")
	if err != nil {
		return jmxWorkerReceipt{}, err
	}
	defer os.RemoveAll(privateDir)
	jar := filepath.Join(privateDir, "worker.jar")
	if err := os.WriteFile(jar, payload, 0o600); err != nil {
		return jmxWorkerReceipt{}, err
	}

	input := jmxWorkerInput(profile, budget)
	defer clear(input)
	command := exec.CommandContext(ctx, settings.Java, "-Xmx32m", "-XX:ActiveProcessorCount=1", "-XX:-UsePerfData", "-jar", jar)
	command.Env = []string{}
	command.Dir = privateDir
	command.Stdin = bytes.NewReader(input)
	command.WaitDelay = 200 * time.Millisecond
	output := jmxWorkerOutput{}
	command.Stdout = &output
	runErr := command.Run()
	if err := ctx.Err(); err != nil {
		return jmxWorkerReceipt{}, err
	}
	if output.exceeded {
		return jmxWorkerReceipt{}, errJMXWorker
	}
	return decodeJMXWorkerReply(output.data.Bytes(), runErr)
}

func jmxWorkerInput(profile jmxProfile, budget int64) []byte {
	size := 22 + len(profile.Host) + len(profile.IP) + len(profile.User) + len(profile.Password) + len(profile.CA)
	data := binary.BigEndian.AppendUint32(make([]byte, 0, size), 0x41494a31)
	for _, value := range []string{profile.Host, profile.IP} {
		data = binary.BigEndian.AppendUint16(data, uint16(len(value)))
		data = append(data, value...)
	}
	data = binary.BigEndian.AppendUint16(data, uint16(profile.RegistryPort))
	data = binary.BigEndian.AppendUint16(data, uint16(profile.ServerPort))
	data = binary.BigEndian.AppendUint32(data, uint32(budget))
	for _, value := range []string{profile.User, profile.Password, profile.CA} {
		data = binary.BigEndian.AppendUint16(data, uint16(len(value)))
		data = append(data, value...)
	}
	return data
}

func decodeJMXWorkerReply(raw []byte, runErr error) (jmxWorkerReceipt, error) {
	var reply jmxWorkerReceipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&reply) != nil || decoder.Decode(new(any)) != io.EOF {
		return reply, errJMXWorker
	}
	if reply.Status == "ok" && runErr == nil && reply.Code == "" && len(reply.Metrics) >= 1 && len(reply.Metrics) <= 6 {
		for preset, value := range reply.Metrics {
			if !monitoring.ValidJMXMetric(preset) || value < 0 || value > 9007199254740991 {
				return jmxWorkerReceipt{}, errJMXWorker
			}
		}
		return reply, nil
	}
	var exit *exec.ExitError
	if reply.Status == "fail" && errors.As(runErr, &exit) && exit.ExitCode() == 1 && len(reply.Metrics) == 0 {
		switch reply.Code {
		case "invalid_profile", "endpoint_not_allowed", "response_rejected", "timeout", "tls_failed", "access_denied", "unavailable":
			return reply, nil
		}
	}
	return jmxWorkerReceipt{}, errJMXWorker
}

type jmxWorkerOutput struct {
	data     bytes.Buffer
	exceeded bool
}

func (out *jmxWorkerOutput) Write(data []byte) (int, error) {
	if len(data) > jmxMaxReply-out.data.Len() {
		out.exceeded = true
		return 0, errJMXWorker
	}
	return out.data.Write(data)
}
