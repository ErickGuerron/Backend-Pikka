# Sprint 1 de Anthony

Las dos tareas asignadas a Anthony están implementadas en ramas de funcionalidad.
Las PR están en borrador; la revisión por otro integrante y la integración
en `develop` quedan como pasos de cierre del equipo. Esta evidencia corresponde
a la implementación y validación local del 11 de octubre de 2026.

| Tarea | Rama | Entrega |
|---|---|---|
| S1-BE-01 | `feature/s1-be-01-anthony-orders-contract` | [PR 17](https://github.com/ErickGuerron/Backend-Pikka/pull/17): Protobuf orders.v1, Go generado, OpenAPI y documentación del contrato |
| S1-BE-02 | `feature/s1-be-02-anthony-order-domain` | [PR 18](https://github.com/ErickGuerron/Backend-Pikka/pull/18): dominio, fechas, estados, migraciones, pruebas e inclusión en CI |

## Gitflow

`main` mantiene la base estable y `develop` la integración. Las ramas siguen el
prefijo `feature/`. El contrato parte de `origin/develop`; la implementación parte
del commit del contrato para permitir validar el conjunto antes de integrarlo.

1. Revisar el contrato en una PR hacia `develop`.
2. Integrar primero esa PR, una vez aprobada por otro integrante y por CI.
3. Revisar el dominio inicialmente contra la rama de contrato para mostrar solo
   S1-BE-02. Después de integrar el contrato, cambiar la base de esa PR a `develop`
   y actualizar la rama antes de integrarla.
4. Integrar el dominio en `develop` tras su aprobación. La publicación hacia
   `main` se realiza después mediante el flujo de release del equipo.

La dependencia entre ramas no implica que el contrato ya esté mergeado. No se
mezclan tareas de Erick, Xabier o Mabe ni la integración gRPC del Sprint 2.
El repositorio frontend queda en `develop`; no hay tareas de Anthony asignadas
allí en el Sprint 1. Se verificó la compatibilidad de sus nombres de campos y
estados con el OpenAPI conservado.

## Evidencia local

| Comprobación | Resultado |
|---|---|
| `buf lint` | Aprobado |
| `buf breaking` contra `origin/develop` | Aprobado |
| Generación Go con plugins de CI | Compila; sin cambios en los contratos existentes |
| OpenAPI con Redocly 1.34.0 | Aprobado |
| Pruebas unitarias de Order Service | Aprobadas; 100 % de cobertura del dominio |
| Pruebas PostgreSQL 16 y PostGIS 3.4 | Aprobadas; migración aplicada dos veces y down/up reversible |
| `go vet` de Order Service | Aprobado |
| golangci-lint 2.14.0 de Order Service, incluido build tag integration | Aprobado, 0 incidencias |
| Pruebas `-race` y `go vet` de todos los módulos | Aprobadas en Linux con Go 1.26.9 |
| Order Service como módulo independiente (`GOWORK=off`) | Pruebas unitarias y de integración aprobadas |
| `govulncheck` de Order Service con Go 1.26.9, con y sin workspace | Sin vulnerabilidades detectadas |

Las pruebas de dominio incluyen fechas imposibles, año bisiesto, fecha futura,
entrega del mismo día con desfase UTC, ventanas invertidas o que cruzan medianoche,
Unicode, coordenadas no finitas, los límites geográficos y la matriz de transiciones.
Los estados finales no admiten cambios y la cancelación ordinaria se detiene al
recoger el pedido.

Las pruebas de base usan `order_service_user`, verifican que no puede acceder a
`auth` ni `zones` y comprueban restricciones sobre estado, revisión, coordenadas,
fecha finita, ventanas y versión. Todas las filas de prueba se revierten.

La cobertura unitaria del módulo completo es 81 %; el 100 % se refiere al dominio.
El aplicador de migraciones también se verifica con las pruebas de integración.
El primer análisis de vulnerabilidades con el Go 1.26.5 instalado detectó alertas
de la biblioteca estándar. Se agregó `toolchain go1.26.9` al workspace y al nuevo
módulo para seleccionar el parche corregido. Referencia:
[GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607).

Se añadieron commits de corrección para rechazar NUL en direcciones antes de
persistir y limitar las fechas SQL a los años 0001–9999 mediante una segunda
migración reversible. CI valida además Order Service con `GOWORK=off`, para
detectar dependencias ausentes en el módulo independiente.

La prueba independiente detectó además dos alertas de `golang.org/x/text` 0.38,
ocultas por la versión más reciente resuelta en el workspace. Se fijó 0.42 y
se volvió a ejecutar el análisis de seguridad en modo independiente. Referencia:
[GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629).

Los escaneos OWASP y k6 del cierre global del sprint son actividades distintas
de estas dos tareas y no se presentan como ejecutados en esta entrega.
