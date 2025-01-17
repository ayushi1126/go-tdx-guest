// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package verify provides the library functions to verify a TDX quote
package verify

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"reflect"
	"time"

	"github.com/google/go-tdx-guest/abi"
	"github.com/google/go-tdx-guest/pcs"
	pb "github.com/google/go-tdx-guest/proto/tdx"
	"github.com/google/go-tdx-guest/verify/trust"
	"github.com/google/logger"
	"go.uber.org/multierr"
)

var (

	// Embedded certificate used when trusted root is nil
	//go:embed trusted_root.pem
	defaultRootCertByte []byte

	trustedRootCertificate *x509.Certificate

	// ErrOptionsNil error returned when options parameter is empty
	ErrOptionsNil = errors.New("options parameter is empty")
	// ErrPCKCertChainNil error returned when PCK certificate chain field is empty in quote
	ErrPCKCertChainNil = errors.New("PCK certificate chain is empty")
	// ErrPCKCertChainInvalid error returned when PCK certificate chain has incomplete certificates
	ErrPCKCertChainInvalid = errors.New("incomplete PCK Certificate chain found, should contain 3 concatenated PEM-formatted 'CERTIFICATE'-type block (PCK Leaf Cert||Intermediate CA Cert||Root CA Cert)")
	// ErrRootCertNil error returned when Root CA certificate is empty
	ErrRootCertNil = errors.New("root certificate is empty")
	// ErrPCKCertNil error returned when PCK leaf certificate is empty
	ErrPCKCertNil = errors.New("PCK certificate is empty")
	// ErrIntermediateCertNil error returned when Intermediate CA certificate is empty
	ErrIntermediateCertNil = errors.New("intermediate certificate is empty")
	// ErrCertPubKeyType error returned when certificate public key is not of type ecdsa
	ErrCertPubKeyType = errors.New("certificate public key is not of type ecdsa public key")
	// ErrPublicKeySize error returned when public key bytes are of unexpected size
	ErrPublicKeySize = errors.New("public key is of unexpected size")
	// ErrKeyMismatch error returned when trusted public key is different from root CA certificate's public key
	ErrKeyMismatch = errors.New("root certificate's public key does not match with trusted public key")
	// ErrHashVerificationFail error returned when message digest verification failed using quote's
	ErrHashVerificationFail = errors.New("unable to verify message digest using quote's signature and ecdsa attestation key")
	// ErrSHA56VerificationFail error returned when sha256 verification fails
	ErrSHA56VerificationFail = errors.New("QE Report Data does not match with value of SHA 256 calculated over the concatenation of ECDSA Attestation Key and QE Authenticated Data")
	// ErrPckCertCANil error returned when CA is missing in PCK Certificate
	ErrPckCertCANil = errors.New("could not find CA from PCK certificate")
	// ErrEmptyRootCRLUrl error returned when QE identity issuer's chain root certificate has empty CRL distribution points
	ErrEmptyRootCRLUrl = errors.New("empty url found in QeIdentity issuer's chain which is required to receive ROOT CA CRL")
	// ErrCollateralNil error returned when collaterals are missing
	ErrCollateralNil = errors.New("collateral received is an empty structure")
	// ErrMissingPckCrl error returned when PCK CRL is missing
	ErrMissingPckCrl = errors.New("missing PCK CRL in the collaterals obtained")
	// ErrMissingRootCaCrl error returned when Root CA CRL CRL is missing
	ErrMissingRootCaCrl = errors.New("missing ROOT CA CRL in the collaterals obtained")
	// ErrMissingTcbInfoBody error returned when TCB info body is missing
	ErrMissingTcbInfoBody = errors.New("missing tcbInfo body in the collaterals obtained")
	// ErrMissingEnclaveIdentityBody error returned when Enclave Identity body is missing
	ErrMissingEnclaveIdentityBody = errors.New("missing enclaveIdentity body in the collaterals obtained")
	// ErrTcbInfoNil error returned when tcbInfo response structure is missing
	ErrTcbInfoNil = errors.New("tcbInfo is empty in collaterals")
	// ErrQeIdentityNil error returned when QeIdentity response structure is missing
	ErrQeIdentityNil = errors.New("QeIdentity is empty in collaterals")
	// ErrMissingPCKCrlSigningCert error returned when signing certificate is missing in issuer chain of PCK CRL
	ErrMissingPCKCrlSigningCert = errors.New("missing signing certificate in the issuer chain of PCK CRL")
	// ErrMissingPCKCrlRootCert error returned when root certificate is missing in issuer chain of PCK CRL
	ErrMissingPCKCrlRootCert = errors.New("missing root certificate in the issuer chain of PCK CRL")
	// ErrMissingTcbInfoSigningCert error returned when signing certificate is missing in issuer chain of tcbInfo
	ErrMissingTcbInfoSigningCert = errors.New("missing signing certificate in the issuer chain of tcbInfo")
	// ErrMissingTcbInfoRootCert error returned when root certificate is missing in issuer chain of tcbInfo
	ErrMissingTcbInfoRootCert = errors.New("missing root certificate in the issuer chain of tcbInfo")
	// ErrMissingQeIdentitySigningCert error returned when signing certificate is missing in issuer chain of QeIdentity
	ErrMissingQeIdentitySigningCert = errors.New("missing signing certificate in the issuer chain of QeIdentity")
	// ErrMissingQeIdentityRootCert error returned when root certificate is missing in issuer chain of QeIdentity
	ErrMissingQeIdentityRootCert = errors.New("missing root certificate in the issuer chain of QeIdentity")
	// ErrRootCaCrlExpired error returned when Root CA CRL is expired
	ErrRootCaCrlExpired = errors.New("root CA CRL has expired")
	// ErrPCKCrlExpired error returned when PCK CRL is expired
	ErrPCKCrlExpired = errors.New("PCK CRL has expired")
	// ErrTcbInfoExpired error returned when tcbInfo response is expired
	ErrTcbInfoExpired = errors.New("tcbInfo has expired")
	// ErrQeIdentityExpired error returned when QeIdentity response is expired
	ErrQeIdentityExpired = errors.New("QeIdentity has expired")
	// ErrPCKCrlSigningCertExpired error returned when PCK CRL signing certificate is expired
	ErrPCKCrlSigningCertExpired = errors.New("PCK CRL signing certificate has expired")
	// ErrPCKCrlRootCertExpired error returned when PCK CRL root certificate is expired
	ErrPCKCrlRootCertExpired = errors.New("PCK CRL root certificate has expired")
	// ErrTcbInfoSigningCertExpired error returned when tcbInfo signing certificate is expired
	ErrTcbInfoSigningCertExpired = errors.New("tcbInfo signing certificate has expired")
	// ErrTcbInfoRootCertExpired error returned when tcbInfo root certificate is expired
	ErrTcbInfoRootCertExpired = errors.New("tcbInfo root certificate has expired")
	// ErrQeIdentitySigningCertExpired error returned when QeIdentity signing certificate is expired
	ErrQeIdentitySigningCertExpired = errors.New("QeIdentity signing certificate has expired")
	// ErrQeIdentityRootCertExpired error returned when QeIdentity root certificate is expired
	ErrQeIdentityRootCertExpired = errors.New("QeIdentity root certificate has expired")
	// ErrCrlEmpty error returned when Certificate Revocation list is empty
	ErrCrlEmpty = errors.New("CRL is empty")
	// ErrTrustedCertEmpty error returned when no trusted certificate is provided for verification
	ErrTrustedCertEmpty = errors.New("trusted certificate is empty")
	// ErrRevocationCheckFailed error returned when CheckRevocations parameter is set to true and GetCollateral is set to false
	ErrRevocationCheckFailed = errors.New("unable to check for certificate revocation as GetCollateral parameter in the options is set to false")
	// ErrTcbInfoTcbLevelsMissing error returned when TCBLevels array in TCB info is of length 0
	ErrTcbInfoTcbLevelsMissing = errors.New("tcbInfo contains empty TcbLevels")
	// ErrQeIdentityTcbLevelsMissing error returned when TCBLevels array in QE Identity is of length 0
	ErrQeIdentityTcbLevelsMissing = errors.New("QeIdentity contains empty TcbLevels")
	// ErrPckLeafCertExpired error returned when PCK Leaf certificate has expired
	ErrPckLeafCertExpired = errors.New("PCK leaf certificate in PCK certificate chain has expired")
	// ErrRootCaCertExpired error returned when Root CA certificate has expired
	ErrRootCaCertExpired = errors.New("root CA certificate in PCK certificate chain has expired")
	// ErrIntermediateCaCertExpired error returned when Intermediate CA certificate has expired
	ErrIntermediateCaCertExpired = errors.New("intermediate CA certificate in PCK certificate chain has expired")
	// ErrTcbStatus error returned when TCB status is out of date
	ErrTcbStatus = errors.New("unable to find latest status of TCB, it is now OutOfDate")
	// ErrCertNil error returned when certificate is not provided
	ErrCertNil = errors.New("certificate is nil")
	// ErrParentCertNil error returned when parent certificate is not provided
	ErrParentCertNil = errors.New("parent certificate is nil")
)

