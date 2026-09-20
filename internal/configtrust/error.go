package configtrust

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

const Prefix = "devcontainer configuration requires explicit trust:\n"
const Suffix = "inspect these settings and explicitly pass --trust-config if trusted (configuration values omitted)"

type Required struct{ Findings []string }

func (e *Required) Error() string { return Prefix + strings.Join(e.Findings, "\n") + "\n" + Suffix }
func (e *Required) GRPCStatus() *status.Status {
	s := status.New(codes.FailedPrecondition, e.Error())
	detailed, err := s.WithDetails(&errdetails.ErrorInfo{Domain: "cxz.config", Reason: "TRUST_REQUIRED"})
	if err != nil {
		return s
	}
	return detailed
}
func IsRequired(err error) bool {
	if err == nil {
		return false
	}
	st := status.Convert(err)
	for _, v := range st.Details() {
		if info, ok := v.(*errdetails.ErrorInfo); ok && info.Domain == "cxz.config" && info.Reason == "TRUST_REQUIRED" {
			return true
		}
	}
	// Older managers returned this exact diagnostic as Unknown, without details.
	return st.Code() == codes.Unknown && strings.Contains(st.Message(), Prefix) && strings.HasSuffix(st.Message(), Suffix)
}
