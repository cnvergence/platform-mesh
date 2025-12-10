package github

import (
	"context"
	"net/http"

	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub GraphQL client
type Client struct {
	gql *graphql.Client
}

// NewClient creates a new GitHub GraphQL client with the provided token
func NewClient(token string) *Client {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	return &Client{
		gql: graphql.NewClient("https://api.github.com/graphql", httpClient),
	}
}

// NewClientWithHTTP creates a new GitHub GraphQL client with a custom HTTP client
func NewClientWithHTTP(httpClient *http.Client) *Client {
	return &Client{
		gql: graphql.NewClient("https://api.github.com/graphql", httpClient),
	}
}

// Query executes a GraphQL query
func (c *Client) Query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	return c.gql.Query(ctx, q, variables)
}