const (
	pubKeySize        = 64
	pubKeyHeaderByte  = 0x04
	tcbInfoVersion    = 3.0
	qeIdentityVersion = 2.0

	rootCertPhrase                 = "Intel SGX Root CA"
	intermediateCertPhrase         = "Intel SGX PCK Platform CA"
	pckCertPhrase                  = "Intel SGX PCK Certificate"
	processorIssuer                = "Intel SGX PCK Processor CA"
	processorIssuerID              = "processor"
	platformIssuer                 = "Intel SGX PCK Platform CA"
	platformIssuerID               = "platform"
	sgxPckCrlIssuerChainPhrase     = "Sgx-Pck-Crl-Issuer-Chain"
	sgxQeIdentityIssuerChainPhrase = "Sgx-Enclave-Identity-Issuer-Chain"
	tcbInfoIssuerChainPhrase       = "Tcb-Info-Issuer-Chain"
	tcbInfoPhrase                  = "tcbInfo"
	enclaveIdentityPhrase          = "enclaveIdentity"
	certificateType                = "CERTIFICATE"
	tcbInfoID                      = "TDX"
	qeIdentityID                   = "TD_QE"
	tcbSigningPhrase               = "Intel SGX TCB Signing"
)

// Options represents verification options for a TDX attestation quote.
type Options struct {
	// CheckRevocations set to true if the verifier should retrieve the CRL from the network and check
	// if the PCK certificate chain have been revoked.
	CheckRevocations bool
	// GetCollateral set to true if the verifier should retrieve the collaterals from the network using PCS.
	GetCollateral bool
	// Getter takes a URL and returns the body of its contents. By default uses http.Get and returns the header and body
	Getter trust.HTTPSGetter
	// Now is the time at which to verify the validity of certificates and collaterals. If unset, uses time.Now().
	Now time.Time
	// TrustedRoots specifies the root CertPool to trust when verifying PCK certificate chain.
	// If nil, embedded certificate will be used
	TrustedRoots *x509.CertPool

	chain             *PCKCertificateChain
	collateral        *Collateral
	pckCertExtensions *pcs.PckExtensions
}

// DefaultOptions returns a useful default verification option setting
func DefaultOptions() *Options {
	return &Options{
		Getter: trust.DefaultHTTPSGetter(),
		Now:    time.Now(),
	}
}

type tdQuoteBodyOptions struct {
	tcbInfo           pcs.TcbInfo
	pckCertExtensions *pcs.PckExtensions
}

type qeReportOptions struct {
	qeIdentity *pcs.EnclaveIdentity
}

// PCKCertificateChain contains certificate chains
type PCKCertificateChain struct {
	PCKCertificate          *x509.Certificate // PCK Leaf certificate
	RootCertificate         *x509.Certificate // Root CA certificate
	IntermediateCertificate *x509.Certificate // Intermediate CA certificate
}

// Collateral contains information received from Intel PCS API service
type Collateral struct {
	PckCrlIssuerIntermediateCertificate     *x509.Certificate
	PckCrlIssuerRootCertificate             *x509.Certificate
	PckCrl                                  *x509.RevocationList
	TcbInfoIssuerIntermediateCertificate    *x509.Certificate
	TcbInfoIssuerRootCertificate            *x509.Certificate
	TdxTcbInfo                              pcs.TdxTcbInfo
	TcbInfoBody                             []byte
	QeIdentityIssuerIntermediateCertificate *x509.Certificate
	QeIdentityIssuerRootCertificate         *x509.Certificate
	QeIdentity                              pcs.QeIdentity
	EnclaveIdentityBody                     []byte
	RootCaCrl                               *x509.RevocationList
}

// CRLUnavailableErr represents a problem with fetching the CRL from the network.
// This type is special to allow for easy "fail open" semantics for CRL unavailability. See
// Adam Langley's write-up on CRLs and network unreliability
// https://www.imperialviolet.org/2014/04/19/revchecking.html
type CRLUnavailableErr struct {
	error
}

func applyMask(a, b []byte) []byte {
	data := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		data[i] = a[i] & b[i]
	}

	return data
}

