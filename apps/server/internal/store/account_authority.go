package store

type authorityFailure string

func (e authorityFailure) Error() string { return string(e) }

const (
	ErrUnauthorized   authorityFailure = "unauthorized"
	ErrTokenLimit     authorityFailure = "token limit reached"
	ErrLoginFailed    authorityFailure = "login_failed"
	ErrSignupClosed   authorityFailure = "signup_closed"
	ErrAccountBlocked authorityFailure = "account_blocked"
	ErrFactorEnrolled authorityFailure = "personal tokens cannot be issued by the operator for an account with an enrolled factor"
	ErrReauthRequired authorityFailure = "reauth_required"
	ErrFactorRequired authorityFailure = "recent factor required"
)

type GoogleIdentity struct {
	Sub, Email   string
	Next         string
	PendingHash  string
	SignupEmails []string
}

type GoogleSessionParams struct {
	Identity       GoogleIdentity
	Session        CreateSessionParams
	ReplacedHashes []string
	Pending        PendingParams
}

type WebPersonalTokenParams struct {
	Token     CreatePersonalTokenParams
	SessionID int64
}
