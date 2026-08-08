package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/updatetrust"
)

type artifactFlag []string

func (f *artifactFlag) String() string { return strings.Join(*f, ",") }
func (f *artifactFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type artifactSignature struct {
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

type signatureManifest struct {
	SchemaVersion      string                       `json:"schema_version"`
	Version            string                       `json:"version"`
	SignatureAlgorithm string                       `json:"signature_algorithm"`
	SigningKeyID       string                       `json:"signing_key_id"`
	Artifacts          map[string]artifactSignature `json:"artifacts"`
}

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("use generate, sign ou verify"))
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = generate(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		err = fmt.Errorf("comando desconhecido: %s", os.Args[1])
	}
	if err != nil {
		fatal(err)
	}
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	privatePath := fs.String("private-key", "", "arquivo PEM privado")
	publicPath := fs.String("public-key", "", "arquivo PEM público")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*privatePath) == "" || strings.TrimSpace(*publicPath) == "" {
		return errors.New("private-key e public-key são obrigatórios")
	}
	if fileExists(*privatePath) || fileExists(*publicPath) {
		return errors.New("recusado: arquivo de chave já existe")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return err
	}
	if err := writeExclusive(*privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		return err
	}
	if err := writeExclusive(*publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o644); err != nil {
		_ = os.Remove(*privatePath)
		return err
	}
	fmt.Printf("public_key_hex=%s\n", hex.EncodeToString(publicKey))
	return nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	privatePath := fs.String("private-key", "", "arquivo PEM privado")
	version := fs.String("version", "", "versão do agente")
	keyID := fs.String("key-id", updatetrust.OfficialKeyID, "identificador público da chave")
	output := fs.String("output", "UPDATE_SIGNATURES.json", "manifesto de saída")
	var artifacts artifactFlag
	fs.Var(&artifacts, "artifact", "artefato a assinar; repetível")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*privatePath) == "" || strings.TrimSpace(*version) == "" || len(artifacts) == 0 {
		return errors.New("private-key, version e ao menos um artifact são obrigatórios")
	}
	privateKey, err := loadPrivateKey(*privatePath)
	if err != nil {
		return err
	}
	manifest := signatureManifest{
		SchemaVersion:      "aiceberg-agent-update-signatures.v1",
		Version:            strings.TrimSpace(*version),
		SignatureAlgorithm: updatetrust.Algorithm,
		SigningKeyID:       strings.TrimSpace(*keyID),
		Artifacts:          make(map[string]artifactSignature, len(artifacts)),
	}
	sort.Strings(artifacts)
	for _, artifactPath := range artifacts {
		name, item, err := signArtifact(privateKey, manifest, artifactPath)
		if err != nil {
			return err
		}
		if _, exists := manifest.Artifacts[name]; exists {
			return fmt.Errorf("artefato duplicado: %s", name)
		}
		manifest.Artifacts[name] = item
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(*output, raw, 0o644)
}

func signArtifact(privateKey ed25519.PrivateKey, manifest signatureManifest, artifactPath string) (string, artifactSignature, error) {
	name := filepath.Base(artifactPath)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "", artifactSignature{}, fmt.Errorf("nome de artefato inválido: %s", artifactPath)
	}
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		return "", artifactSignature{}, err
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	message := updatetrust.Message(manifest.Version, sha, manifest.SigningKeyID)
	signature := ed25519.Sign(privateKey, []byte(message))
	return name, artifactSignature{SHA256: sha, Signature: hex.EncodeToString(signature)}, nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "manifesto JSON")
	publicPath := fs.String("public-key", "", "arquivo PEM público")
	artifactDir := fs.String("artifact-dir", "", "diretório dos artefatos")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *publicPath == "" || *artifactDir == "" {
		return errors.New("manifest, public-key e artifact-dir são obrigatórios")
	}
	publicKey, err := loadPublicKey(*publicPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var manifest signatureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != "aiceberg-agent-update-signatures.v1" || len(manifest.Artifacts) == 0 {
		return errors.New("manifesto inválido ou vazio")
	}
	for name, item := range manifest.Artifacts {
		if filepath.Base(name) != name {
			return fmt.Errorf("nome inseguro no manifesto: %s", name)
		}
		artifactRaw, err := os.ReadFile(filepath.Join(*artifactDir, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(artifactRaw)
		sha := hex.EncodeToString(sum[:])
		if sha != strings.ToLower(item.SHA256) {
			return fmt.Errorf("sha256 divergente: %s", name)
		}
		if err := updatetrust.Verify(hex.EncodeToString(publicKey), manifest.Version, sha, manifest.SignatureAlgorithm, item.Signature, manifest.SigningKeyID); err != nil {
			return fmt.Errorf("assinatura inválida %s: %w", name, err)
		}
	}
	fmt.Printf("verified=%d version=%s key_id=%s\n", len(manifest.Artifacts), manifest.Version, manifest.SigningKeyID)
	return nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("chave privada PEM inválida")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("chave privada não é Ed25519")
	}
	return privateKey, nil
}

func loadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("chave pública PEM inválida")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	publicKey, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("chave pública não é Ed25519")
	}
	return publicKey, nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