func extractCaFromPckCert(pckCert *x509.Certificate) (string, error) {
	pckIssuer := pckCert.Issuer.CommonName
	if pckIssuer == platformIssuer {
		return platformIssuerID, nil
	}
	if pckIssuer == processorIssuer {
		return processorIssuerID, nil
	}
	return "", ErrPckCertCANil
}

// bytesToEcdsaPubKey converts byte array to ecdsa public key format
func bytesToEcdsaPubKey(b []byte) (*ecdsa.PublicKey, error) {
	size := len(b)
	if size != pubKeySize {
		return nil, ErrPublicKeySize
	}
	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(b[0 : pubKeySize/2]),
		Y:     new(big.Int).SetBytes(b[pubKeySize/2 : pubKeySize]),
	}

	if !publicKey.Curve.IsOnCurve(publicKey.X, publicKey.Y) {
		return nil, fmt.Errorf("public key is not on curve %q", publicKey.Curve.Params().Name)
	}
	return publicKey, nil
}

func headerToIssuerChain(header map[string][]string, phrase string) (*x509.Certificate, *x509.Certificate, error) {
	issuerChain, ok := header[phrase]
	if !ok {
		return nil, nil, fmt.Errorf("%q is empty", phrase)
	}
	if len(issuerChain) != 1 {
		return nil, nil, fmt.Errorf("issuer chain is expected to be of size 1, found %d", len(issuerChain))
	}
	if issuerChain[0] == "" {
		return nil, nil, fmt.Errorf("issuer chain certificates missing in %q", phrase)
	}

	certChain, err := url.QueryUnescape(issuerChain[0])
	if err != nil {
		return nil, nil, fmt.Errorf("unable to decode issuer chain in %q: %v", phrase, err)
	}

	intermediate, rem := pem.Decode([]byte(certChain))
	if intermediate == nil || len(rem) == 0 {
		return nil, nil, fmt.Errorf("could not parse PEM formatted signing certificate in %q", phrase)
	}
	if intermediate.Type != certificateType {
		return nil, nil, fmt.Errorf(`the %q PEM block type is %q. Expect %q`, phrase, intermediate.Type, certificateType)
	}
	intermediateCert, err := x509.ParseCertificate(intermediate.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("could not interpret DER bytes of signing certificate in %q: %v", phrase, err)
	}

	root, rem := pem.Decode(rem)
	if root == nil || len(rem) != 0 {
		return nil, nil, fmt.Errorf("could not parse PEM formatted root certificate in %q", phrase)
	}
	if root.Type != certificateType {
		return nil, nil, fmt.Errorf(`the %q PEM block type is %q. Expect %q`, phrase, root.Type, certificateType)
	}
	rootCert, err := x509.ParseCertificate(root.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("could not interpret DER bytes of root certificate in %q: %v", phrase, err)
	}
	return intermediateCert, rootCert, nil
}

func bodyToCrl(body []byte) (*x509.RevocationList, error) {
	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DER bytes of CRL: %v", err)
	}
	return crl, nil
}

func bodyToRawMessage(name string, body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("%q is empty", name)
	}
	var rawbody map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawbody); err != nil {
		return nil, fmt.Errorf("could not convert %q body to raw message: %v", name, err)
	}
	val, ok := rawbody[name]
	if !ok {
		return nil, fmt.Errorf("%q field is missing in the response received", name)
	}

	return val, nil
}

func getPckCrl(ca string, getter trust.HTTPSGetter, collateral *Collateral) error {
	pcsCrlURL := pcs.PckCrlURL(ca)
	header, body, err := getter.Get(pcsCrlURL)
	if err != nil {
		return CRLUnavailableErr{multierr.Append(err, errors.New("could not fetch PCK CRL"))}
	}
	pckCrlIntermediateCert, pckCrlRootCert, err := headerToIssuerChain(header, sgxPckCrlIssuerChainPhrase)
	if err != nil {
		return err
	}
	collateral.PckCrlIssuerIntermediateCertificate = pckCrlIntermediateCert
	collateral.PckCrlIssuerRootCertificate = pckCrlRootCert

	crl, err := bodyToCrl(body)
	if err != nil {
		return CRLUnavailableErr{multierr.Append(err, errors.New("could not fetch PCK CRL"))}
	}
	collateral.PckCrl = crl
	return nil
}

func getTcbInfo(fmspc string, getter trust.HTTPSGetter, collateral *Collateral) error {
	tcbInfoURL := pcs.TcbInfoURL(fmspc)
	header, body, err := getter.Get(tcbInfoURL)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: fmt.Sprintf("could not receive tcbInfo response: %v", err),
		}
	}

	tcbInfoIntermediateCert, tcbInfoRootCert, err := headerToIssuerChain(header, tcbInfoIssuerChainPhrase)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: err.Error(),
		}
	}

	collateral.TcbInfoIssuerIntermediateCertificate = tcbInfoIntermediateCert
	collateral.TcbInfoIssuerRootCertificate = tcbInfoRootCert

	if err := json.Unmarshal(body, &collateral.TdxTcbInfo); err != nil {
		return &trust.AttestationRecreationErr{
			Msg: fmt.Sprintf("unable to unmarshal tcbInfo response: %v", err),
		}
	}

	tcbInfoRawBody, err := bodyToRawMessage(tcbInfoPhrase, body)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: err.Error(),
		}
	}
	collateral.TcbInfoBody = tcbInfoRawBody
	return nil
}

func getQeIdentity(getter trust.HTTPSGetter, collateral *Collateral) error {
	qeIdentityURL := pcs.QeIdentityURL()
	header, body, err := getter.Get(qeIdentityURL)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: fmt.Sprintf("could not receive QeIdentity response: %v", err),
		}
	}

	qeIdentityIntermediateCert, qeIdentityRootCert, err := headerToIssuerChain(header, sgxQeIdentityIssuerChainPhrase)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: err.Error(),
		}
	}
	collateral.QeIdentityIssuerIntermediateCertificate = qeIdentityIntermediateCert
	collateral.QeIdentityIssuerRootCertificate = qeIdentityRootCert

	if err := json.Unmarshal(body, &collateral.QeIdentity); err != nil {
		return &trust.AttestationRecreationErr{
			Msg: fmt.Sprintf("unable to unmarshal QeIdentity response: %v", err),
		}
	}

	qeIdentityRawBody, err := bodyToRawMessage(enclaveIdentityPhrase, body)
	if err != nil {
		return &trust.AttestationRecreationErr{
			Msg: err.Error(),
		}
	}
	collateral.EnclaveIdentityBody = qeIdentityRawBody
	return nil
}

