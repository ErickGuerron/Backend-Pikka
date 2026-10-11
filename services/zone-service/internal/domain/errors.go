package domain

import "errors"

// Errores de dominio. Los adaptadores los traducen a códigos de transporte.
var (
	ErrInvalidCode       = errors.New("zone code is required and must be at most 32 characters")
	ErrInvalidName       = errors.New("zone name is required and must be at most 120 characters")
	ErrInvalidCoordinate = errors.New("coordinate is out of range")
	ErrInvalidRing       = errors.New("zone needs at least 4 vertices (plus the closing point)")
	ErrRingNotClosed     = errors.New("zone ring must be closed: first and last points must match")
	ErrNotConvex         = errors.New("zone must be a convex polygon: no crossing sides and no inward corners")
	ErrZoneNotFound      = errors.New("zone not found")
	ErrZoneCodeTaken     = errors.New("zone code already exists")
)
