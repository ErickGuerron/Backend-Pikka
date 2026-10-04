# ADR 0003: Un esquema y un usuario de PostgreSQL por servicio

Estado: aceptada (Fase 1)

## Decisión

`infrastructure/postgres/init/01-schemas-and-users.sh` crea, al inicializar el
volumen, los esquemas `auth`, `orders`, `zones`, `routing`, `drivers` y
`tracking`, y un usuario por servicio dueño solo de su esquema. Se revoca
`CREATE` en `public` y el acceso por defecto a los demás esquemas. PostGIS se
instala en `public` para que todos los servicios usen sus tipos y funciones.

Cada servicio aplica sus migraciones al arrancar con `golang-migrate`; la tabla
`schema_migrations` queda dentro de su propio esquema.

## Consecuencias

Un servicio que intente leer tablas de otro falla con `permission denied`, así
la regla 1 de la sección 49 se cumple también a nivel de base de datos.
