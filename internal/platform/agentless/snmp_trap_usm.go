package agentless

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type SNMPTrapUSMProfile struct {
	EngineID     string `json:"engine_id"`
	User         string `json:"user"`
	AuthProtocol string `json:"auth_protocol"`
	AuthPassword string `json:"auth_password"`
	PrivPassword string `json:"priv_password"`
	Context      string `json:"context_name,omitempty"`
}

type trapUSM struct {
	decoder   gosnmp.GoSNMP
	security  *gosnmp.UsmSecurityParameters
	authHash  crypto.Hash
	macSize   int
	engineKey [32]byte
	context   string
}

func newTrapUSM(profile *SNMPTrapUSMProfile) (*trapUSM, error) {
	if profile == nil {
		return nil, ErrSNMPTrapConfig
	}
	engine, err := hex.DecodeString(profile.EngineID)
	if err != nil || len(engine) < 5 || len(engine) > 32 || !validTrapUSMName(profile.User, false) ||
		!validTrapUSMName(profile.Context, true) || !validTrapPassword(profile.AuthPassword) ||
		!validTrapPassword(profile.PrivPassword) || profile.AuthPassword == profile.PrivPassword {
		return nil, ErrSNMPTrapConfig
	}
	protocol, hash, macSize := trapAuthAlgorithm(profile.AuthProtocol)
	if !hash.Available() {
		return nil, ErrSNMPTrapConfig
	}
	security := &gosnmp.UsmSecurityParameters{
		AuthoritativeEngineID: string(engine), UserName: profile.User, AuthenticationProtocol: protocol,
		AuthenticationPassphrase: profile.AuthPassword, PrivacyProtocol: gosnmp.AES, PrivacyPassphrase: profile.PrivPassword,
	}
	if err := security.InitSecurityKeys(); err != nil {
		return nil, ErrSNMPTrapConfig
	}
	security.AuthenticationPassphrase, security.PrivacyPassphrase = "", ""
	return &trapUSM{
		decoder:  gosnmp.GoSNMP{Version: gosnmp.Version3, MsgFlags: gosnmp.AuthPriv, SecurityModel: gosnmp.UserSecurityModel, SecurityParameters: security},
		security: security, authHash: hash, macSize: macSize, engineKey: sha256.Sum256(engine), context: profile.Context,
	}, nil
}

func trapAuthAlgorithm(name string) (gosnmp.SnmpV3AuthProtocol, crypto.Hash, int) {
	switch name {
	case "sha":
		return gosnmp.SHA, crypto.SHA1, 12
	case "sha224":
		return gosnmp.SHA224, crypto.SHA224, 16
	case "sha256":
		return gosnmp.SHA256, crypto.SHA256, 24
	case "sha384":
		return gosnmp.SHA384, crypto.SHA384, 32
	case "sha512":
		return gosnmp.SHA512, crypto.SHA512, 48
	default:
		return gosnmp.NoAuth, 0, 0
	}
}

func validTrapUSMName(text string, optional bool) bool {
	if len(text) > 32 || (!optional && text == "") || !utf8.ValidString(text) {
		return false
	}
	return strings.IndexFunc(text, unicode.IsControl) < 0
}

func validTrapPassword(text string) bool {
	return len(text) >= 8 && len(text) <= 128 && !strings.ContainsRune(text, 0)
}

func (u *trapUSM) decode(raw []byte, now time.Time) (*gosnmp.SnmpPacket, *entities.SNMPTrapReplayProof) {
	fields, offset, ok := u.envelope(raw)
	if !ok {
		return nil, nil
	}
	// Verify a fixed-size MAC before using the SNMP parser or decrypting the scoped PDU.
	mac := hmac.New(u.authHash.New, u.security.SecretKey)
	_, _ = mac.Write(raw[:offset])
	var empty [48]byte
	_, _ = mac.Write(empty[:u.macSize])
	_, _ = mac.Write(raw[offset+u.macSize:])
	if !hmac.Equal(fields.auth, mac.Sum(nil)[:u.macSize]) {
		return nil, nil
	}
	proof := &entities.SNMPTrapReplayProof{Engine: u.engineKey,
		Boots: uint32(fields.boots), EngineTime: uint32(fields.engineTime), Digest: sha256.Sum256(raw), ReceivedAt: now.Unix()}
	packet := parseAuthenticatedTrap(&u.decoder, raw)
	if packet == nil || packet.PDUType != gosnmp.SNMPv2Trap || packet.ContextEngineID != u.security.AuthoritativeEngineID ||
		packet.ContextName != u.context {
		return nil, nil
	}
	return packet, proof
}

