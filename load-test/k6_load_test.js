import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

const deliveryRate = new Rate('delivery_success_rate');
const latencyTrend = new Trend('delivery_latency');

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '60s', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(99)<200'],
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  const createEndpointPayload = JSON.stringify({
    url: 'https://httpbin.org/post',
  });

  const epRes = http.post(`${BASE_URL}/api/v1/endpoints`, createEndpointPayload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(epRes, { 'endpoint created': (r) => r.status === 201 });

  const endpointId = epRes.json('id');

  for (let i = 0; i < 5; i++) {
    const eventPayload = JSON.stringify({
      endpoint_id: endpointId,
      payload: { event: 'load_test', ts: Date.now(), seq: i },
    });

    const evRes = http.post(`${BASE_URL}/api/v1/events`, eventPayload, {
      headers: { 'Content-Type': 'application/json' },
    });

    check(evRes, { 'event created': (r) => r.status === 201 });

    sleep(0.1);
  }

  const statsRes = http.get(`${BASE_URL}/api/v1/stats`);
  check(statsRes, { 'stats ok': (r) => r.status === 200 });
}
