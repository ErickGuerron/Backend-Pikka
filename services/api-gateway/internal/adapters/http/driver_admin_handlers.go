package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// DriverCreator is what driver creation needs from the Driver Service.
type DriverCreator interface {
	CreateDriver(ctx context.Context, m grpcclient.CallMeta, req *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error)
}

// driverAdminHandlers maps POST /api/v1/drivers to Driver Service CreateDriver. AdminOnly limits the
// route to ADMIN. Before creating the profile it looks the user up in Auth and only proceeds when the
// user has the DRIVER role and is active.
type driverAdminHandlers struct {
	auth    AuthClient
	drivers DriverCreator
}

type createDriverRequest struct {
	UserID   string `json:"userId"`
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}

type driverDTO struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	FullName  string `json:"fullName"`
	Phone     string `json:"phone"`
	Active    bool   `json:"active"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// create responds 201 with the driver. A missing user comes back as 404 from Auth; a user with another
// role, or an inactive user, as 422 (FAILED_PRECONDITION); a user who already has a profile as 409 (ALREADY_EXISTS).
func (h *driverAdminHandlers) create(c echo.Context) error {
	var req createDriverRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	userID := strings.TrimSpace(req.UserID)
	fullName := strings.TrimSpace(req.FullName)
	phone := strings.TrimSpace(req.Phone)
	if userID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "userId is required")
	}
	if fullName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "fullName is required")
	}
	if phone == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "phone is required")
	}

	ctx := c.Request().Context()
	m := callMeta(c)
	user, err := h.auth.GetUser(ctx, m, &authv1.GetUserRequest{Id: userID})
	if err != nil {
		return err
	}
	if user.GetUser().GetRole() != authv1.Role_ROLE_DRIVER {
		return status.Error(codes.FailedPrecondition, "user must have the DRIVER role")
	}
	if !user.GetUser().GetActive() {
		return status.Error(codes.FailedPrecondition, "user must be active")
	}

	res, err := h.drivers.CreateDriver(ctx, m, &driversv1.CreateDriverRequest{
		UserId:   userID,
		FullName: fullName,
		Phone:    phone,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toDriverDTO(res.GetDriver()))
}

func toDriverDTO(d *driversv1.Driver) driverDTO {
	return driverDTO{
		ID:        d.GetId(),
		UserID:    d.GetUserId(),
		FullName:  d.GetFullName(),
		Phone:     d.GetPhone(),
		Active:    d.GetActive(),
		Status:    strings.TrimPrefix(d.GetStatus().String(), "DRIVER_STATUS_"),
		CreatedAt: d.GetCreatedAt(),
	}
}
