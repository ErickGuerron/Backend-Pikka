// Smoke test de rendimiento: verifica que el Gateway responde bajo una carga
// mínima. Se ejecuta con `make perf-smoke` (contenedor k6 del perfil "perf").
//
// Las pruebas de carga reales (picos, rutas, asignación concurrente) se agregan
// en la Fase 7 con datasets de test-data/seeds.
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  vus: 5,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const health = http.get(`${BASE_URL}/health`);
  check(health, { 'health responde 200': (r) => r.status === 200 });

  const ready = http.get(`${BASE_URL}/ready`);
  check(ready, { 'ready responde 200': (r) => r.status === 200 });

  sleep(1);
}