func getRootCrl(getter trust.HTTPSGetter, collateral *Collateral) error {
	rootCrlURL := collateral.QeIdentityIssuerRootCertificate.CRLDistributionPoints // QE identity issuer chain's root certificate contains url for Root CA CRL
	if len(rootCrlURL) == 0 {
		return ErrEmptyRootCRLUrl
	}
	var errs error
	for i := range rootCrlURL {
		_, body, err := getter.Get(rootCrlURL[i])
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		crl, err := bodyToCrl(body)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		collateral.RootCaCrl = crl
		return nil
	}

	return CRLUnavailableErr{multierr.Append(errs, errors.New("could not fetch root CRL"))}
}

func obtainCollateral(fmspc string, ca string, options *Options) (*Collateral, error) {
	getter := options.Getter
	if getter == nil {
		getter = trust.DefaultHTTPSGetter()
	}
	collateral := &Collateral{}

	if err := getTcbInfo(fmspc, getter, collateral); err != nil {
		return nil, fmt.Errorf("unable to receive tcbInfo: %v", err)
	}
	if err := getQeIdentity(getter, collateral); err != nil {
		return nil, fmt.Errorf("unable to receive QeIdentity: %v", err)
	}
	if options.CheckRevocations {
		if err := getPckCrl(ca, getter, collateral); err != nil {
			return nil, fmt.Errorf("unable to receive PCK CRL: %v", err)
		}
		if err := getRootCrl(getter, collateral); err != nil {
			return nil, fmt.Errorf("unable to receive Root CA CRL: %v", err)
		}
	}
	return collateral, nil
}

func checkCollateralExpiration(collateral *Collateral, options *Options) error {
	currentTime := options.Now

	tcbInfo := collateral.TdxTcbInfo.TcbInfo
	qeIdentity := collateral.QeIdentity.EnclaveIdentity

	if currentTime.After(tcbInfo.NextUpdate) {
		return ErrTcbInfoExpired
	}
	if currentTime.After(qeIdentity.NextUpdate) {
		return ErrQeIdentityExpired
	}
	if currentTime.After(collateral.TcbInfoIssuerIntermediateCertificate.NotAfter) {
		return ErrTcbInfoSigningCertExpired
	}
	if currentTime.After(collateral.TcbInfoIssuerRootCertificate.NotAfter) {
		return ErrTcbInfoRootCertExpired
	}
	if currentTime.After(collateral.QeIdentityIssuerRootCertificate.NotAfter) {
		return ErrQeIdentityRootCertExpired
	}
	if currentTime.After(collateral.QeIdentityIssuerIntermediateCertificate.NotAfter) {
		return ErrQeIdentitySigningCertExpired
	}
	if options.CheckRevocations {
		if currentTime.After(collateral.RootCaCrl.NextUpdate) {
			return ErrRootCaCrlExpired
		}
		if currentTime.After(collateral.PckCrl.NextUpdate) {
			return ErrPCKCrlExpired
		}
		if currentTime.After(collateral.PckCrlIssuerIntermediateCertificate.NotAfter) {
			return ErrPCKCrlSigningCertExpired
		}
		if currentTime.After(collateral.PckCrlIssuerRootCertificate.NotAfter) {
			return ErrPCKCrlRootCertExpired
		}
	}
	return nil
}

func checkCertificateExpiration(chain *PCKCertificateChain, options *Options) error {
	currentTime := options.Now

	if currentTime.After(chain.RootCertificate.NotAfter) {
		return ErrRootCaCertExpired
	}
	if currentTime.After(chain.IntermediateCertificate.NotAfter) {
		return ErrIntermediateCaCertExpired
	}
	if currentTime.After(chain.PCKCertificate.NotAfter) {
		return ErrPckLeafCertExpired
	}
	return nil
}

func verifyCollateral(options *Options) error {
	collateral := options.collateral
	if collateral == nil {
		return ErrCollateralNil
	}
	if collateral.TcbInfoBody == nil {
		return ErrMissingTcbInfoBody
	}
	if collateral.EnclaveIdentityBody == nil {
		return ErrMissingEnclaveIdentityBody
	}
	if reflect.DeepEqual(collateral.TdxTcbInfo, pcs.TdxTcbInfo{}) {
		return ErrTcbInfoNil
	}
	if reflect.DeepEqual(collateral.QeIdentity, pcs.QeIdentity{}) {
		return ErrQeIdentityNil
	}

	if collateral.TcbInfoIssuerIntermediateCertificate == nil {
		return ErrMissingTcbInfoSigningCert
	}
	if collateral.TcbInfoIssuerRootCertificate == nil {
		return ErrMissingTcbInfoRootCert
	}
	if collateral.QeIdentityIssuerIntermediateCertificate == nil {
		return ErrMissingQeIdentitySigningCert
	}
	if collateral.QeIdentityIssuerRootCertificate == nil {
		return ErrMissingQeIdentityRootCert
	}
	if options.CheckRevocations {
		if collateral.PckCrl == nil {
			return ErrMissingPckCrl
		}
		if collateral.RootCaCrl == nil {
			return ErrMissingRootCaCrl
		}
		if collateral.PckCrlIssuerIntermediateCertificate == nil {
			return ErrMissingPCKCrlSigningCert
		}
		if collateral.PckCrlIssuerRootCertificate == nil {
			return ErrMissingPCKCrlRootCert
		}
	}

	return checkCollateralExpiration(collateral, options)
}

func extractChainFromQuote(quote *pb.QuoteV4) (*PCKCertificateChain, error) {
	certChainBytes := quote.GetSignedData().GetCertificationData().GetQeReportCertificationData().GetPckCertificateChainData().GetPckCertChain()
	if certChainBytes == nil {
		return nil, ErrPCKCertChainNil
	}

	pck, rem := pem.Decode(certChainBytes)
	if pck == nil || len(rem) == 0 || pck.Type != certificateType {
		return nil, ErrPCKCertChainInvalid
	}
	pckCert, err := x509.ParseCertificate(pck.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not interpret PCK leaf certificate DER bytes: %v", err)
	}

	intermediate, rem := pem.Decode(rem)
	if intermediate == nil || len(rem) == 0 || intermediate.Type != certificateType {
		return nil, ErrPCKCertChainInvalid
	}
	intermediateCert, err := x509.ParseCertificate(intermediate.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not interpret Intermediate CA certificate DER bytes: %v", err)
	}

	root, rem := pem.Decode(rem)
	if root == nil || len(rem) != 0 || root.Type != certificateType {
		return nil, ErrPCKCertChainInvalid
	}

	rootCert, err := x509.ParseCertificate(root.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not interpret Root CA certificate DER bytes: %v", err)
	}
	return &PCKCertificateChain{PCKCertificate: pckCert,
		RootCertificate:         rootCert,
		IntermediateCertificate: intermediateCert}, nil
}

