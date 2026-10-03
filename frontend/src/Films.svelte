<script>
	import { onMount } from 'svelte';
	import { api, setHash } from './lib/api.js';
	import { setFilm } from './lib/playerState.js';
	import { formatSize } from './lib/format.js';

	// Свойство: в App мы передаём user
	let user;
	let films = $state([]);
	let loading = $state(true);
	let error = $state(null);

	function openFilm(film) {
		// Запоминаем фильм и переходим на страницу плеера.
		setFilm(film);
		setHash('player');
	}

	function handleDownload(id, title) {
		// Скачать
		const a = document.createElement('a');
		a.href = api.download(id);
		a.download = title || 'film';
		a.click();
	}


	onMount(async () => {
		load();
	});

	async function load() {
		loading = true;
		error = null;
		const r = await api.films();
		if (r.ok) {
			films = r.data || [];
		} else {
			error = r.error || 'Не удалось загрузить список';
			films = [];
		}
		loading = false;
	}
</script>

<section class="films">
  <h2>Фильмы ({#if films.length} {films.length}{/if})</h2>
  {#if loading}
    <p>Загрузка…</p>
  {:else if error}
    <p class="err">{error}</p>
  {:else}
    {#if films.length === 0}
      <p class="muted">Фильмов ещё нет. Проверьте MOVIE_ROOT.</p>
    {:else}
      <div class="grid">
        {#each films as film}
          <article class="film">
            <img src={api.cover(film.ID)} alt={film.Name} onerror={(e) => (e.target.style.display = 'none')} />
            <div class="film-body">
              <h3>{film.Name}</h3>
              <p class="meta">{formatSize(film.Size)}</p>
              <div class="actions">
                <button class="btn primary" onclick={() => openFilm(film)}>▶ Открыть</button>
                <button class="btn ghost" onclick={() => handleDownload(film.ID, film.Name)} >⬇ Скачать</button>
              </div>
            </div>
          </article>
        {/each}
      </div>
    {/if}
  {/if}
  <button class="btn ghost" onclick={load} >↻ Обновить</button>
</section>

<style>
.films h2 { margin-top: 0; }
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 16px;
}
.film {
  background: #101828;
  border: 1px solid #223;
  border-radius: 12px;
  overflow: hidden;
}
.film img { width: 100%; height: 140px; object-fit: cover; }
.film-body { padding: 12px; }
.film h3 { margin: 0 0 6px; font-size: 18px; }
.meta { font-size: 13px; opacity: .8; margin: 0 0 10px; }
.actions { display: flex; gap: 8px; }
.btn {
  flex: 1;
  padding: 8px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font: inherit;
  background: #2563eb;
  color: #fff;
}
.btn.ghost {
  background: transparent;
  color: #7dd3fc;
  border: 1px solid #334;
}
.err { color: #fca5a5; }
.muted { opacity: .6; }
</style>
