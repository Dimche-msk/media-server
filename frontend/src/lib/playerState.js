// Держит фильм, который открыли «Открыть» из списка, до рендера плеера.
// Отдельный модуль — не реативный state, просто carrier для передачи данных
// из Films.svelte в Player.svelte без лишних пропов/хэша.
let current = null;

export function setFilm(film) {
  current = film;
}

export function getFilm() {
  return current;
}
