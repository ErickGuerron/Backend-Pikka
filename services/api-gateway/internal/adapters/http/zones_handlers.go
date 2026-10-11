package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// ZonesClient es lo que los handlers necesitan del Zone Service.
type ZonesClient interface {
	CreateZone(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.CreateZoneRequest) (*zonesv1.CreateZoneResponse, error)
	GetZone(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error)
	ClassifyPoint(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error)
	ListZones(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.ListZonesRequest) (*zonesv1.ListZonesResponse, error)
	UpdateZone(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.UpdateZoneRequest) (*zonesv1.UpdateZoneResponse, error)
	SetZoneStatus(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.SetZoneStatusRequest) (*zonesv1.SetZoneStatusResponse, error)
	DeleteZone(ctx context.Context, m grpcclient.CallMeta, req *zonesv1.DeleteZoneRequest) (*zonesv1.DeleteZoneResponse, error)
}

func (h *zonesHandlers) list(c echo.Context) error {
	res, err := h.zones.ListZones(c.Request().Context(), callMeta(c), &zonesv1.ListZonesRequest{})
	if err != nil {
		return err
	}
	out := make([]zoneDTO, 0, len(res.GetZones()))
	for _, z := range res.GetZones() {
		out = append(out, toZoneDTO(z))
	}
	return c.JSON(http.StatusOK, out)
}

func (h *zonesHandlers) update(c echo.Context) error {
	var req createZoneRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.zones.UpdateZone(c.Request().Context(), callMeta(c), &zonesv1.UpdateZoneRequest{
		Id:    c.Param("id"),
		Code:  req.Code,
		Name:  req.Name,
		Ring:  toProtoRing(req.Ring),
		Holes: toProtoHoles(req.Holes),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toZoneDTO(res.GetZone()))
}

// Como en auth, los handlers solo traducen REST <-> gRPC. Las reglas y el rol se validan en Zone Service.
type zonesHandlers struct {
	zones ZonesClient
}

type coordinateDTO struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type zoneDTO struct {
	ID           string            `json:"id"`
	Code         string            `json:"code"`
	Name         string            `json:"name"`
	Ring         []coordinateDTO   `json:"ring"`
	Holes        [][]coordinateDTO `json:"holes"`
	Status       string            `json:"status"`
	StatusReason string            `json:"statusReason"`
	CreatedAt    string            `json:"createdAt"`
}

type createZoneRequest struct {
	Code  string            `json:"code"`
	Name  string            `json:"name"`
	Ring  []coordinateDTO   `json:"ring"`
	Holes [][]coordinateDTO `json:"holes"`
}

func (h *zonesHandlers) create(c echo.Context) error {
	var req createZoneRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.zones.CreateZone(c.Request().Context(), callMeta(c), &zonesv1.CreateZoneRequest{
		Code:  req.Code,
		Name:  req.Name,
		Ring:  toProtoRing(req.Ring),
		Holes: toProtoHoles(req.Holes),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toZoneDTO(res.GetZone()))
}

func (h *zonesHandlers) get(c echo.Context) error {
	res, err := h.zones.GetZone(c.Request().Context(), callMeta(c), &zonesv1.GetZoneRequest{Id: c.Param("id")})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toZoneDTO(res.GetZone()))
}

func (h *zonesHandlers) classify(c echo.Context) error {
	lat, err := strconv.ParseFloat(c.QueryParam("latitude"), 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "latitude must be a number")
	}
	lng, err := strconv.ParseFloat(c.QueryParam("longitude"), 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "longitude must be a number")
	}
	res, err := h.zones.ClassifyPoint(c.Request().Context(), callMeta(c), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: lat, Longitude: lng},
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toZoneDTO(res.GetZone()))
}

func toProtoRing(points []coordinateDTO) []*zonesv1.Coordinate {
	ring := make([]*zonesv1.Coordinate, 0, len(points))
	for _, p := range points {
		ring = append(ring, &zonesv1.Coordinate{Latitude: p.Latitude, Longitude: p.Longitude})
	}
	return ring
}

// toProtoHoles convierte los huecos del JSON. Un hueco vacío llega como anillo sin puntos y el dominio lo rechaza.
func toProtoHoles(holes [][]coordinateDTO) []*zonesv1.Ring {
	out := make([]*zonesv1.Ring, 0, len(holes))
	for _, h := range holes {
		out = append(out, &zonesv1.Ring{Points: toProtoRing(h)})
	}
	return out
}

func toZoneDTO(z *zonesv1.Zone) zoneDTO {
	ring := make([]coordinateDTO, 0, len(z.GetRing()))
	for _, p := range z.GetRing() {
		ring = append(ring, coordinateDTO{Latitude: p.GetLatitude(), Longitude: p.GetLongitude()})
	}
	holes := make([][]coordinateDTO, 0, len(z.GetHoles()))
	for _, h := range z.GetHoles() {
		hole := make([]coordinateDTO, 0, len(h.GetPoints()))
		for _, p := range h.GetPoints() {
			hole = append(hole, coordinateDTO{Latitude: p.GetLatitude(), Longitude: p.GetLongitude()})
		}
		holes = append(holes, hole)
	}
	return zoneDTO{
		ID:           z.GetId(),
		Code:         z.GetCode(),
		Name:         z.GetName(),
		Ring:         ring,
		Holes:        holes,
		Status:       z.GetStatus(),
		StatusReason: z.GetStatusReason(),
		CreatedAt:    z.GetCreatedAt(),
	}
}

type statusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type deleteRequest struct {
	Reason string `json:"reason"`
}

// setStatus activa o desactiva una zona. El motivo es obligatorio y queda en la bitácora del servicio.
func (h *zonesHandlers) setStatus(c echo.Context) error {
	var req statusRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.zones.SetZoneStatus(c.Request().Context(), callMeta(c), &zonesv1.SetZoneStatusRequest{
		Id:     c.Param("id"),
		Status: req.Status,
		Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toZoneDTO(res.GetZone()))
}

// remove borra físicamente una zona inactiva.
func (h *zonesHandlers) remove(c echo.Context) error {
	var req deleteRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if _, err := h.zones.DeleteZone(c.Request().Context(), callMeta(c), &zonesv1.DeleteZoneRequest{
		Id:     c.Param("id"),
		Reason: req.Reason,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
