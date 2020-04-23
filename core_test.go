package coordinator

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math"
	"testing"
	"time"

	"edgeless.systems/mesh/coordinator/quote"
	"edgeless.systems/mesh/coordinator/rpc"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestCore(t *testing.T) {

	const manifest string = `{
		"Packages": {
			"tikv": {
				"MREnclave": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31],
				"MiscSelect": 1111111,
				"Attributes": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]
			},
			"tidb": {
				"MRSigner": [31,30,29,28,27,26,25,24,23,22,21,20,19,18,17,16,15,14,13,12,11,10,9,8,7,6,5,4,3,2,1,0],
				"ISVProdID": 44,
				"ISVSVN": 3,
				"Attributes": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15],
				"MiscSelect": 1111111
			}
		},
		"Infrastructures": {
			"Azure": {
				"QESVN": 2,
				"PCESVN": 3,
				"CPUSVN": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15],
				"RootCA": [3,3,3]
			},
			"Alibaba": {
				"QESVN": 2,
				"PCESVN": 4,
				"CPUSVN": [15,14,13,12,11,10,9,8,7,6,5,4,3,2,1,0],
				"RootCA": [4,4,4]
			}
		},
		"Nodes": {
			"tikv_first": {
				"Package": "tikv",
				"MaxActivations": 1,
				"Parameters": {
					"Files": {
						"/abc/defg.txt": [7,7,7],
						"/ghi/jkl.mno": [8,8,8]
					},
					"Env": {
						"IS_FIRST": "true"
					},
					"Argv": [
						"--first",
						"serve"
					]
				}
			},
			"tikv_other": {
				"Package": "tikv",
				"Parameters": {
					"Argv": [
						"serve"
					]
				}
			},
			"tidb": {
				"Package": "tidb"
			}
		},
		"Clients": {
			"owner": [9,9,9]
		}
	}`

	var clientServer rpc.ClientServer
	var nodeServer rpc.NodeServer

	validator := quote.NewMockValidator()
	issuer := quote.NewMockIssuer()
	MiscSelect := uint32(1111111)
	Attributes := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	createTikvConnection := func(nodeType string) (ctx context.Context, req *rpc.ActivationReq) {
		cert, csr, err := generateNodeCredentials()
		assert.Nil(t, err)
		assert.NotNil(t, cert, csr)

		// create mock quote for certificate
		certQuote, err := issuer.Issue(cert)
		assert.Nil(t, err)
		assert.NotNil(t, certQuote)

		MREnclave := [32]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}

		QESVN := uint16(2)
		PCESVN := uint16(3)
		CPUSVN := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

		validator.AddValidQuote(certQuote, cert,
			// tikv
			quote.PackageProperties{
				MREnclave:  &MREnclave,
				MiscSelect: &MiscSelect,
				Attributes: &Attributes,
			},
			// azure
			quote.InfrastructureProperties{
				QESVN:  &QESVN,
				PCESVN: &PCESVN,
				CPUSVN: &CPUSVN,
				RootCA: []byte{3, 3, 3},
			},
		)

		req = &rpc.ActivationReq{
			CSR:      csr,
			NodeType: nodeType,
			Quote:    certQuote,
		}

		// create a connection context that contains the client cert
		ctx = context.WithValue(context.TODO(), clientTLSCert, cert)
		return
	}

	// actual tests

	t.Run("create server", func(t *testing.T) {
		c, err := NewCore("edgeless", validator, issuer)
		assert.NotNil(t, c)
		assert.Nil(t, err)
		assert.Equal(t, c.state, acceptingManifest)
		assert.Equal(t, c.cert.Subject.Organization, []string{"edgeless"})
		assert.Equal(t, c.cert.Subject.CommonName, coordinatorName)
		clientServer = c
		nodeServer = c
	})

	t.Run("try to activate first tikv prematurely", func(t *testing.T) {
		ctx, req := createTikvConnection("tikv_first")
		resp, err := nodeServer.Activate(ctx, req)
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})

	t.Run("try to set broken manifest", func(t *testing.T) {
		assert.NotNil(t, clientServer.SetManifest(context.TODO(), []byte(manifest)[:len(manifest)-1]))
	})

	t.Run("set manifest", func(t *testing.T) {
		assert.Nil(t, clientServer.SetManifest(context.TODO(), []byte(manifest)))
	})

	t.Run("activate first tikv", func(t *testing.T) {
		ctx, req := createTikvConnection("tikv_first")
		resp, err := nodeServer.Activate(ctx, req)
		assert.Nil(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("try to activate another first tikv", func(t *testing.T) {
		ctx, req := createTikvConnection("tikv_first")
		resp, err := nodeServer.Activate(ctx, req)
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	})

	t.Run("activate 10 other tikv", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			ctx, req := createTikvConnection("tikv_other")
			resp, err := nodeServer.Activate(ctx, req)
			assert.Nil(t, err)
			assert.NotNil(t, resp)
		}
	})

	createTidbConnection := func() (ctx context.Context, req *rpc.ActivationReq) {
		cert, csr, err := generateNodeCredentials()
		assert.Nil(t, err)
		assert.NotNil(t, cert, csr)

		// create mock quote for certificate
		certQuote, err := issuer.Issue(cert)
		assert.Nil(t, err)
		assert.NotNil(t, certQuote)

		MRSigner := [32]byte{31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
		ISVProdID := uint16(44)
		ISVSVN := uint16(3)

		QESVN := uint16(2)
		PCESVN := uint16(4)
		CPUSVN := [16]byte{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}

		validator.AddValidQuote(certQuote, cert,
			// tidb
			quote.PackageProperties{
				MRSigner:   &MRSigner,
				ISVProdID:  &ISVProdID,
				ISVSVN:     &ISVSVN,
				MiscSelect: &MiscSelect,
				Attributes: &Attributes,
			},
			// alibaba
			quote.InfrastructureProperties{
				QESVN:  &QESVN,
				PCESVN: &PCESVN,
				CPUSVN: &CPUSVN,
				RootCA: []byte{4, 4, 4},
			},
		)

		req = &rpc.ActivationReq{
			CSR:      csr,
			NodeType: "tidb",
			Quote:    certQuote,
		}

		// create a connection context that contains the client cert
		ctx = context.WithValue(context.TODO(), clientTLSCert, cert)
		return
	}

	t.Run("activate 10 tidb", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			ctx, req := createTidbConnection()
			resp, err := nodeServer.Activate(ctx, req)
			assert.Nil(t, err)
			assert.NotNil(t, resp)
		}
	})
}

func generateNodeCredentials() (cert []byte, csr []byte, err error) {
	const orgName string = "Acme Inc."
	// create CSR for first TiKV node
	pubk, privk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	// create self-signed certificate for use in initial TLS connection
	notBefore := time.Now()
	notAfter := notBefore.Add(math.MaxInt64)

	serialNumber, err := generateSerial()
	if err != nil {
		return
	}

	templateCert := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{orgName},
			CommonName:   coordinatorName,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: false,
		IsCA:                  true,
	}
	cert, err = x509.CreateCertificate(rand.Reader, &templateCert, &templateCert, pubk, privk)

	// create CSR
	templateCSR := x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{orgName},
		},
		PublicKey: pubk,
	}
	csr, err = x509.CreateCertificateRequest(rand.Reader, &templateCSR, privk)
	return
}
