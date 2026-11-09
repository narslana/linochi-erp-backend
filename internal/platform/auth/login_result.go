package auth

// Reason is why a login ended as it did. It is the label of
// linochi_auth_logins_total, so the set is closed: a value that is not one of
// these must never be made up from user input. The browser is never told the
// reason, only the Code it maps to; the log has the reason and the trace_id.
type Reason string

const (
	// ReasonOK is a login that made a session.
	ReasonOK Reason = "ok"

	// Failures of the exchange with the provider (OIDCClient).

	// ReasonProviderUnavailable is a provider that could not be reached or whose
	// discovery document could not be trusted (also an untrusted certificate).
	ReasonProviderUnavailable Reason = "provider_unavailable"
	// ReasonExchangeFailed is the token endpoint refusing the code or the client.
	ReasonExchangeFailed Reason = "exchange_failed"
	// ReasonTokenInvalid is an ID token that is not signed, issued, addressed or
	// timed right.
	ReasonTokenInvalid Reason = "token_invalid"
	// ReasonNonceMismatch is an ID token made for another login.
	ReasonNonceMismatch Reason = "nonce_mismatch"
	// ReasonMFARequired is a login that did not use two factors.
	ReasonMFARequired Reason = "mfa_missing"
	// ReasonUserInfoFailed is the userinfo endpoint failing.
	ReasonUserInfoFailed Reason = "userinfo_failed"
	// ReasonSubjectMismatch is userinfo describing another person than the ID token.
	ReasonSubjectMismatch Reason = "subject_mismatch"
	// ReasonClaimsMissing is a provider that did not give what a user record needs.
	ReasonClaimsMissing Reason = "claims_missing"

	// Failures of the callback itself (the handler).

	// ReasonAttemptUnknown is a state that matches no login in progress.
	ReasonAttemptUnknown Reason = "attempt_unknown"
	// ReasonAttemptExpired is a login that took longer than LoginAttemptTTL.
	ReasonAttemptExpired Reason = "attempt_expired"
	// ReasonBindingMismatch is a callback from another browser than the one that
	// started the login.
	ReasonBindingMismatch Reason = "binding_mismatch"
	// ReasonProviderDenied is the provider saying the person may not use the app.
	ReasonProviderDenied Reason = "provider_denied"
	// ReasonProviderError is the provider answering the browser with another error.
	ReasonProviderError Reason = "provider_error"
	// ReasonBadCallback is a callback with no state or no code.
	ReasonBadCallback Reason = "bad_callback"
	// ReasonNoRole is a person Authelia let in who has no group that gives a role.
	ReasonNoRole Reason = "no_role"
	// ReasonUserArchived is a person whose account has been archived.
	ReasonUserArchived Reason = "user_archived"
	// ReasonTooManyAttempts is the cap on logins in progress being reached.
	ReasonTooManyAttempts Reason = "too_many_attempts"
	// ReasonInternal is anything that is our own fault, a database that is down for
	// instance.
	ReasonInternal Reason = "internal_error"
)

// ErrorCode is the only thing the browser is told about a login that failed: a
// fixed code in the URL of the web app's error page, never a detail.
type ErrorCode string

const (
	// CodeNoAccess is a person who is not allowed in.
	CodeNoAccess ErrorCode = "no-access"
	// CodeExpired is a login that took too long, was used twice or was lost.
	CodeExpired ErrorCode = "expired"
	// CodeFailed is everything else.
	CodeFailed ErrorCode = "failed"
)

// LoginErrorPath is the page of the web app that explains a failed login. It is
// relative on purpose: the backend and the web app share an origin.
const LoginErrorPath = "/login-error"

// Code is what the browser is told for this reason.
func (r Reason) Code() ErrorCode {
	switch r {
	case ReasonMFARequired, ReasonProviderDenied, ReasonNoRole, ReasonUserArchived:
		return CodeNoAccess
	case ReasonAttemptUnknown, ReasonAttemptExpired:
		return CodeExpired
	default:
		return CodeFailed
	}
}

// LoginFailure is an error that carries the Reason it counts under.
type LoginFailure struct {
	Reason Reason
	Err    error
}

func (e *LoginFailure) Error() string {
	return "login failed (" + string(e.Reason) + "): " + e.Err.Error()
}
func (e *LoginFailure) Unwrap() error { return e.Err }

// allReasons is every Reason, so that the login counter has every series from
// the start: a rate() over a series that has never been written is empty, and an
// alert on "many refused logins" must not depend on the first one having happened.
var allReasons = []Reason{
	ReasonOK, ReasonProviderUnavailable, ReasonExchangeFailed, ReasonTokenInvalid, ReasonNonceMismatch,
	ReasonMFARequired, ReasonUserInfoFailed, ReasonSubjectMismatch, ReasonClaimsMissing,
	ReasonAttemptUnknown, ReasonAttemptExpired, ReasonBindingMismatch, ReasonProviderDenied, ReasonProviderError,
	ReasonBadCallback, ReasonNoRole, ReasonUserArchived, ReasonTooManyAttempts, ReasonInternal,
}
