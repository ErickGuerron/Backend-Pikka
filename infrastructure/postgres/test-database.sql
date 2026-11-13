-- Run from the postgres maintenance database as the cluster administrator.
-- The application database is never used as a template.
SELECT pg_advisory_lock(hashtext('pikka:last_mile_test:provision'));
SELECT 'CREATE DATABASE last_mile_test TEMPLATE template0'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'last_mile_test') \gexec
SELECT pg_advisory_unlock(hashtext('pikka:last_mile_test:provision'));

\connect last_mile_test

BEGIN;
SELECT pg_advisory_xact_lock(hashtext('pikka:last_mile_test:provision'));
CREATE EXTENSION IF NOT EXISTS postgis;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON DATABASE last_mile_test FROM PUBLIC;
GRANT CONNECT, CREATE ON DATABASE last_mile_test
    TO auth_service_user, order_service_user, driver_service_user, zone_service_user;
CREATE SCHEMA IF NOT EXISTS auth AUTHORIZATION auth_service_user;
CREATE SCHEMA IF NOT EXISTS orders AUTHORIZATION order_service_user;
CREATE SCHEMA IF NOT EXISTS drivers AUTHORIZATION driver_service_user;
CREATE SCHEMA IF NOT EXISTS zones AUTHORIZATION zone_service_user;
COMMIT;
