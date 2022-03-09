package provider

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/crypto/ssh"
)

func decodePEM(d *schema.ResourceData, pemKey, pemType string) (*pem.Block, error) {
	block, _ := pem.Decode([]byte(d.Get(pemKey).(string)))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", pemKey)
	}
	if pemType != "" && block.Type != pemType {
		return nil, fmt.Errorf("invalid PEM type in %s: %s", pemKey, block.Type)
	}

	return block, nil
}

func parsePrivateKey(d *schema.ResourceData, pemKey, algoKey string) (interface{}, error) {
	algoName := Algorithm(d.Get(algoKey).(string))

	keyFunc, ok := keyParsers[algoName]
	if !ok {
		return nil, fmt.Errorf("invalid %s: %#v", algoKey, algoName)
	}

	block, err := decodePEM(d, pemKey, "")
	if err != nil {
		return nil, err
	}

	key, err := keyFunc(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s: %s", pemKey, err)
	}

	return key, nil
}

func parseCertificate(d *schema.ResourceData, pemKey string) (*x509.Certificate, error) {
	block, err := decodePEM(d, pemKey, "")
	if err != nil {
		return nil, err
	}

	certs, err := x509.ParseCertificates(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %s", pemKey, err)
	}
	if len(certs) < 1 {
		return nil, fmt.Errorf("no certificates found in %s", pemKey)
	}
	if len(certs) > 1 {
		return nil, fmt.Errorf("multiple certificates found in %s", pemKey)
	}

	return certs[0], nil
}

func parseCertificateRequest(d *schema.ResourceData, pemKey string) (*x509.CertificateRequest, error) {
	block, err := decodePEM(d, pemKey, pemCertReqType)
	if err != nil {
		return nil, err
	}

	certReq, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %s", pemKey, err)
	}

	return certReq, nil
}

func readPublicKey(d *schema.ResourceData, prvKey interface{}) error {
	pubKey := publicKey(prvKey)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key error: %s", err)
	}
	pubKeyPemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}

	d.SetId(hashForState(string(pubKeyBytes)))

	if err := d.Set("public_key_pem", string(pem.EncodeToMemory(pubKeyPemBlock))); err != nil {
		return fmt.Errorf("error setting value on key 'public_key_pem': %s", err)
	}

	// NOTE: ECDSA keys with elliptic curve P-224 are not supported by `x/crypto/ssh`,
	// so this will return an error: in that case, we set the below fields to emptry strings
	sshPubKey, err := ssh.NewPublicKey(publicKey(prvKey))
	var pubKeySSH, pubKeySSHFingerprintMD5, pubKeySSHFingerprintSHA256 string
	if err == nil {
		sshPubKeyBytes := ssh.MarshalAuthorizedKey(sshPubKey)

		pubKeySSH = removeNewLine(string(sshPubKeyBytes))
		pubKeySSHFingerprintMD5 = ssh.FingerprintLegacyMD5(sshPubKey)
		pubKeySSHFingerprintSHA256 = ssh.FingerprintSHA256(sshPubKey)
	}

	if err := d.Set("public_key_openssh", pubKeySSH); err != nil {
		return fmt.Errorf("error setting value on key 'public_key_openssh': %s", err)
	}

	if err := d.Set("public_key_fingerprint_md5", pubKeySSHFingerprintMD5); err != nil {
		return fmt.Errorf("error setting value on key 'public_key_fingerprint_md5': %s", err)
	}

	if err := d.Set("public_key_fingerprint_sha256", pubKeySSHFingerprintSHA256); err != nil {
		return fmt.Errorf("error setting value on key 'public_key_fingerprint_sha256': %s", err)
	}

	return nil
}

func removeNewLine(pubKey string) string {
	return strings.TrimSuffix(pubKey, "\n") // remove newline
}
