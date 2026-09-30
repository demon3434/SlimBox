import { bus } from './bus.js';

export const AUTH_TOKEN_KEY = 'slimbox_auth_token';

/**
 * Retrieve the saved authentication token from localStorage.
 * @returns {string}
 */
export function getAuthToken() {
  return localStorage.getItem(AUTH_TOKEN_KEY) || '';
}

/**
 * Persist or clear the authentication token in localStorage.
 * @param {string|null} token
 */
export function setAuthToken(token) {
  if (token) {
    localStorage.setItem(AUTH_TOKEN_KEY, token.trim());
  } else {
    localStorage.removeItem(AUTH_TOKEN_KEY);
  }
}

let isAuthPromptActive = false;

bus.on('auth:unlocked', () => {
  isAuthPromptActive = false;
});

function triggerAuthRequired() {
  if (isAuthPromptActive) return;
  isAuthPromptActive = true;
  bus.emit('auth:required');
}

/**
 * Enhanced fetch wrapper with Authorization Bearer header injection and 401 interception.
 * @param {RequestInfo|string} input
 * @param {RequestInit} [init={}]
 * @returns {Promise<Response>}
 */
export async function apiFetch(input, init = {}) {
  const options = { ...init };
  options.headers = options.headers || {};

  const token = getAuthToken();
  const url = typeof input === 'string' ? input : (input.url || '');
  const isInternalApi = url.startsWith('/api/') || (url.startsWith('http') && url.includes(window.location.host + '/api/'));

  if (token && isInternalApi) {
    if (options.headers instanceof Headers) {
      if (!options.headers.has('Authorization')) {
        options.headers.set('Authorization', `Bearer ${token}`);
      }
    } else if (Array.isArray(options.headers)) {
      options.headers.push(['Authorization', `Bearer ${token}`]);
    } else {
      if (!options.headers['Authorization']) {
        options.headers['Authorization'] = `Bearer ${token}`;
      }
    }
  }

  const response = await fetch(input, options);

  if (
    response.status === 401 &&
    !url.includes('/api/v1/auth/status') &&
    !url.includes('/api/v1/auth/login') &&
    !url.includes('/api/v1/auth/setup') &&
    !url.includes('/api/v1/auth/verify')
  ) {
    triggerAuthRequired();
  }

  return response;
}