func validateX509Cert(cert *x509.Certificate, version int, signatureAlgorithm x509.SignatureAlgorithm, publicKeyAlgorithm x509.PublicKeyAlgorithm, curve string) error {
	if cert.Version != version {
		return fmt.Errorf("certificate's version found %v. Expected %d", cert.Version, version)
	}

	if cert.SignatureAlgorithm != signatureAlgorithm {
		return fmt.Errorf("certificate's signature algorithm found %v. Expected %v", cert.SignatureAlgorithm, signatureAlgorithm)
	}

	if cert.PublicKeyAlgorithm != publicKeyAlgorithm {
		return fmt.Errorf("certificate's public Key algorithm found %v. Expected %v", cert.PublicKeyAlgorithm, publicKeyAlgorithm)
	}

	// Locally bind the public key any type to allow for occurrence typing in the switch statement.
	switch pub := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if pub.Curve.Params().Name != curve {
			return fmt.Errorf("certificate's public key curve is %q. Expected %q", pub.Curve.Params().Name, curve)
		}
	default:
		return ErrCertPubKeyType
	}
	return nil
}

func validateCertificate(cert *x509.Certificate, parent *x509.Certificate, phrase string) error {
	if cert == nil {
		return ErrCertNil
	}
	if parent == nil {
		return ErrParentCertNil
	}

	// Signature algorithm: ECDSA
	// Signature hash algorithm sha256
	// Subject Public Key Info ECDSA on curve P-256
	if err := validateX509Cert(cert, 3, x509.ECDSAWithSHA256, x509.ECDSA, "P-256"); err != nil {
		return err
	}

	if cert.Subject.CommonName != phrase {
		return fmt.Errorf("%q is not expected in certificate's subject name. Expected %q", cert.Subject.CommonName, phrase)
	}

	if cert.Issuer.String() != parent.Subject.String() {
		return fmt.Errorf("certificate's issuer name (%q), does not match with parent certificate's subject name (%q)", cert.Issuer.String(), parent.Subject.String())
	}

	if err := cert.CheckSignatureFrom(parent); err != nil {
		return fmt.Errorf("certificate signature verification using parent certificate failed: %v", err)
	}
	return nil
}

func validateCRL(crl *x509.RevocationList, trustedCertificate *x509.Certificate) error {
	if crl == nil {
		return ErrCrlEmpty
	}
	if trustedCertificate == nil {
		return ErrTrustedCertEmpty
	}

	if crl.Issuer.String() != trustedCertificate.Subject.String() {
		return fmt.Errorf("CRL issuer's name %q does not match with expected name %q", crl.Issuer.String(), trustedCertificate.Subject.String())
	}
	if err := crl.CheckSignatureFrom(trustedCertificate); err != nil {
		return fmt.Errorf("CRL signature verification failed using trusted certificate: %v", err)
	}

	return nil
}

func getHeaderAndTdQuoteBodyInAbiBytes(quote *pb.QuoteV4) ([]byte, error) {
	header, err := abi.HeaderToAbiBytes(quote.GetHeader())
	if err != nil {
		return nil, fmt.Errorf("could not convert header to ABI bytes: %v", err)
	}
	tdQuoteBody, err := abi.TdQuoteBodyToAbiBytes(quote.GetTdQuoteBody())
	if err != nil {
		return nil, fmt.Errorf("could not convert TD Quote Body to ABI bytes: %v", err)
	}
	return append(header, tdQuoteBody...), nil
}

func verifyPCKCertificationChain(options *Options) error {
	chain := options.chain
	collateral := options.collateral
	rootCert := chain.RootCertificate
	if rootCert == nil {
		return ErrRootCertNil
	}

	intermediateCert := chain.IntermediateCertificate
	if intermediateCert == nil {
		return ErrIntermediateCertNil
	}

	pckCert := chain.PCKCertificate
	if pckCert == nil {
		return ErrPCKCertNil
	}

	// root certificate should be a self-signed certificate
	if err := validateCertificate(rootCert, rootCert, rootCertPhrase); err != nil {
		return fmt.Errorf("unable to validate root cert: %v", err)
	}
	if err := validateCertificate(intermediateCert, rootCert, intermediateCertPhrase); err != nil {
		return fmt.Errorf("unable to validate Intermediate CA certificate: %v", err)
	}
	if err := validateCertificate(pckCert, intermediateCert, pckCertPhrase); err != nil {
		return fmt.Errorf("unable to validate PCK leaf certificate: %v", err)
	}

	if _, err := pckCert.Verify(x509Options(options.TrustedRoots, intermediateCert, options.Now)); err != nil {
		return fmt.Errorf("error verifying PCK Certificate: %v (%v)", err, rootCert.IsCA)
	}

	if options.CheckRevocations {
		if options.GetCollateral {
			if err := validateCRL(collateral.RootCaCrl, rootCert); err != nil {
				return fmt.Errorf("root CA CRL verification failed using root certificate in PCK Certificate chain: %v", err)
			}
			if err := validateCRL(collateral.PckCrl, intermediateCert); err != nil {
				return fmt.Errorf("PCK CRL verification failed using intermediate certificate in PCK Certificate chain: %v", err)
			}
			if collateral.PckCrl.Issuer.String() != pckCert.Issuer.String() {
				return fmt.Errorf("issuer's name(%q) in PCK CRL does not match with PCK Leaf Certificate's issuer name(%q)", collateral.PckCrl.Issuer.String(), pckCert.Issuer.String())
			}
			for _, bad := range collateral.RootCaCrl.RevokedCertificates {
				if intermediateCert.SerialNumber.Cmp(bad.SerialNumber) == 0 {
					return fmt.Errorf("intermediate certificate in PCK certificate chain was revoked at %v", bad.RevocationTime)
				}
			}
			for _, bad := range collateral.PckCrl.RevokedCertificates {
				if pckCert.SerialNumber.Cmp(bad.SerialNumber) == 0 {
					return fmt.Errorf("PCK Leaf certificate in PCK certificate chain was revoked at %v", bad.RevocationTime)
				}
			}
		} else {
			return ErrRevocationCheckFailed
		}
	}

	return checkCertificateExpiration(chain, options)
}

