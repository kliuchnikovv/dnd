import { apiBaseUrl } from '../config';

const ENV = process.env.EXPO_PUBLIC_API_URL;
afterEach(() => {
  if (ENV === undefined) delete process.env.EXPO_PUBLIC_API_URL;
  else process.env.EXPO_PUBLIC_API_URL = ENV;
});

test('без схемы подставляет https (голый host → не file:///)', () => {
  process.env.EXPO_PUBLIC_API_URL = 'dnd-production-cdfe.up.railway.app';
  expect(apiBaseUrl()).toBe('https://dnd-production-cdfe.up.railway.app');
});

test('явную схему сохраняет и срезает хвостовые слэши', () => {
  process.env.EXPO_PUBLIC_API_URL = 'http://localhost:8080/';
  expect(apiBaseUrl()).toBe('http://localhost:8080');
});

test('пустое значение → локальный дефолт', () => {
  delete process.env.EXPO_PUBLIC_API_URL;
  expect(apiBaseUrl()).toBe('http://localhost:8080');
});
