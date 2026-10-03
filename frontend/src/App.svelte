<script>
	import { onMount } from 'svelte';
	import { api, hashRoute, setHash } from './lib/api.js';
	import Login from './Login.svelte';
	import Films from './Films.svelte';
	import Admin from './Admin.svelte';
	import Player from './Player.svelte';

	let route = $state(hashRoute());
	let user = $state(null);

	$effect(() => {
		window.addEventListener('popstate', onPop);
		return () => window.removeEventListener('popstate', onPop);
	});

	function onPop() {
		route = hashRoute();
	}

	async function checkAuth() {
		const r = await api.me();
		if (r.ok) {
			user = r.data;
		} else {
			user = null;
		}
	}

	onMount(async () => {
		checkAuth();
	});

	async function doLogin(username, password) {
		const r = await api.login(username, password);
		if (r.ok) {
			user = r.data;
		}
	}

	async function doLogout() {
		await api.logout();
		user = null;
		setHash('login');
	}

	$effect(() => {
		if (route === 'admin' && !user) setHash('login');
	});
</script>

<header class="top">
  <div class="brand">Media Server</div>
  {#if user}
    <div class="user">
      <span>Привет, {user.username}</span>
      {#if user.role === 'admin'}<a href="#/admin" onclick={() => setHash('admin')} class="nav-link">Админ</a>{/if}
      <a href="#" onclick={doLogout} class="nav-link">Выйти</a>
    </div>
  {/if}
</header>

<main>
  {#if user}
  {#if route === 'player'}
  	<Player {user} />
  {:else if route === 'admin' && user.role === 'admin'}
  	<Admin {user} />
  {:else}
  	<Films {user} />
  {/if}
  {:else}
  	<Login {user} {doLogin} />
  {/if}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family: system-ui, -apple-system, sans-serif;
		background: #0b0f1a;
		color: #e2e8f0;
	}
	.top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 12px 20px;
		background: #101828;
		border-bottom: 1px solid #223;
	}
	.brand {
		font-size: 20px;
		font-weight: 700;
	}
	.user {
		display: flex;
		gap: 16px;
		align-items: center;
	}
	.user span { opacity: .9; }
	.nav-link {
		color: #7dd3fc;
		text-decoration: none;
	}
	main {
		padding: 20px;
		max-width: 1200px;
		margin: 0 auto;
	}
</style>
