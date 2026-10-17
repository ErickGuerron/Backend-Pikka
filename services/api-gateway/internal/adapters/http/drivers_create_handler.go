package http

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
)

// driverAdminHandlers da de alta repartidores: un usuario DRIVER en Auth Service y su perfil en
// Driver Service. Son dos servicios con base de datos propia, así que no hay transacción común.
type driverAdminHandlers struct {
	auth    AuthClient
	drivers DriverClient
	log     *slog.Logger
}

type createDriverRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	BirthDate   string `json:"birthDate"`
	Phone       string `json:"phone"`
	VehicleType string `json:"vehicleType"`
	Identifier  string `json:"identifier"`
	Plate       string `json:"plate"`
	City        string `json:"city"`
}

type driverProfileDTO struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	BirthDate   string `json:"birthDate"`
	Phone       string `json:"phone"`
	VehicleType string `json:"vehicleType"`
	Identifier  string `json:"identifier"`
	Plate       string `json:"plate"`
	City        string `json:"city"`
	Active      bool   `json:"active"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

type createDriverResponse struct {
	User   userDTO          `json:"user"`
	Driver driverProfileDTO `json:"driver"`
}

// create valida la forma de la fecha y del vehículo antes de tocar nada, crea el usuario DRIVER y
// después el perfil. Si el perfil falla, desactiva el usuario recién creado (compensación) y
// devuelve el error original: Auth Service no borra usuarios, así que el correo queda reservado.
func (h *driverAdminHandlers) create(c echo.Context) error {
	var req createDriverRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	birth := strings.TrimSpace(req.BirthDate)
	if _, err := time.Parse(time.DateOnly, birth); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "birthDate must be YYYY-MM-DD")
	}
	vehicle, ok := vehicleToProto(req.VehicleType)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "vehicleType must be MOTORIZADO, AUTOMOVIL or SUV")
	}

	ctx := c.Request().Context()
	meta := callMeta(c)
	created, err := h.auth.CreateUser(ctx, meta, &authv1.CreateUserRequest{
		Email:    req.Email,
		Password: req.Password,
		FullName: strings.TrimSpace(req.FirstName) + " " + strings.TrimSpace(req.LastName),
		Role:     authv1.Role_ROLE_DRIVER,
	})
	if err != nil {
		return err
	}
	user := created.GetUser()

	profile, err := h.drivers.CreateDriver(ctx, meta, &driversv1.CreateDriverRequest{
		UserId:      user.GetId(),
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		BirthDate:   birth,
		Phone:       req.Phone,
		VehicleType: vehicle,
		Identifier:  req.Identifier,
		Plate:       req.Plate,
		City:        req.City,
	})
	if err != nil {
		if _, derr := h.auth.DeactivateUser(ctx, meta, &authv1.DeactivateUserRequest{Id: user.GetId()}); derr != nil {
			h.log.Error("driver profile failed and the created user could not be deactivated",
				"userId", user.GetId(), "requestId", meta.RequestID, "error", derr)
		}
		return err
	}
	return c.JSON(http.StatusCreated, createDriverResponse{User: toUserDTO(user), Driver: toDriverProfileDTO(profile.GetDriver())})
}

// driverListItemDTO es la vista de la flota para el personal: sin fecha de nacimiento ni identificador.
type driverListItemDTO struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	FullName    string `json:"fullName"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Phone       string `json:"phone"`
	City        string `json:"city"`
	VehicleType string `json:"vehicleType"`
	Plate       string `json:"plate"`
	Active      bool   `json:"active"`
	Status      string `json:"status"`
}

// list responde un arreglo (vacío si no hay repartidores) con la flota completa.
func (h *driverAdminHandlers) list(c echo.Context) error {
	res, err := h.drivers.ListDrivers(c.Request().Context(), callMeta(c), &driversv1.ListDriversRequest{})
	if err != nil {
		return err
	}
	out := make([]driverListItemDTO, 0, len(res.GetDrivers()))
	for _, d := range res.GetDrivers() {
		out = append(out, driverListItemDTO{
			ID: d.GetId(), UserID: d.GetUserId(), FullName: d.GetFirstName() + " " + d.GetLastName(),
			FirstName: d.GetFirstName(), LastName: d.GetLastName(), Phone: d.GetPhone(), City: d.GetCity(),
			VehicleType: vehicleFromProto(d.GetVehicleType()), Plate: d.GetPlate(), Active: d.GetActive(),
			Status: driverStatusFromProto(d.GetStatus()),
		})
	}
	return c.JSON(http.StatusOK, out)
}

func toDriverProfileDTO(p *driversv1.DriverProfile) driverProfileDTO {
	return driverProfileDTO{
		ID: p.GetId(), UserID: p.GetUserId(), FirstName: p.GetFirstName(), LastName: p.GetLastName(),
		BirthDate: p.GetBirthDate(), Phone: p.GetPhone(), VehicleType: vehicleFromProto(p.GetVehicleType()),
		Identifier: p.GetIdentifier(), Plate: p.GetPlate(), City: p.GetCity(), Active: p.GetActive(),
		Status: driverStatusFromProto(p.GetStatus()), CreatedAt: p.GetCreatedAt(),
	}
}

func vehicleToProto(v string) (driversv1.VehicleType, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "MOTORIZADO":
		return driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE, true
	case "AUTOMOVIL":
		return driversv1.VehicleType_VEHICLE_TYPE_CAR, true
	case "SUV":
		return driversv1.VehicleType_VEHICLE_TYPE_SUV, true
	default:
		return driversv1.VehicleType_VEHICLE_TYPE_UNSPECIFIED, false
	}
}

func vehicleFromProto(v driversv1.VehicleType) string {
	switch v {
	case driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE:
		return "MOTORIZADO"
	case driversv1.VehicleType_VEHICLE_TYPE_CAR:
		return "AUTOMOVIL"
	case driversv1.VehicleType_VEHICLE_TYPE_SUV:
		return "SUV"
	default:
		return ""
	}
}

func driverStatusFromProto(s driversv1.DriverStatus) string {
	switch s {
	case driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE:
		return "AVAILABLE"
	case driversv1.DriverStatus_DRIVER_STATUS_ASSIGNED:
		return "ASSIGNED"
	case driversv1.DriverStatus_DRIVER_STATUS_ON_ROUTE:
		return "ON_ROUTE"
	case driversv1.DriverStatus_DRIVER_STATUS_OFFLINE:
		return "OFFLINE"
	default:
		return ""
	}
}
