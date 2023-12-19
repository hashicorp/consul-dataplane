package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-cleanhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	// defaultAuthURL is the URL of the production auth endpoint.
	defaultAuthURL = "https://auth.idp.hashicorp.com"

	// The audience is the API identifier configured in the auth provider and
	// must be provided when requesting an access token for the API. The value
	// is the same regardless of environment.
	aud = "https://api.hashicorp.cloud"

	// tokenPath is the path used to retrieve the access token.
	tokenPath string = "/oauth2/token"
)

var defaultConfig *Config = &Config{}

type Config struct {
	// AuthURL is the URL that will be used to authenticate.
	AuthURL *url.URL
	// AuthTLSConfig is the TLS configuration for the auth endpoint. TLS can not
	// be disabled for the auth endpoint, but the configuration can be set to a
	// custom one for tests or development.
	AuthTLSConfig *tls.Config
}

func (c *Config) canonicalize() {
	if c.AuthURL == nil {
		c.AuthURL, _ = url.Parse(defaultAuthURL)
	}
	if c.AuthTLSConfig == nil {
		c.AuthTLSConfig = &tls.Config{}
	}
}

func (c *Config) TokenSource(clientID, clientSecret string) oauth2.TokenSource {
	c.canonicalize()
	tokenTransport := cleanhttp.DefaultPooledTransport()
	tokenTransport.TLSClientConfig = c.AuthTLSConfig
	ctx := context.WithValue(
		context.Background(),
		oauth2.HTTPClient,
		&http.Client{Transport: tokenTransport},
	)

	// Set client credentials token URL based on auth URL.
	tokenURL := c.AuthURL
	tokenURL.Path = tokenPath

	clientCredentials := clientcredentials.Config{
		EndpointParams: url.Values{"audience": {aud}},
		TokenURL:       tokenURL.String(),
		ClientID:       clientID,
		ClientSecret:   clientSecret,
	}

	return clientCredentials.TokenSource(ctx)
}

func TokenSource(clientID, clientSecret string) oauth2.TokenSource {
	return defaultConfig.TokenSource(clientID, clientSecret)
}
