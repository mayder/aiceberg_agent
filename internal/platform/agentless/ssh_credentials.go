package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/crypto/ssh"
)

type sshProbeAddress struct {
	ip   netip.Addr
	port int
}

func (a sshProbeAddress) String() string {
	return net.JoinHostPort(a.ip.String(), strconv.Itoa(a.port))
}

func sshProbeEndpoint(endpoint *entities.AgentlessEndpoint) (sshProbeAddress, bool) {
	if endpoint == nil || endpoint.ID < 1 || endpoint.Tipo != "ip" {
		return sshProbeAddress{}, false
	}
	ip, err := netip.ParseAddr(endpoint.Endereco)
	if err != nil || ip.Zone() != "" || !probeHTTPAddressAllowed(endpoint.Endereco, ip.Unmap()) {
		return sshProbeAddress{}, false
	}
	port := 22
	if endpoint.Porta != nil {
		port = *endpoint.Porta
	}
	return sshProbeAddress{ip.Unmap(), port}, port > 0 && port <= 65535
}

type sshProbeProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	HostKey    string `json:"host_key"`
	PrivateKey string `json:"private_key"`
	Passphrase string `json:"passphrase,omitempty"`
}

var sshProbeUsername = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`)

func loadSSHProfile(path string, job entities.AgentlessJob, selection monitoring.SSHSelection, target sshProbeAddress) (sshProbeProfile, bool) {
	if job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 || job.ProbePolicy == nil ||
		job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return sshProbeProfile{}, false
	}
	raw, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return sshProbeProfile{}, false
	}
	defer clear(raw)
	var file struct {
		Profiles []sshProbeProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return sshProbeProfile{}, false
	}
	var match *sshProbeProfile
	for _, profile := range file.Profiles {
		if profile.Reference != selection.CredentialRef {
			continue
		}
		if match != nil {
			return sshProbeProfile{}, false
		}
		match = &profile
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.IP != target.ip.String() || match.Port != target.port ||
		!sshProbeUsername.MatchString(match.Username) || len(match.HostKey) > 8192 || len(match.PrivateKey) > 32768 ||
		len(match.Passphrase) > 1024 {
		return sshProbeProfile{}, false
	}
	return *match, true
}

func (p sshProbeProfile) clientConfig() (*ssh.ClientConfig, bool) {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(p.HostKey))
	if err != nil || len(options) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, false
	}
	var algorithms []string
	switch key.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		algorithms = []string{key.Type()}
	case ssh.KeyAlgoRSA:
		algorithms = []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	default:
		return nil, false
	}
	var signer ssh.Signer
	if p.Passphrase == "" {
		signer, err = ssh.ParsePrivateKey([]byte(p.PrivateKey))
	} else {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(p.PrivateKey), []byte(p.Passphrase))
	}
	if err != nil {
		return nil, false
	}
	pin := ssh.FixedHostKey(key)
	return &ssh.ClientConfig{
		User:              p.Username,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyAlgorithms: algorithms,
		HostKeyCallback: func(host string, address net.Addr, presented ssh.PublicKey) error {
			if pin(host, address, presented) != nil {
				return errSSHHostKey
			}
			return nil
		},
	}, true
}
