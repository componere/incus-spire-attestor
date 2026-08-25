package spire

import (
	"context"

	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/agent/nodeattestor/v1"

	"github.com/componere/incus-spire-attestor/internal/wire"
)

// agentExchange maps the agent NodeAttestor stream onto agent.Exchange.
type agentExchange struct {
	// stream is the AidAttestation bidirectional stream.
	stream nodeattestorv1.NodeAttestor_AidAttestationServer
}

// SendPayload sends the guest-claims payload as the first stream response.
func (e *agentExchange) SendPayload(_ context.Context, payload []byte) error {
	return e.stream.Send(&nodeattestorv1.PayloadOrChallengeResponse{
		Data: &nodeattestorv1.PayloadOrChallengeResponse_Payload{Payload: payload},
	})
}

// ReceiveChallenge receives one challenge message from the stream.
//
// A nil Challenge or nil challenge field is wire.ErrInvalid. ReceiveChallenge
// does not start a receive goroutine; the stream context bounds Recv.
func (e *agentExchange) ReceiveChallenge(_ context.Context) ([]byte, error) {
	msg, err := e.stream.Recv()
	if err != nil {
		return nil, err
	}
	if msg == nil || msg.Challenge == nil {
		return nil, wire.ErrInvalid
	}
	return msg.Challenge, nil
}

// SendResponse sends the nonce response as the challenge-response message.
func (e *agentExchange) SendResponse(_ context.Context, response []byte) error {
	return e.stream.Send(&nodeattestorv1.PayloadOrChallengeResponse{
		Data: &nodeattestorv1.PayloadOrChallengeResponse_ChallengeResponse{ChallengeResponse: response},
	})
}
