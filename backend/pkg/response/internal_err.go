package response

import "net/http"

// StatusClientClosedRequest is the non-standard 499 used when the client went
// away before the handler finished (nginx convention). Never sent to a live
// client: by definition nobody is listening.
const StatusClientClosedRequest = 499

type serverErrorRecorder interface {
	RecordServerError(err error, publicMessage string)
}

type errorCodeRecorder interface {
	RecordErrorCode(code string)
}

type failureRecorder interface {
	RecordFailure(err error)
}

// InternalErr writes a generic 500 response and records err for server-side
// logging. When the request context is already cancelled (client disconnected
// or aborted the request) it writes 499 instead: that is not a server fault.
func InternalErr(w http.ResponseWriter, r *http.Request, err error, publicMessage string) {
	if r != nil && r.Context().Err() != nil {
		if rec, ok := w.(failureRecorder); ok {
			if err == nil {
				err = r.Context().Err()
			}
			rec.RecordFailure(err)
		}
		Error(w, r, StatusClientClosedRequest, "CLIENT_CLOSED_REQUEST", "Client closed request")
		return
	}
	if err != nil {
		if rec, ok := w.(serverErrorRecorder); ok {
			rec.RecordServerError(err, publicMessage)
		}
	}
	Internal(w, r, publicMessage)
}

// RecordFailure attaches the cause of a 4xx response to the request so the
// access log can show why (e.g. why a sign-in was rejected). It never changes
// the response. Messages are scrubbed before they are logged.
func RecordFailure(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if rec, ok := w.(failureRecorder); ok {
		rec.RecordFailure(err)
	}
}
