package lifecycle

import (
	"context"
	"crypto/rand"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math/big"
)

// Called under shared.mu; the database unique index is the final authority.
func (s Layer) newSessionAlias(ctx context.Context) (string, error) {
	start, err := rand.Int(rand.Reader, big.NewInt(int64(len(sessionalias.Words))))
	if err != nil {
		return "", err
	}
	for i := range sessionalias.Words {
		word := sessionalias.Words[(int(start.Int64())+i)%len(sessionalias.Words)]
		_, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: &word}.Build()}.Build())
		if status.Code(err) == codes.NotFound {
			return word, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", status.Error(codes.ResourceExhausted, "session word aliases exhausted; rename unused sessions to free a generated word")
}
