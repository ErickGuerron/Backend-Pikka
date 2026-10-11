-- Run with order_service_user and search_path=orders,public.
-- No foreign keys or queries cross another service's schema.
CREATE TABLE orders (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    address       VARCHAR(300) NOT NULL,
    latitude      DOUBLE PRECISION NOT NULL,
    longitude     DOUBLE PRECISION NOT NULL,
    delivery_date DATE NOT NULL,
    window_start  TIME(0) WITHOUT TIME ZONE NOT NULL,
    window_end    TIME(0) WITHOUT TIME ZONE NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    review_reasons TEXT[] NOT NULL DEFAULT '{}',
    version       BIGINT NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orders_address_valid CHECK (char_length(btrim(address)) BETWEEN 5 AND 300),
    CONSTRAINT orders_latitude_valid CHECK (latitude BETWEEN -90 AND 90),
    CONSTRAINT orders_longitude_valid CHECK (longitude BETWEEN -180 AND 180),
    CONSTRAINT orders_delivery_date_finite CHECK (isfinite(delivery_date)),
    CONSTRAINT orders_window_valid CHECK (
        window_start < window_end AND window_end < TIME '24:00'
        AND EXTRACT(SECOND FROM window_start) = 0
        AND EXTRACT(SECOND FROM window_end) = 0
    ),
    CONSTRAINT orders_status_valid CHECK (status IN (
        'PENDING', 'PENDING_REVIEW', 'ROUTE_ASSIGNED', 'PICKED_UP',
        'IN_TRANSIT', 'DELIVERED', 'NOT_DELIVERED', 'CANCELLED'
    )),
    CONSTRAINT orders_review_reasons_valid CHECK (
        review_reasons <@ ARRAY['ZONE_NOT_COVERED', 'INVALID_WINDOW', 'CAPACITY_EXCEEDED', 'CUSTOMER_BLOCKED']::TEXT[]
        AND array_position(review_reasons, NULL) IS NULL
        AND ((status = 'PENDING_REVIEW' AND cardinality(review_reasons) > 0)
             OR (status <> 'PENDING_REVIEW' AND cardinality(review_reasons) = 0))
    ),
    CONSTRAINT orders_version_positive CHECK (version > 0),
    CONSTRAINT orders_timestamps_valid CHECK (
        isfinite(created_at) AND isfinite(updated_at) AND updated_at >= created_at
    )
);

-- Planning filters by delivery day and state, never by registration day.
CREATE INDEX orders_delivery_date_status_idx ON orders (delivery_date, status, id);
CREATE INDEX orders_created_at_idx ON orders (created_at DESC, id);
