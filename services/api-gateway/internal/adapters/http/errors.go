package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// APIError es el formato único de error de la API pública.
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

type errorBody struct {
	Error APIError `json:"error"`
}

var grpcToHTTP = map[codes.Code]struct {
	status int
	code   string
}{
	codes.InvalidArgument:    {http.StatusBadRequest, "INVALID_ARGUMENT"},
	codes.Unauthenticated:    {http.StatusUnauthorized, "UNAUTHENTICATED"},
	codes.PermissionDenied:   {http.StatusForbidden, "FORBIDDEN"},
	codes.NotFound:           {http.StatusNotFound, "NOT_FOUND"},
	codes.AlreadyExists:      {http.StatusConflict, "CONFLICT"},
	codes.Aborted:            {http.StatusConflict, "CONFLICT"},
	codes.FailedPrecondition: {http.StatusUnprocessableEntity, "FAILED_PRECONDITION"},
	codes.ResourceExhausted:  {http.StatusTooManyRequests, "RATE_LIMITED"},
	codes.DeadlineExceeded:   {http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT"},
	codes.Unavailable:        {http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
}

var httpCodes = map[int]string{
	http.StatusBadRequest:            "INVALID_ARGUMENT",
	http.StatusUnauthorized:          "UNAUTHENTICATED",
	http.StatusForbidden:             "FORBIDDEN",
	http.StatusNotFound:              "NOT_FOUND",
	http.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
	http.StatusRequestEntityTooLarge: "PAYLOAD_TOO_LARGE",
	http.StatusUnsupportedMediaType:  "UNSUPPORTED_MEDIA_TYPE",
	http.StatusTooManyRequests:       "RATE_LIMITED",
	http.StatusServiceUnavailable:    "SERVICE_UNAVAILABLE",
}

// ErrorHandler transforma cualquier error (gRPC, Echo o desconocido) al formato APIError.
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status, body := translate(err)
	body.RequestID = c.Response().Header().Get(echo.HeaderXRequestID)
	if status >= http.StatusInternalServerError {
		c.Set("error", err.Error())
	}
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	_ = c.JSON(status, errorBody{Error: body})
}

func translate(err error) (int, APIError) {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		code, ok := httpCodes[he.Code]
		if !ok {
			code = "ERROR"
		}
		msg := http.StatusText(he.Code)
		if m, ok := he.Message.(string); ok && he.Code < http.StatusInternalServerError {
			msg = m
		}
		return he.Code, APIError{Code: code, Message: msg}
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		if m, ok := grpcToHTTP[st.Code()]; ok {
			return m.status, APIError{Code: m.code, Message: st.Message()}
		}
	}
	return http.StatusInternalServerError, APIError{Code: "INTERNAL", Message: "internal error"}
}
