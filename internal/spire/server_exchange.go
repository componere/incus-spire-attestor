package spire

import (
	"context"
	"slices"

	nodeattestorv1 "github.com/spiffe/spire-plugin-sdk/proto/spire/plugin/server/nodeattestor/v1"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/wire"
)

// recvResult is one bounded Recv outcome from the handler-local worker.
type recvResult struct {
	// req is the received attestation request.
	req *nodeattestorv1.AttestRequest
	// err is the Recv failure, if any.
	err error
}

// serverExchange maps the server NodeAttestor stream onto server.Exchange.
type serverExchange struct {
	// stream is the Attest bidirectional stream.
	stream nodeattestorv1.NodeAttestor_AttestServer
}

// ReceivePayload receives the first stream message as guest-claims payload.
//
// ReceivePayload calls Recv synchronously and accepts only AttestRequest_Payload.
// Another oneof or a nil request is wire.ErrInvalid.
func (e *serverExchange) ReceivePayload(_ context.Context) ([]byte, error) {
	req, err := e.stream.Recv()
	if err != nil {
		return nil, err
	}
	payload, ok := req.GetRequest().(*nodeattestorv1.AttestRequest_Payload)
	if !ok {
		return nil, wire.ErrInvalid
	}
	return payload.Payload, nil
}

// SendChallenge sends the config-nonce challenge as AttestResponse_Challenge.
func (e *serverExchange) SendChallenge(_ context.Context, challenge []byte) error {
	return e.stream.Send(&nodeattestorv1.AttestResponse{
		Response: &nodeattestorv1.AttestResponse_Challenge{Challenge: challenge},
	})
}

// ReceiveResponse receives the challenge-response under ctx and stream context.
//
// ReceiveResponse starts exactly one handler-local goroutine around the
// blocking second Recv and a buffered result channel. It selects over the
// result, ctx.Done(), and the stream context. It accepts only
// AttestRequest_ChallengeResponse; another oneof or a nil request is
// wire.ErrInvalid. Returning on timeout or cancellation lets RPC/stream
// cancellation release the blocked Recv.
func (e *serverExchange) ReceiveResponse(ctx context.Context) ([]byte, error) {
	results := make(chan recvResult, 1)
	go func() {
		req, err := e.stream.Recv()
		results <- recvResult{req: req, err: err}
	}()

	select {
	case result := <-results:
		if result.err != nil {
			return nil, result.err
		}
		response, ok := result.req.GetRequest().(*nodeattestorv1.AttestRequest_ChallengeResponse)
		if !ok {
			return nil, wire.ErrInvalid
		}
		return response.ChallengeResponse, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.stream.Context().Done():
		return nil, e.stream.Context().Err()
	}
}

// SendAttributes sends terminal AgentAttributes with copied selector values.
func (e *serverExchange) SendAttributes(_ context.Context, attrs attest.Attributes) error {
	return e.stream.Send(&nodeattestorv1.AttestResponse{
		Response: &nodeattestorv1.AttestResponse_AgentAttributes{
			AgentAttributes: &nodeattestorv1.AgentAttributes{
				SpiffeId:       attrs.AgentID,
				CanReattest:    attrs.CanReattest,
				SelectorValues: slices.Clone(attrs.Selectors),
			},
		},
	})
}
