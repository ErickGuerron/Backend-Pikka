-- Lookup route -> reserved driver (GetDriverByRouteID). Partial: only reserved drivers have a route_id.
CREATE INDEX drivers_route_id_idx ON drivers (route_id) WHERE route_id <> '';
