package k8s

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "tok"

// testCACertPEM generates a self-signed CA certificate PEM for testing.
func testCACertPEM(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating key")

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err, "creating certificate")

	var buf bytes.Buffer
	require.NoError(t, pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}), "encoding PEM")

	return buf.String()
}

func TestNewDynamicClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
	}{
		{
			name: "missing API server",
			cfg: ClientConfig{
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: true,
		},
		{
			name: "missing token func",
			cfg: ClientConfig{
				APIServer: "https://k8s.example.com",
			},
			wantErr: true,
		},
		{
			name: "token func error",
			cfg: ClientConfig{
				APIServer: "https://k8s.example.com",
				TokenFunc: func(context.Context) (string, error) {
					return "", fmt.Errorf("auth failed")
				},
			},
			wantErr: true,
		},
		{
			name: "http API server",
			cfg: ClientConfig{
				APIServer: "http://k8s.example.com",
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: true,
		},
		{
			name: "no scheme API server",
			cfg: ClientConfig{
				APIServer: "k8s.example.com",
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: true,
		},
		{
			name: "empty host",
			cfg: ClientConfig{
				APIServer: "https://",
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: true,
		},
		{
			name: "invalid CA data",
			cfg: ClientConfig{
				APIServer: "https://k8s.example.com",
				CAData:    "not-valid-base64!!!",
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: true,
		},
		{
			name: "valid config without CA",
			cfg: ClientConfig{
				APIServer: "https://k8s.example.com",
				TokenFunc: func(context.Context) (string, error) { return testToken, nil },
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.cfg

			client, err := NewDynamicClient(cfg)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}

func TestNewDynamicClient_ValidConfigWithCA(t *testing.T) {
	t.Parallel()

	caData := base64.StdEncoding.EncodeToString([]byte(testCACertPEM(t)))

	client, err := NewDynamicClient(ClientConfig{
		APIServer: "https://k8s.example.com",
		CAData:    caData,
		TokenFunc: func(context.Context) (string, error) { return testToken, nil },
	})

	require.NoError(t, err)
	assert.NotNil(t, client)
}
