package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFirstName      = fmt.Errorf("first name is required and must be at most %d characters", MaxNameRunes)
	ErrInvalidLastName       = fmt.Errorf("last name is required and must be at most %d characters", MaxNameRunes)
	ErrInvalidBirthDate      = fmt.Errorf("birth date is required and the driver must be at least %d years old", MinDriverAge)
	ErrInvalidVehicleType    = errors.New("vehicle type must be MOTORIZADO, AUTOMOVIL or SUV")
	ErrInvalidIdentifier     = fmt.Errorf("identifier must have between %d and %d letters or digits", MinIdentifierLen, MaxIdentifierLen)
	ErrInvalidPlate          = fmt.Errorf("plate must have between %d and %d letters or digits", MinPlateLen, MaxPlateLen)
	ErrInvalidCity           = fmt.Errorf("city is required and must be at most %d characters", MaxCityRunes)
	ErrInvalidPhone          = fmt.Errorf("phone must have between %d and %d digits and an optional leading +", MinPhoneDigits, MaxPhoneDigits)
	ErrInvalidUserID         = errors.New("user id is required")
	ErrInvalidDriverID       = errors.New("driver id is required")
	ErrInvalidStatus         = errors.New("invalid driver status")
	ErrInvalidTransition     = errors.New("invalid status transition")
	ErrDriverInactive        = errors.New("driver is inactive")
	ErrInvalidRouteID        = errors.New("route id is required")
	ErrDriverAlreadyReserved = errors.New("driver is already reserved for another route")
	ErrDriverOffline         = errors.New("driver is offline")
	ErrRouteMismatch         = errors.New("route does not match the driver's current reservation")
)
