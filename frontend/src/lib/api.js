// Тонкий API-клиент. Все вызовы — к /api/*.
const BASE = '/api';

async function req(method, path, body, { token } = {}) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...(body && { 'Content-Type': 'application/json' }),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  let data;
  try {
    data = await res.json();
  } catch {
    data = null;
  }
  const payload = {
    ok: res.ok,
    status: res.status,
    data,
  };
  if (!res.ok) {
    payload.error = data?.error || data?.error_message || data?.Error || res.statusText;
  }
  return payload;
}

export const api = {
  login: (username, password) => req('POST', '/auth/login', { username, password }),
  logout: () => req('POST', '/auth/logout'),
  me: () => req('GET', '/auth/me'),
  changePassword: (old, newPassword, confirm) =>
    req('PUT', '/auth/password', { old, new: newPassword, confirm }),
  films: () => req('GET', '/films'),
  // cover — отдаёт как <img src>/fetch (прокидывает заголовки)
  cover: (id) => `${BASE}/films/cover/${id}`,
  stream: (id) => `${BASE}/films/stream/${id}`,
  download: (id) => `${BASE}/films/download/${id}`,
  health: () => req('GET', '/healthz'),
  // Admin
  adminIndex: () => req('GET', '/admin'),
  adminCreate: (body) => req('POST', `/admin/users${body.path ? body.path : ''}`, body.body || body),
  adminList: () => req('GET', '/admin/users'),
  adminGet: (id) => req('GET', `/admin/users/${id}`),
  adminUpdate: (id, body) => req('PUT', `/admin/users/${id}`, body),
  adminDelete: (id) => req('DELETE', `/admin/users/${id}`),
  adminRole: (id, role) => req('PUT', `/admin/users/${id}/role`, { role }),
  adminResetPassword: (id, password) =>
    req('POST', `/admin/users/${id}/reset-password`, { password }),
  adminSettings: () => req('GET', '/admin/settings'),
  adminSetSetting: (key, value) =>
    req('PUT', `/admin/settings/${key}`, { value }),
};

// Простой hash-роутер: #/login #/films #/admin
export const hashRoute = () =>
  location.hash.replace('#', '') || 'films';

export const setHash = (r) => {
  if (location.hash !== `#${r}`) {
    history.pushState(null, '', `#${r}`);
    window.dispatchEvent(new PopStateEvent('popstate'));
  }
};
