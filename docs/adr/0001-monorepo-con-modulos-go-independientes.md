# ADR 0001: Repositorio de backend con un módulo Go por servicio

Estado: aceptada (Fase 1)

## Contexto

La base técnica recomienda un monorepo (sección 36) y servicios desplegables
de forma independiente (sección 7.1), cada uno con su `go.mod` (sección 20).
Se decidió separar el frontend en su propio repositorio (Frontend-Pikka); este
repositorio (Backend-Pikka) contiene todo el backend y los contratos.

## Decisión

- Cada servicio en `services/<nombre>` es un módulo Go propio.
- Los contratos gRPC viven en el módulo `github.com/ErickGuerron/Backend-Pikka/contracts`; el código generado
  se versiona en `contracts/gen/go` para que compilar un servicio no requiera
  `buf`. CI verifica que el código generado esté al día y corre `buf breaking`
  en cada PR.
- Los servicios referencian `github.com/ErickGuerron/Backend-Pikka/contracts` con `replace => ../../contracts`
  y `go.work` une todo para el IDE.
- Las rutas de módulo usan el prefijo `github.com/ErickGuerron/Backend-Pikka/`, que coincide con el repositorio.
- No hay librería compartida entre servicios: lo poco que se repite (logger,
  carga de configuración) se duplica a propósito para no acoplar despliegues.

## Consecuencias

Las imágenes Docker se construyen desde la raíz del repositorio para copiar
`contracts/`. Un cambio en un `.proto` obliga a regenerar y a revisar el impacto
en los servicios que lo usan dentro del mismo PR.