func x509Options(trustedRoots *x509.CertPool, intermediateCert *x509.Certificate, now time.Time) x509.VerifyOptions {
	if trustedRoots == nil {
		logger.Warning("Using embedded Intel certificate for TDX attestation root of trust")
		trustedRoots = x509.NewCertPool()
		trustedRoots.AddCert(trustedRootCertificate)
	}

	intermediates := x509.NewCertPool()
	if intermediateCert != nil {
		intermediates.AddCert(intermediateCert)
	}

	return x509.VerifyOptions{Roots: trustedRoots, Intermediates: intermediates, CurrentTime: now}
}

func verifyHash256(quote *pb.QuoteV4) error {
	qeReportCertificationData := quote.GetSignedData().GetCertificationData().GetQeReportCertificationData()
	qeReportData := qeReportCertificationData.GetQeReport().GetReportData()
	qeAuthData := qeReportCertificationData.GetQeAuthData().GetData()
	attestKey := quote.GetSignedData().GetEcdsaAttestationKey()

	concatOfAttestKeyandQeAuthData := append(attestKey, qeAuthData...)
	var hashedMessage []byte
	hashedConcatOfAttestKeyandQeAuthData := sha256.Sum256(concatOfAttestKeyandQeAuthData)
	hashedMessage = hashedConcatOfAttestKeyandQeAuthData[:]
	hashedMessage = append(hashedMessage, make([]byte, len(qeReportData)-len(hashedMessage))...)
	if !bytes.Equal(hashedMessage, qeReportData) {
		return ErrSHA56VerificationFail
	}
	return nil
}

func isSvnHigherOrEqual(svn []byte, components []pcs.TcbComponent) bool {
	for i := range svn {
		if svn[i] < components[i].Svn {
			return false
		}
	}
	return true
}

func getMatchingTcbLevel(tcbLevels []pcs.TcbLevel, tdReport *pb.TDQuoteBody, pckCertPceSvn uint16, pckCertCPUSvnComponents []byte) (pcs.TcbLevel, error) {
	for _, tcbLevel := range tcbLevels {
		if isSvnHigherOrEqual(pckCertCPUSvnComponents, tcbLevel.Tcb.SgxTcbcomponents) &&
			pckCertPceSvn >= tcbLevel.Tcb.Pcesvn &&
			isSvnHigherOrEqual(tdReport.GetTeeTcbSvn(), tcbLevel.Tcb.TdxTcbcomponents) {
			return tcbLevel, nil
		}
	}
	return pcs.TcbLevel{}, fmt.Errorf("no matching TCB level found")
}

func checkQeTcbStatus(tcbLevels []pcs.TcbLevel, isvsvn uint32) error {
	for _, tcbLevel := range tcbLevels {
		if tcbLevel.Tcb.Isvsvn <= isvsvn {
			if tcbLevel.TcbStatus != pcs.TcbComponentStatusUpToDate {
				return fmt.Errorf("TCB Status is not %q, found %q", pcs.TcbComponentStatusUpToDate, tcbLevel.TcbStatus)
			}
			return nil
		}
	}
	return ErrTcbStatus
}

func checkTcbInfoTcbStatus(tcbLevels []pcs.TcbLevel, tdQuoteBody *pb.TDQuoteBody, pckCertExtensions *pcs.PckExtensions) error {
	matchingTcbLevel, err := getMatchingTcbLevel(tcbLevels, tdQuoteBody, pckCertExtensions.TCB.PCESvn, pckCertExtensions.TCB.CPUSvnComponents)
	if err != nil {
		return err
	}

	if matchingTcbLevel.Tcb.TdxTcbcomponents[1].Svn != tdQuoteBody.GetTeeTcbSvn()[1] {
		return fmt.Errorf("SVN at index 1(%v) in Tcb.TdxTcbcomponents is not equal to TD Quote Body's index 1(%v) TEE TCB svn value", matchingTcbLevel.Tcb.TdxTcbcomponents[1].Svn, tdQuoteBody.GetTeeTcbSvn()[1])
	}

	if matchingTcbLevel.TcbStatus != pcs.TcbComponentStatusUpToDate {
		return fmt.Errorf("TCB Status is not %q, found %q", pcs.TcbComponentStatusUpToDate, matchingTcbLevel.TcbStatus)
	}
	return nil
}

func verifyTdQuoteBody(tdQuoteBody *pb.TDQuoteBody, tdQuoteBodyOptions *tdQuoteBodyOptions) error {
	if tdQuoteBodyOptions.pckCertExtensions.FMSPC != tdQuoteBodyOptions.tcbInfo.Fmspc {
		return fmt.Errorf("FMSPC from PCK Certificate(%q) is not equal to FMSPC value from PCS's reported TDX TCB info(%q)", tdQuoteBodyOptions.pckCertExtensions.FMSPC, tdQuoteBodyOptions.tcbInfo.Fmspc)
	}

	if tdQuoteBodyOptions.pckCertExtensions.PCEID != tdQuoteBodyOptions.tcbInfo.PceID {
		return fmt.Errorf("PCEID from PCK Certificate(%q) is not equal to PCEID from PCS's reported TDX TCB info(%q)", tdQuoteBodyOptions.pckCertExtensions.PCEID, tdQuoteBodyOptions.tcbInfo.PceID)
	}

	if !bytes.Equal(tdQuoteBodyOptions.tcbInfo.TdxModule.Mrsigner.Bytes, tdQuoteBody.GetMrSignerSeam()) {
		return fmt.Errorf("MRSIGNERSEAM value from TD Quote Body(%q) is not equal to TdxModule.Mrsigner field in PCS's reported TDX TCB info(%q)", hex.EncodeToString(tdQuoteBody.GetMrSignerSeam()), hex.EncodeToString(tdQuoteBodyOptions.tcbInfo.TdxModule.Mrsigner.Bytes))
	}

	if len(tdQuoteBodyOptions.tcbInfo.TdxModule.AttributesMask.Bytes) != len(tdQuoteBody.GetSeamAttributes()) {
		return fmt.Errorf("size of SeamAttributes from TD Quote Body(%d) is not equal to size of TdxModule.AttributesMask in PCS's reported TDX TCB info(%d)", len(tdQuoteBodyOptions.tcbInfo.TdxModule.AttributesMask.Bytes), len(tdQuoteBody.GetSeamAttributes()))
	}
	attributesMask := applyMask(tdQuoteBodyOptions.tcbInfo.TdxModule.AttributesMask.Bytes, tdQuoteBody.GetSeamAttributes())

	if !bytes.Equal(tdQuoteBodyOptions.tcbInfo.TdxModule.Attributes.Bytes, attributesMask) {
		return fmt.Errorf("AttributesMask value(%q) is not equal to TdxModule.Attributes field in PCS's reported TDX TCB info(%q)", hex.EncodeToString(attributesMask), hex.EncodeToString(tdQuoteBodyOptions.tcbInfo.TdxModule.Attributes.Bytes))
	}

	if err := checkTcbInfoTcbStatus(tdQuoteBodyOptions.tcbInfo.TcbLevels, tdQuoteBody, tdQuoteBodyOptions.pckCertExtensions); err != nil {
		return fmt.Errorf("PCS's reported TDX TCB info failed TCB status check: %v", err)
	}
	return nil
}

