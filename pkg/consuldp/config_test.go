// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/stretchr/testify/require"
)

func TestConfig_TLS(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := TLSConfig{Disabled: true}

		out, err := cfg.Load()
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("CACertsPath is a file", func(t *testing.T) {
		cfg := TLSConfig{
			CACertsPath: "testdata/certs/ca/cert.pem",
		}

		out, err := cfg.Load()
		require.NoError(t, err)

		cert := loadCertificate(t, "testdata/certs/server/cert.pem")
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:       out.RootCAs,
			CurrentTime: cert.NotBefore,
		})
		require.NoError(t, err)
	})

	t.Run("CACertsPath is a directory", func(t *testing.T) {
		cfg := TLSConfig{
			CACertsPath: "testdata/certs/ca",
		}

		out, err := cfg.Load()
		require.NoError(t, err)

		cert := loadCertificate(t, "testdata/certs/server/cert.pem")
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:       out.RootCAs,
			CurrentTime: cert.NotBefore,
		})
		require.NoError(t, err)
	})

	t.Run("setting a client certificate", func(t *testing.T) {
		cfg := TLSConfig{
			CertFile: "testdata/certs/server/cert.pem",
			KeyFile:  "testdata/certs/server/key.pem",
		}

		out, err := cfg.Load()
		require.NoError(t, err)

		require.Len(t, out.Certificates, 1)

		cert, err := x509.ParseCertificate(out.Certificates[0].Certificate[0])
		require.NoError(t, err)
		require.Equal(t, "server.dc1.consul", cert.Subject.CommonName)

		require.NotNil(t, out.Certificates[0].PrivateKey)
	})
}

func TestConfig_Credentials(t *testing.T) {
	tokFile, err := os.CreateTemp(os.TempDir(), "bearer-token")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(tokFile.Name()) })
	t.Cleanup(func() { _ = tokFile.Close() })

	_, err = tokFile.WriteString("bearer-token-from-file")
	require.NoError(t, err)

	testCases := map[string]struct {
		in  CredentialsConfig
		out discovery.Credentials
	}{
		"no credentials": {
			in:  CredentialsConfig{Type: CredentialsTypeNone},
			out: discovery.Credentials{},
		},
		"static credentials": {
			in: CredentialsConfig{
				Type: CredentialsTypeStatic,
				Static: StaticCredentialsConfig{
					Token: "my-acl-token",
				},
			},
			out: discovery.Credentials{
				Type: discovery.CredentialsTypeStatic,
				Static: discovery.StaticTokenCredential{
					Token: "my-acl-token",
				},
			},
		},
		"login credentials (bearer token)": {
			in: CredentialsConfig{
				Type: CredentialsTypeLogin,
				Login: LoginCredentialsConfig{
					AuthMethod:  "jwt",
					Namespace:   "namespace-1",
					Partition:   "partition-a",
					Datacenter:  "primary-dc",
					BearerToken: "bearer-token",
					Meta:        map[string]string{"foo": "bar"},
				},
			},
			out: discovery.Credentials{
				Type: discovery.CredentialsTypeLogin,
				Login: discovery.LoginCredential{
					AuthMethod:  "jwt",
					Namespace:   "namespace-1",
					Partition:   "partition-a",
					Datacenter:  "primary-dc",
					BearerToken: "bearer-token",
					Meta:        map[string]string{"foo": "bar"},
				},
			},
		},
		"login credentials (bearer file)": {
			in: CredentialsConfig{
				Type: CredentialsTypeLogin,
				Login: LoginCredentialsConfig{
					AuthMethod:      "jwt",
					Namespace:       "namespace-1",
					Partition:       "partition-a",
					Datacenter:      "primary-dc",
					BearerTokenPath: tokFile.Name(),
					Meta:            map[string]string{"foo": "bar"},
				},
			},
			out: discovery.Credentials{
				Type: discovery.CredentialsTypeLogin,
				Login: discovery.LoginCredential{
					AuthMethod:  "jwt",
					Namespace:   "namespace-1",
					Partition:   "partition-a",
					Datacenter:  "primary-dc",
					BearerToken: "bearer-token-from-file",
					Meta:        map[string]string{"foo": "bar"},
				},
			},
		},
	}
	for desc, tc := range testCases {
		t.Run(desc, func(t *testing.T) {
			got, err := tc.in.ToDiscoveryCredentials()
			require.NoError(t, err)
			require.Equal(t, tc.out, got)
		})
	}
}

func loadCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	pemBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	block, _ := pem.Decode(pemBytes)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	return cert
}