// Keep third-party parsing errors within the datagram boundary. For example, a
// valid authentication key and mismatched privacy key can yield invalid BER.
func parseAuthenticatedTrap(decoder *gosnmp.GoSNMP, raw []byte) (packet *gosnmp.SnmpPacket) {
	defer func() {
		if recover() != nil {
			packet = nil
		}
	}()
	packet, err := decoder.UnmarshalTrap(raw, true)
	if err != nil {
		return nil
	}
	return packet
}

type trapUSMFields struct {
	engine, user, auth, salt []byte
	boots, engineTime        int64
}

func (u *trapUSM) envelope(raw []byte) (trapUSMFields, int, bool) {
	var outer, header asn1.RawValue
	var version int
	var sec, scoped []byte
	tail, err := asn1.Unmarshal(raw, &outer)
	if err != nil || len(tail) != 0 || !isTrapSequence(outer) {
		return trapUSMFields{}, 0, false
	}
	tail, err = asn1.Unmarshal(outer.Bytes, &version)
	if err != nil || version != int(gosnmp.Version3) {
		return trapUSMFields{}, 0, false
	}
	tail, err = asn1.Unmarshal(tail, &header)
	if err != nil || !isTrapSequence(header) || !validTrapUSMHeader(header.Bytes) {
		return trapUSMFields{}, 0, false
	}
	tail, err = asn1.Unmarshal(tail, &sec)
	if err != nil {
		return trapUSMFields{}, 0, false
	}
	securityEnd := len(raw) - len(tail)
	left, err := asn1.Unmarshal(tail, &scoped)
	if err != nil || len(left) != 0 || len(scoped) == 0 {
		return trapUSMFields{}, 0, false
	}
	fields, distance, ok := readTrapUSMFields(sec)
	if !ok || string(fields.engine) != u.security.AuthoritativeEngineID || string(fields.user) != u.security.UserName ||
		len(fields.auth) != u.macSize || len(fields.salt) != 8 {
		return trapUSMFields{}, 0, false
	}
	return fields, securityEnd - distance, true
}

func validTrapUSMHeader(raw []byte) bool {
	var id, size, model int64
	var flags []byte
	tail, err := readTrapASNFields(raw, &id, &size, &flags, &model)
	return err == nil && len(tail) == 0 && id >= 0 && id <= math.MaxInt32 &&
		size >= 484 && size <= math.MaxInt32 && len(flags) == 1 && flags[0] == byte(gosnmp.AuthPriv) &&
		model == int64(gosnmp.UserSecurityModel)
}

func readTrapUSMFields(raw []byte) (trapUSMFields, int, bool) {
	var seq asn1.RawValue
	var fields trapUSMFields
	tail, err := asn1.Unmarshal(raw, &seq)
	if err != nil || len(tail) != 0 || !isTrapSequence(seq) {
		return fields, 0, false
	}
	tail, err = readTrapASNFields(seq.Bytes, &fields.engine, &fields.boots, &fields.engineTime, &fields.user, &fields.auth)
	if err != nil || fields.boots < 0 || fields.boots >= math.MaxInt32 || fields.engineTime < 0 || fields.engineTime > math.MaxInt32 {
		return fields, 0, false
	}
	distance := len(tail) + len(fields.auth)
	left, err := asn1.Unmarshal(tail, &fields.salt)
	return fields, distance, err == nil && len(left) == 0
}

func readTrapASNFields(raw []byte, values ...any) ([]byte, error) {
	for _, value := range values {
		tail, err := asn1.Unmarshal(raw, value)
		if err != nil {
			return nil, err
		}
		raw = tail
	}
	return raw, nil
}

func isTrapSequence(value asn1.RawValue) bool {
	return value.Tag == asn1.TagSequence && value.Class == asn1.ClassUniversal && value.IsCompound
}
