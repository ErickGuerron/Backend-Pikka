# ADR 0004: Seguimiento del cliente con enlace protegido

Estado: reemplazada por el ADR 0007 (el seguimiento del cliente pasa a la app móvil con cuenta)

## Contexto

El enunciado del proyecto deja el seguimiento para el cliente como opcional:
el estado de la entrega debe ser visible para el negocio y, opcionalmente,
para el cliente. El personal del negocio registra el pedido en nombre del
cliente, y el sistema arma las rutas y las asigna a repartidores disponibles.

Se evaluaron dos formas de que el cliente consulte su pedido:

- **Cuenta con contraseña y rol `CUSTOMER`.** Exige alta de usuarios, gestión
  de contraseñas (alta, recuperación, cambio), un rol nuevo en Auth y reglas de
  pertenencia en cada servicio. Es más trabajo del que pide el enunciado.
- **Enlace protegido por pedido.** El operador entrega al cliente una URL con un
  token opaco. Quien tiene el enlace consulta el estado de ese pedido, sin
  cuenta ni contraseña.

Se elige el enlace protegido. Es suficiente para el alcance del enunciado y
mantiene el sistema más pequeño.

## Decisión

1. **Sin rol `CUSTOMER` ni cuentas de cliente.** Auth Service no cambia. Los
   roles siguen siendo `ADMIN`, `OPERATOR` y `DRIVER`.
2. **Generación del enlace.** Al registrar el pedido, Order Service genera un
   token aleatorio de al menos 128 bits, sin relación con el ID del pedido. El
   enlace tiene la forma `https://<dominio>/seguimiento/<token>`.
3. **Almacenamiento.** La base guarda solo el hash SHA-256 del token, nunca el
   token en claro. Así una fuga de la base no permite usar los enlaces.
4. **Alcance.** Cada token da acceso de solo lectura a un único pedido. No da
   acceso a otros pedidos del mismo cliente ni a ninguna otra parte del
   sistema.
5. **Vigencia.** El enlace caduca al cumplirse la fecha de entrega más un plazo
   configurable (por defecto 7 días). El operador puede revocarlo antes desde
   la web.
6. **Endpoint público.** `GET /api/v1/tracking/{token}` es la única ruta del
   Gateway que no exige JWT, además de `/health`, `/ready` y la documentación.
   Tracking Service valida el token contra su hash, comprueba vigencia y
   revocación, y responde solo con los datos de la sección 7.
7. **Datos que devuelve.** Estado del pedido, fecha y ventana, dirección de
   entrega, historial de cambios de estado con fecha y hora, posición de la
   parada en la ruta (número de paradas previas), y estado del repartidor
   (`ON_ROUTE`, etc.). No devuelve nombre ni teléfono del repartidor, datos de
   otros pedidos ni otras paradas.
8. **Protección contra abuso.** Límite de peticiones por IP y por token. Las
   respuestas a tokens inválidos, vencidos o revocados son idénticas (`404`),
   para no revelar cuáles existen.
9. **Manejo del enlace en el navegador.** La página de seguimiento responde con
   `Referrer-Policy: no-referrer` y `X-Robots-Tag: noindex`, para que el token no
   se filtre por cabeceras de referencia ni quede indexado.
10. **Actualización en vivo.** La página consulta por REST. Si después se agrega
    WebSocket, el canal también valida el token.

## Razón de negocio: el operador como intermediario

El pedido no entra directamente desde el cliente al sistema porque el negocio
necesita validar y decidir antes de comprometer una entrega:

- **Cobertura.** Confirmar que la dirección cae en una zona atendida
  (`ClassifyPoint`). Una dirección fuera de zona no debe entrar al armado de
  rutas.
- **Capacidad y fecha.** Un pedido puede llegar días antes, pero la entrega
  depende de la capacidad de la fecha (`MAX_ORDERS_PER_ROUTE`). El operador
  confirma la fecha y la ventana reales antes de que el pedido sea planificable.
- **Calidad de los datos.** Las direcciones escritas por clientes suelen estar
  incompletas. El operador las corrige y evita rutas inválidas.
- **Excepciones.** Pedidos especiales, cambios de último momento y ajustes
  manuales (sección 3.2) requieren criterio humano.
- **Atención al cliente.** Quien recibe la llamada o el mensaje de un cliente
  es el mismo que registra el pedido, así que hay un solo responsable.

Este modelo es el mismo que usan negocios con entregas programadas, como las
florerías que reciben pedidos por teléfono o mensajería y los registran en
tienda, o los servicios de paquetería que dan de alta la guía en mostrador. En
plataformas de comida a domicilio el cliente sí pide directamente, pero el
comercio confirma y prepara el pedido antes de que el repartidor lo recoja;
el comercio cumple el papel de intermediario en ese paso.

## Consecuencias

- Order Service guarda el hash del token y su fecha de caducidad, y expone la
  generación y revocación del enlace al operador.
- Tracking Service implementa la consulta pública y consume datos de Order y
  Routing mediante gRPC.
- El Gateway agrega una única excepción de autenticación para
  `/api/v1/tracking/{token}`. Debe quedar cubierta por pruebas y por un escaneo
  de OWASP ZAP.
- Pruebas obligatorias: token inválido, vencido, revocado y de otro pedido dan la
  misma respuesta; la respuesta no incluye datos de otros pedidos ni datos
  personales del repartidor; el token no aparece en logs.
- El frontend web agrega una vista pública de seguimiento, separada de la
  consola del operador, y la acción de generar y revocar el enlace en el
  detalle del pedido.
- Riesgo aceptado: quien reenvíe el enlace le da acceso a ese pedido hasta que
  caduque o se revoque. Se mitiga con vigencia corta y revocación.
