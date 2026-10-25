// Stress test: progressive stages 10 -> 50 -> 100 -> 200 VUs, then recovery. Read-only endpoints only.
// Run: k6 run -e BASE_URL=http://localhost:8080 test-data/performance/stress.js
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '1m', target: 50 },
    { duration: '1m', target: 100 },
    { duration: '1m', target: 200 },
    { duration: '1m', target: 10 }, // recovery
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1500'],
  },
};

export default function () {
  const health = http.get(`${BASE_URL}/health`);
  check(health, { 'health 200': (r) => r.status === 200 });

  const catalog = http.get(`${BASE_URL}/api/v1/catalog/products`);
  check(catalog, { 'catalog 200': (r) => r.status === 200 });

  sleep(0.5);
}
