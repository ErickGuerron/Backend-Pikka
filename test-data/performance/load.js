// Load test: ramp to ~20 VUs against read-only endpoints (health, ready, public catalog).
// Run: k6 run -e BASE_URL=http://localhost:8080 test-data/performance/load.js
// Note: the Gateway rate limiter is per client IP; raise RATE_LIMIT_* in the target env if 429s appear.
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  stages: [
    { duration: '30s', target: 20 },
    { duration: '1m', target: 20 },
    { duration: '15s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<800'],
  },
};

export default function () {
  const health = http.get(`${BASE_URL}/health`);
  check(health, { 'health 200': (r) => r.status === 200 });

  const ready = http.get(`${BASE_URL}/ready`);
  check(ready, { 'ready 200': (r) => r.status === 200 });

  const catalog = http.get(`${BASE_URL}/api/v1/catalog/products`);
  check(catalog, { 'catalog 200': (r) => r.status === 200 });

  sleep(1);
}
