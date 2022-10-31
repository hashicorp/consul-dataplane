package integrationtests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"

	"github.com/hashicorp/consul/api"
)

// AuthMethod is a JWT ACL auth-method, that allows us to easily generate
// bearer tokens in tests.
type AuthMethod struct {
	key *ecdsa.PrivateKey
}

func NewAuthMethod(t *testing.T) *AuthMethod {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	return &AuthMethod{key: key}
}

// GenerateToken generates a JWT bearer token for the given service's identity.
func (am *AuthMethod) GenerateToken(t *testing.T, service string) string {
	t.Helper()

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.ES256,
		Key:       am.key,
	}, nil)
	require.NoError(t, err)

	claims := &jwt.Claims{
		Subject:  service,
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
		Issuer:   am.jwtIssuer(),
		Audience: am.jwtAudience(),
	}

	token, err := jwt.Signed(signer).
		Claims(claims).
		CompactSerialize()
	require.NoError(t, err)

	return token
}

// Register the auth-method and binding rules with the Consul server.
func (am *AuthMethod) Register(t *testing.T, server *ConsulServer) {
	t.Helper()

	_, _, err := server.Client.ACL().AuthMethodCreate(&api.ACLAuthMethod{
		Name: am.name(),
		Type: "jwt",
		Config: map[string]any{
			"BoundIssuer":          am.jwtIssuer(),
			"BoundAudiences":       am.jwtAudience(),
			"JWTValidationPubKeys": []string{am.publicKeyPEM(t)},
			"ClaimMappings":        map[string]string{"sub": "service"},
		},
	}, nil)
	require.NoError(t, err)

	_, _, err = server.Client.ACL().BindingRuleCreate(&api.ACLBindingRule{
		AuthMethod: am.name(),
		BindType:   api.BindingRuleBindTypeService,
		BindName:   "${value.service}",
	}, nil)
	require.NoError(t, err)
}

func (am *AuthMethod) publicKeyPEM(t *testing.T) string {
	t.Helper()

	der, err := x509.MarshalPKIXPublicKey(&am.key.PublicKey)
	require.NoError(t, err)

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
	return string(keyPem)
}

func (*AuthMethod) name() string {
	return "auth-method"
}

func (*AuthMethod) jwtIssuer() string {
	return "issuer"
}

func (*AuthMethod) jwtAudience() jwt.Audience {
	return jwt.Audience{"audience"}
}
