package accounts

import (
	"context"
	"io"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SessionLogin carries ephemeral provider output and code input. Input is
// closed on completion/cancellation, including browser-only login completion.
type SessionLogin struct {
	Project, Account, Key string
	// Identity is the account's pinned provider identity, empty when it has
	// none. A login that reaches a different one is refused rather than
	// installed, so this has to arrive before the provider is run.
	Identity string
	Input    io.ReadCloser
	Output   io.Writer
}

// SessionLoginResult reports the identity the accepted login reached, and is
// empty when the credential did not say or nothing was logged in.
type SessionLoginResult struct{ Identity string }

type SessionLoginClient interface {
	LoginSession(ctx context.Context, r SessionLogin) (SessionLoginResult, error)
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

// AuxiliaryLoginClient authenticates a dedicated helper profile on the selected Manager.
type AuxiliaryLoginClient interface {
	LoginAuxiliary(context.Context, string, io.ReadCloser, io.Writer) error
}
