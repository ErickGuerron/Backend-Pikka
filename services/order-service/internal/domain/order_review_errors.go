package domain

import "errors"

var (
	// ErrOrderNotFound: no existe un pedido con ese id.
	ErrOrderNotFound = errors.New("order not found")

	// ErrOrderNotInReview: la decisión del operador solo aplica a pedidos PENDING_REVIEW.
	ErrOrderNotInReview = errors.New("order is not in review")

	// ErrSpatialReasonNotOverridable: un pedido fuera de zona o sin localizar no se confirma
	// tal cual; el operador debe corregir la dirección o rechazarlo.
	ErrSpatialReasonNotOverridable = errors.New("order outside zones or without a located address must be corrected or rejected")

	// ErrGeocoderUnavailable: el servicio de geocodificación no respondió; no se valida a ciegas.
	ErrGeocoderUnavailable = errors.New("geocoder is unavailable")

	// ErrNotOperator: solo ADMIN y OPERATOR revisan pedidos.
	ErrNotOperator = errors.New("only administrators and operators can review orders")

	// ErrReasonRequired: toda decisión del operador queda registrada con motivo (5 a 300 caracteres).
	ErrReasonRequired = errors.New("review reason must contain 5 to 300 characters")
)
