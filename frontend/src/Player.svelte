<script>
	import { onMount } from 'svelte';
	import { api } from './lib/api.js';
	import { setHash } from './lib/api.js';
	import { getFilm, setFilm } from './lib/playerState.js';
	import { formatSize } from './lib/format.js';

	// Свойство: в App мы передаём user
	let user;

	// Фильм, который открыли из списка; ставится до рендера страницы.
	let film = $state(getFilm());
	let playing = $state(false);
	let fallback = $state(false); // true — браузер не смог воспроизвести, показываем запасной вариант

	function openDirect() {
		window.open(api.stream(film.ID), '_blank');
	}

	function download() {
		const a = document.createElement('a');
		a.href = api.download(film.ID);
		a.download = film.Name || 'film';
		a.click();
	}

	onMount(() => {
		const f = getFilm();
		if (f) film = f;
	});

	function onVideoError() {
		// Браузер не смог распарсить/декодировать контейнер → показываем fallback.
		fallback = true;
	}
</script>

<div class="player-wrap">
  {#if film}
    <div class="player-bar">
      <h2>{film.Name}</h2>
      {#if film.Size}<span class="muted">{formatSize(film.Size)}</span>{/if}
      <a href="#" class="back" onclick={() => setHash('films')}>&larr; К списку</a>
    </div>

    {#if fallback}
      <div class="fallback">
        <p class="err">
          Браузер не может воспроизвести этот файл.
          Вероятно, он не поддерживает кодек (MKV/H.265 и т.п.).
        </p>
        <div class="actions">
          <a class="btn primary" href={api.download(film.ID)} target="_blank">Скачать файл</a>
          <a class="btn ghost" href={api.stream(film.ID)} target="_blank">Открыть напрямую</a>
        </div>
      </div>
    {:else}
      <video
        src={api.stream(film.ID)}
        controls
        preload="metadata"
        playsinline
        onclick={onVideoError}
      />
    {/if}
  {:else}
    <p class="muted">Фильм не найден.</p>
  {/if}
</div>

<style>
.player-wrap {
  max-width: 1100px;
  margin: 0 auto;
  width: 100%;
}
.player-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.player-bar h2 {
  margin: 0;
  font-size: 20px;
}
.player-bar .muted {
  opacity: .6;
  font-size: 14px;
}
.back {
  margin-left: auto;
  color: #7dd3fc;
  text-decoration: none;
  font-size: 14px;
}
video {
  width: 100%;
  max-width: 1200px;
  background: #000;
  border-radius: 10px;
}
.fallback {
  padding: 24px;
  background: #101828;
  border: 1px solid #223;
  border-radius: 10px;
  color: #e2e8f0;
}
.fallback .actions {
  display: flex;
  gap: 12px;
  margin-top: 16px;
}
.btn {
  display: inline-block;
  padding: 10px 18px;
  border-radius: 8px;
  text-decoration: none;
  color: inherit;
}
.btn.primary {
  background: #2563eb;
  color: #fff;
}
.btn.ghost {
  background: transparent;
  color: #7dd3fc;
  border: 1px solid #334;
}
.err {
  color: #fca5a5;
}
</style>
