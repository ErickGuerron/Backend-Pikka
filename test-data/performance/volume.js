// Volume test: many iterations of the public catalog listing. Read-only; setup() only checks reachability.
// Run: k6 run -e BASE_URL=http://localhost:8080 -e ITERATIONS=20000 test-data/performance/volume.js
import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const ITERATIONS = parseInt(__ENV.ITERATIONS || '10000', 10);

export const options = {
  scenarios: {
    volume: {
      executor: 'shared-iterations',
      vus: 20,
      iterations: ITERATIONS,
      maxDuration: '15m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000'],
  },
};

export function setup() {
  // Non-destructive: only verifies the target is reachable before generating volume.
  const res = http.get(`${BASE_URL}/health`);
  if (res.status !== 200) {
    throw new Error(`target not healthy: ${res.status}`);
  }
}

export default function () {
  const res = http.get(`${BASE_URL}/api/v1/catalog/products`);
  check(res, { 'catalog 200': (r) => r.status === 200 });
}