func verifyQeReport(qeReport *pb.EnclaveReport, qeReportOptions *qeReportOptions) error {

	if len(qeReportOptions.qeIdentity.MiscselectMask.Bytes) != 4 { // To create a uint32 variable, byte array should have size 4
		return fmt.Errorf("MISCSELECTMask field size(%d) in PCS's reported QE Identity is not equal to expected size(4)", len(qeReportOptions.qeIdentity.MiscselectMask.Bytes))
	}
	if len(qeReportOptions.qeIdentity.Miscselect.Bytes) != 4 { // To create a uint32 variable, byte array should have size 4
		return fmt.Errorf("MISCSELECT field size(%d) in PCS's reported QE Identity is not equal to expected size(4)", len(qeReportOptions.qeIdentity.Miscselect.Bytes))
	}
	miscSelectMask := binary.LittleEndian.Uint32(qeReportOptions.qeIdentity.MiscselectMask.Bytes)
	miscSelect := binary.LittleEndian.Uint32(qeReportOptions.qeIdentity.Miscselect.Bytes)
	miscSelectMask = qeReport.GetMiscSelect() & miscSelectMask
	if miscSelectMask != miscSelect {
		return fmt.Errorf("MISCSELECT value(%v) from PCS's reported QE Identity is not equal to MISCSELECTMask value(%v)", miscSelect, miscSelectMask)
	}

	if len(qeReportOptions.qeIdentity.AttributesMask.Bytes) != len(qeReport.GetAttributes()) {
		return fmt.Errorf("size of AttributesMask value(%d) in PCS's reported QE Identity is not equal to size of SeamAttributes value(%d) in QE Report", len(qeReportOptions.qeIdentity.AttributesMask.Bytes), len(qeReport.GetAttributes()))
	}
	qeAttributesMask := applyMask(qeReportOptions.qeIdentity.AttributesMask.Bytes, qeReport.GetAttributes())

	if !bytes.Equal(qeReportOptions.qeIdentity.Attributes.Bytes, qeAttributesMask) {
		return fmt.Errorf("AttributesMask value(%v) is not equal to Attributes value(%v) in PCS's reported QE Identity", qeAttributesMask, qeReportOptions.qeIdentity.Attributes)
	}

	if !bytes.Equal(qeReportOptions.qeIdentity.Mrsigner.Bytes, qeReport.GetMrSigner()) {
		return fmt.Errorf("MRSIGNER value(%q) in QE Report is not equal to MRSIGNER value(%q) in PCS's reported QE Identity", hex.EncodeToString(qeReport.GetMrSigner()), qeReportOptions.qeIdentity.Mrsigner)
	}

	if qeReport.GetIsvProdId() != uint32(qeReportOptions.qeIdentity.IsvProdID) {
		return fmt.Errorf("ISV PRODID value(%v) in QE Report is not equal to ISV PRODID value(%v) in PCS's reported QE Identity", qeReport.GetIsvProdId(), qeReportOptions.qeIdentity.IsvProdID)
	}

	if err := checkQeTcbStatus(qeReportOptions.qeIdentity.TcbLevels, qeReport.GetIsvSvn()); err != nil {
		return fmt.Errorf("PCS's reported QE Identity failed TCB status check: %v", err)
	}
	return nil
}

func verifyQuote(quote *pb.QuoteV4, options *Options) error {
	chain := options.chain
	collateral := options.collateral
	pckCertExtensions := options.pckCertExtensions
	attestkey := quote.GetSignedData().GetEcdsaAttestationKey()
	attestPublicKey, err := bytesToEcdsaPubKey(attestkey)
	if err != nil {
		return fmt.Errorf("attestation key in quote is invalid: %v", err)
	}

	signature := quote.GetSignedData().GetSignature()
	signature, err = abi.SignatureToDER(signature)
	if err != nil {
		return fmt.Errorf("unable to convert QuoteV4's signature to DER format: %v", err)
	}

	message, err := getHeaderAndTdQuoteBodyInAbiBytes(quote)
	if err != nil {
		return fmt.Errorf("could not get message digest for verification: %v", err)
	}

	hashedMessage := sha256.Sum256(message)

	// A hashed version of the attestation key is used as a nonce while generating the signed QE report which in turn is verified with the PCK certificate chain.
	// Thus it is safe to use its raw public key here even though it doesn't have an associated certificate.
	if !ecdsa.VerifyASN1(attestPublicKey, hashedMessage[:], signature) {
		return ErrHashVerificationFail
	}

	qeReportCertificationData := quote.GetSignedData().GetCertificationData().GetQeReportCertificationData()

	if err := tdxProtoQeReportSignature(qeReportCertificationData, chain.PCKCertificate); err != nil {
		return fmt.Errorf("error verifying QE report signature: %v", err)
	}

	if err := verifyHash256(quote); err != nil {
		return fmt.Errorf("error verifying QE report data: %v", err)
	}

	if collateral != nil {
		if err := verifyTdQuoteBody(quote.GetTdQuoteBody(),
			&tdQuoteBodyOptions{
				tcbInfo:           collateral.TdxTcbInfo.TcbInfo,
				pckCertExtensions: pckCertExtensions,
			}); err != nil {
			return err
		}

		if err := verifyQeReport(qeReportCertificationData.GetQeReport(),
			&qeReportOptions{
				qeIdentity: &collateral.QeIdentity.EnclaveIdentity,
			}); err != nil {
			return err
		}
	}

	return nil
}

func tdxProtoQeReportSignature(qeReportCertificationData *pb.QEReportCertificationData, pckCert *x509.Certificate) error {
	rawReport, err := abi.EnclaveReportToAbiBytes(qeReportCertificationData.GetQeReport())
	if err != nil {
		return fmt.Errorf("could not parse QE report: %v", err)
	}
	return tdxQeReportSignature(rawReport, qeReportCertificationData.GetQeReportSignature(), pckCert)
}

