# ADR 0009: Catálogo de productos con stock simple

Estado: aceptada

Enmienda al ADR 0007, sección "Pedidos sin SKU" (en la lista de lo que el MVP no incluye).
El resto del ADR 0007 sigue vigente: app móvil única, validación automática de pedidos y
cola de excepciones del operador. El ADR 0006 sigue vigente para el cobro simulado.

## Contexto

El ADR 0007 dejó fuera del MVP un catálogo de productos porque el servicio se describía solo como
entregas. El negocio pide que el cliente pueda pedir productos concretos desde la landing y la app.

Esto no convierte Pikka en un sistema de inventario. El punto central sigue siendo la entrega; el
producto es lo que se entrega. Por eso el control de stock es deliberadamente simple.

## Decisión

### 1. Catálogo en el Order Service

Los productos viven en el esquema `orders`, en el mismo servicio que los pedidos. No hay un servicio nuevo.

Un producto tiene:

- `id`, `name`, `description`;
- `unit_price`, en centavos de dólar (USD). El precio actual vive aquí; el pedido guarda su copia;
- `active`: un producto inactivo deja de aparecer para el cliente, pero los pedidos anteriores conservan sus datos;
- `stock_quantity`: entero no negativo, o nulo. Nulo significa "sin control de stock" (por ejemplo, servicios).

El catálogo lo administra el operador (`ADMIN` y `OPERATOR`). El cliente solo lee los productos activos.

### 2. Stock con historial de movimientos

- El stock es **global**, no por día ni por zona. Por eso no hay reservas por lote, bodegas ni compras.
- Cada cambio de stock queda registrado como un **movimiento**: tipo, cantidad con signo, stock resultante,
  motivo, usuario, fecha y, si aplica, el pedido. El stock actual es el último resultado del historial.
  Los movimientos no se editan ni se borran.

Tipos de movimiento:

- `RESTOCK`: entrada de mercadería. Al crear un producto, su stock inicial es una entrada.
- `ADJUSTMENT`: corrección manual, positiva o negativa, con motivo obligatorio (por ejemplo, producto
  dañado o conteo físico). No puede dejar el stock en negativo.
- `SALE`: al crear un pedido, cada ítem descuenta su cantidad. El descuento es condicional
  (`UPDATE ... WHERE stock_quantity >= cantidad`) en la misma transacción que el pedido, así dos pedidos
  simultáneos no venden más de lo que hay.
- `RETURN`: al cancelar un pedido en estado `PENDING` o `ROUTE_ASSIGNED` (ADR 0007), el stock se devuelve.

Un pedido que pasa a `DELIVERED`, `NOT_DELIVERED` o `CANCELLED` por otra causa no devuelve stock.

Ejemplo: 4 manzanas; un pedido de 2 deja 2 (`SALE −2`); una entrada de 1 deja 3 (`RESTOCK +1`).

- Si no hay stock suficiente, el pedido se rechaza con un error claro. No entra en la cola de excepciones:
  la cola es para validaciones de logística, no de disponibilidad.

### 3. Pedido con ítems

- Un pedido nuevo tiene al menos un ítem. Cada ítem guarda `product_id`, el nombre y el precio unitario
  **en el momento del pedido**, la cantidad y el subtotal.
- El total del pedido es la suma de los subtotales y se usa para el cobro simulado del ADR 0006.
- Los pedidos creados antes de este ADR no tienen ítems y siguen válidos.
- Cantidad por ítem: entre 1 y 50. Es un límite de sentido común, no de inventario.

### 4. Lo que queda fuera

- Inventario multi-bodega, reposición automática, proveedores y órdenes de compra.
- Reportes de inventario y costos del producto.
- Variantes (talla, color), categorías jerárquicas, imágenes y promociones.
- Stock por día o por zona. Si se necesita, requiere un ADR nuevo.

## Consecuencias

- El contrato `orders.v1` agrega productos y un campo de ítems al crear un pedido. Es un cambio aditivo, pero
  el pedido actual sin ítems queda como caso válido para pedidos anteriores.
- El Gateway expone rutas de catálogo para el operador y de lectura para el cliente.
- La landing puede mostrar productos reales. Los textos de servicio actuales siguen válidos como sección aparte.
- Web y app del cliente necesitan selección de productos y cantidades antes de la ubicación.
- Las validaciones de zona, fecha, ventana y capacidad del ADR 0007 se aplican igual al pedido completo.

## Decisiones tomadas por defecto, a confirmar

- Moneda: USD, porque es la moneda de Ecuador.
- Stock global y no por día: es lo más simple. Cambiarlo es un ADR nuevo.
- Cantidad máxima por ítem: 50.
