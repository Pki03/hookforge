import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  stages: [
    { duration: '10s', target: 100 },
    { duration: '30s', target: 100 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(99)<100'],
    http_req_failed: ['rate<0.01'],
  },
};

let endpointId = null;

export function setup() {
  const payload = JSON.stringify({
    url: 'https://httpbin.org/post',
    allowed_event_types: ['load_test'],
    rate_limit_per_second: 10000,
    rate_limit_burst: 20000,
  });

  const res = http.post(`${BASE_URL}/api/v1/endpoints`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, { 'setup endpoint created': (r) => r.status === 201 });

  return { endpointId: res.json('id') };
}

export default function (data) {
  if (!endpointId) endpointId = data.endpointId;

  const payload = JSON.stringify({
    endpoint_id: endpointId,
    event_type: 'load_test',
    payload: { event: 'load_test', ts: Date.now(), seq: __ITER },
  });

  const res = http.post(`${BASE_URL}/api/v1/events`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, { 'event ingested': (r) => r.status === 201 });
}