func tdxQeReportSignature(qeReport []byte, signature []byte, pckCert *x509.Certificate) error {
	derSignature, err := abi.SignatureToDER(signature)
	if err != nil {
		return fmt.Errorf("unable to convert QE report's signature to DER format: %v", err)
	}

	if err := pckCert.CheckSignature(x509.ECDSAWithSHA256, qeReport, derSignature); err != nil {
		return fmt.Errorf("QE report's signature verification using PCK Leaf Certificate failed: %v", err)
	}

	return nil
}

func verifyResponse(signingPhrase string, rootCertificate *x509.Certificate, signingCertificate *x509.Certificate, rawBody []byte, rawSignature string, crl *x509.RevocationList, options *Options) error {

	if err := validateCertificate(rootCertificate, rootCertificate, rootCertPhrase); err != nil {
		return fmt.Errorf("unable to validate root certificate in the issuer chain: %v", err)
	}
	if err := validateCertificate(signingCertificate, rootCertificate, signingPhrase); err != nil {
		return fmt.Errorf("unable to validate signing certificate in the issuer chain: %v", err)
	}
	if _, err := signingCertificate.Verify(x509Options(options.TrustedRoots, nil, options.Now)); err != nil {
		return fmt.Errorf("unable to verify signing certificate: %v", err)
	}

	signature, err := hex.DecodeString(rawSignature)
	if err != nil {
		return fmt.Errorf("unable to decode signature string in the response: %v", err)
	}

	derSignature, err := abi.SignatureToDER(signature)
	if err != nil {
		return fmt.Errorf("unable to convert signature to DER format: %v", err)
	}

	if err := signingCertificate.CheckSignature(x509.ECDSAWithSHA256, rawBody, derSignature); err != nil {
		return fmt.Errorf("could not verify response body using the signing certificate: %v", err)
	}

	if options.CheckRevocations {
		if options.GetCollateral {
			if err := validateCRL(crl, rootCertificate); err != nil {
				return fmt.Errorf("root CA CRL verification failed using root certificate in the issuer's chain: %v", err)
			}

			for _, bad := range crl.RevokedCertificates {
				if signingCertificate.SerialNumber.Cmp(bad.SerialNumber) == 0 {
					return fmt.Errorf("signing certificate was revoked at %v", bad.RevocationTime)
				}
			}
		} else {
			return ErrRevocationCheckFailed
		}
	}
	return nil
}

func verifyTCBinfo(options *Options) error {
	collateral := options.collateral
	tcbInfo := collateral.TdxTcbInfo.TcbInfo
	signature := collateral.TdxTcbInfo.Signature

	if tcbInfo.ID != tcbInfoID {
		return fmt.Errorf("tcbInfo ID %q does not match with expected ID %q", tcbInfo.ID, tcbInfoID)
	}
	if tcbInfo.Version != tcbInfoVersion {
		return fmt.Errorf("tcbInfo version %v does not match with expected version %v", tcbInfo.Version, tcbInfoVersion)
	}
	if len(tcbInfo.TcbLevels) == 0 {
		return ErrTcbInfoTcbLevelsMissing
	}

	if err := verifyResponse(tcbSigningPhrase, collateral.TcbInfoIssuerRootCertificate, collateral.TcbInfoIssuerIntermediateCertificate,
		collateral.TcbInfoBody, signature, collateral.RootCaCrl, options); err != nil {
		return fmt.Errorf("tcbInfo response verification failed: %v", err)
	}

	return nil
}

func verifyQeIdentity(options *Options) error {
	collateral := options.collateral
	qeIdentity := collateral.QeIdentity.EnclaveIdentity
	signature := collateral.QeIdentity.Signature

	if qeIdentity.ID != qeIdentityID {
		return fmt.Errorf("QeIdentity ID %q does not match with expected ID %q", qeIdentity.ID, qeIdentityID)
	}
	if qeIdentity.Version != qeIdentityVersion {
		return fmt.Errorf("QeIdentity version %v does not match with expected version %v", qeIdentity.Version, qeIdentityVersion)
	}
	if len(qeIdentity.TcbLevels) == 0 {
		return ErrQeIdentityTcbLevelsMissing
	}
	if err := verifyResponse(tcbSigningPhrase, collateral.QeIdentityIssuerRootCertificate, collateral.QeIdentityIssuerIntermediateCertificate,
		collateral.EnclaveIdentityBody, signature, collateral.RootCaCrl, options); err != nil {
		return fmt.Errorf("QeIdentity response verification failed: %v", err)
	}

	return nil
}

func verifyEvidence(quote *pb.QuoteV4, options *Options) error {
	if quote.GetHeader().GetTeeType() != abi.TeeTDX {
		return abi.ErrTeeType
	}

	if err := verifyPCKCertificationChain(options); err != nil {
		return err
	}

	if options.GetCollateral {

		if err := verifyCollateral(options); err != nil {
			return fmt.Errorf("could not verify collaterals obtained: %v", err)
		}
		if err := verifyTCBinfo(options); err != nil {
			return err
		}
		if err := verifyQeIdentity(options); err != nil {
			return err
		}
	}

	return verifyQuote(quote, options)
}

// TdxVerify verifies the protobuf representation of an attestation quote's signature based
// on the quote's SignatureAlgo, provided the certificate chain is valid.
func TdxVerify(quote *pb.QuoteV4, options *Options) error {
	if options == nil {
		return ErrOptionsNil
	}

	if err := abi.CheckQuoteV4(quote); err != nil {
		return fmt.Errorf("QuoteV4 invalid: %v", err)
	}

	chain, err := extractChainFromQuote(quote)
	if err != nil {
		return err
	}
	exts, err := pcs.PckCertificateExtensions(chain.PCKCertificate)
	if err != nil {
		return fmt.Errorf("could not get PCK certificate extensions: %v", err)
	}
	var collateral *Collateral
	if options.GetCollateral {

		ca, err := extractCaFromPckCert(chain.PCKCertificate)
		if err != nil {
			return err
		}
		collateral, err = obtainCollateral(exts.FMSPC, ca, options)
		if err != nil {
			return err
		}
	}
	options.collateral = collateral
	options.pckCertExtensions = exts
	options.chain = chain
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	return verifyEvidence(quote, options)
}

// RawTdxQuoteVerify verifies the raw bytes representation of an attestation quote
func RawTdxQuoteVerify(raw []byte, options *Options) error {
	quote, err := abi.QuoteToProto(raw)
	if err != nil {
		return fmt.Errorf("could not convert raw bytes to QuoteV4: %v", err)
	}

	return TdxVerify(quote, options)
}

// Parse root certificates from the embedded trusted_root certificate file.
func init() {
	root, _ := pem.Decode(defaultRootCertByte)
	trustedRootCertificate, _ = x509.ParseCertificate(root.Bytes)
}
