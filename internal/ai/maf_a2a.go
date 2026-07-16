package ai

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/provider/a2aprovider"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// newA2AAgent connects to a remote or local Agent-to-Agent (A2A) agent by
// resolving its card at p.BaseURL and delegating the turn to it. Local
// instructions and tools don't apply — the remote agent runs its own.
func newA2AAgent(ctx context.Context, p Provider, _ agent.Config) (*agent.Agent, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("the A2A provider needs the agent's card URL as its base URL")
	}
	card, err := agentcard.DefaultResolver.Resolve(ctx, p.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("resolve A2A agent card at %q: %w", p.BaseURL, err)
	}

	// gRPC credentials follow the card URL scheme: plaintext for a local http://
	// agent (e.g. one you run on localhost), TLS otherwise. All three transports
	// are registered so NewFromCard picks whichever the card advertises (gRPC,
	// JSON-RPC, or REST) — the HTTP ones handle both http:// and https:// via the
	// default client.
	var grpcCreds grpc.DialOption
	if strings.HasPrefix(strings.ToLower(p.BaseURL), "http://") {
		grpcCreds = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		grpcCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{}))
	}

	client, err := a2aclient.NewFromCard(ctx, card,
		a2agrpc.WithGRPCTransport(grpcCreds),
		a2aclient.WithJSONRPCTransport(http.DefaultClient),
		a2aclient.WithRESTTransport(http.DefaultClient),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to A2A agent: %w", err)
	}
	return a2aprovider.NewAgent(client, a2aprovider.AgentConfig{Config: agent.Config{Name: "novelide"}}), nil
}
