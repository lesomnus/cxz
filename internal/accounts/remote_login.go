package accounts

import (
	"context"
	"io"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SessionLoginClient carries ephemeral provider output and code input. Input is
// closed on completion/cancellation, including browser-only login completion.
type SessionLoginClient interface {
	LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error
}

// RequiredProjectLogin recognizes only the runtime's structured missing-login
// response. Corrupt credentials, central login and other accounts do not match.
func RequiredProjectLogin(err error, account, fallbackKey string) (alias, key string, ok bool) {
	s := status.Convert(err)
	if s.Code() != codes.FailedPrecondition {
		return "", "", false
	}
	for _, detail := range s.Details() {
		info, matched := detail.(*errdetails.ErrorInfo)
		if !matched || info.Domain != "cxz.auth" || info.Reason != "PROJECT_LOGIN_REQUIRED" {
			continue
		}
		alias, key = info.Metadata["account"], info.Metadata["session_key"]
		if key == "" {
			key = fallbackKey
		}
		if Validate(alias, "claude") == nil && (account == "" || account == alias) {
			return alias, key, true
		}
	}
	return "", "", false
}
