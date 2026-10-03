<script>
	import { onMount } from 'svelte';
	import { api } from './lib/api.js';

	let users = $state([]);
	let settings = $state({});
	let loading = $state(true);
	let error = $state(null);
	let showForm = $state(false);

	// Форма создания пользователя
	let formUsername = $state('');
	let formPassword = $state('');
	let formRole = $state('user');

	// Форма смена роли
	let roleTargetRole = $state('user');

	// Форма смена пароля
	let resetUserId = $state(0);
	let resetPw = $state('');

	async function load() {
		loading = true;
		error = null;
		const uRes = await api.adminList();
		const sRes = await api.adminSettings();
		if (uRes.ok) {
			users = uRes.data || [];
		} else {
			error = 'users: ' + (uRes.error || uRes.error_message || 'unknown');
		}
		if (sRes.ok) {
			settings = sRes.data || {};
		} else {
			error = error + ' settings: ' + (sRes.error || sRes.error_message || 'unknown');
		}
		loading = false;
	}

	onMount(() => load());

	async function createUser() {
		if (!formUsername || !formPassword) return alert('Заполните поля');
		const r = await api.adminCreate({
			username: formUsername,
			password: formPassword,
			role: formRole,
		});
		if (r.ok) {
			formUsername = '';
			formPassword = '';
			load();
		} else alert(r.error || 'create error');
	}

	async function changeRole(id, role) {
		const r = await api.adminRole(id, role);
		if (r.ok) load();
		else alert(r.error || 'role error');
	}

	async function applyResetPw(id, password) {
		const r = await api.adminResetPassword(id, password);
		if (r.ok) {
			load();
		} else alert(r.error || 'reset error');
	}

	async function deleteUser(id) {
		if (!confirm('Удалить пользователя ' + id + '?')) return;
		const r = await api.adminDelete(id);
		if (r.ok) load();
		else alert(r.error || 'delete error');
	}
</script>

<section class="admin">
	<h2>Админ</h2>

	{#if loading}
		<p>Загрузка…</p>
	{:else}
		{#if error}
			<p class="err">{error}</p>
			<button class="btn" onclick={load}>Повторить</button>
		{:else}

			<!-- Пользователи -->
			<h3>Пользователи</h3>
			<button class="btn ghost" onclick={() => (showForm = !showForm)}>
				{showForm ? 'Скрыть форму' : '+ Добавить'}
			</button>
			{#if showForm}
				<div class="form">
					<input placeholder="username" bind:value={formUsername} />
					<input type="password" placeholder="password" bind:value={formPassword} />
					<select bind:value={formRole}>
						<option value="user">user</option>
						<option value="admin">admin</option>
					</select>
					<button class="btn primary" onclick={createUser}>Создать</button>
				</div>
			{/if}

			<table class="table">
				<thead>
					<tr>
						<th>id</th><th>username</th><th>role</th><th></th>
					</tr>
				</thead>
				<tbody>
					{#each users as u (u.id)}
						<tr>
							<td>{u.id}</td>
							<td>{u.username}</td>
							<td>
								<select
									bind:value={roleTargetRole}
									 onchange={() => changeRole(u.id, roleTargetRole)}
								>
									<option value="user">user</option>
									<option value="admin">admin</option>
								</select>
							</td>
							<td>
								<button class="btn ghost" onclick={() => (resetUserId = u.id)}>🔄</button>
								<button class="btn ghost" onclick={() => deleteUser(u.id)}>✕</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>

			<!-- Сброс пароля -->
			{#if resetUserId > 0}
				<div class="form">
					<input type="password" placeholder="новый пароль" bind:value={resetPw} />
					<button class="btn primary" onclick={() => applyResetPw(resetUserId, resetPw)} >Применить</button>
				</div>
			{/if}

			<!-- Настройки -->
			<h3>Настройки</h3>
			<table class="table">
				<tbody>
					{#each Object.entries(settings) as [key, value]}
						<tr>
							<td>{key}</td>
							<td>{value}</td>
						</tr>
					{/each}
				</tbody>
			</table>

		{/if}
	{/if}
</section>

<style>
.admin h2 { margin-top: 0; }
.table { width: 100%; border-collapse: collapse; margin: 10px 0; }
.table th, .table td { padding: 8px; border: 1px solid #334; text-align: left; }
.form { display: flex; gap: 8px; margin: 10px 0; flex-wrap: wrap; }
.form input, .form select { padding: 8px; border-radius: 8px; border: 1px solid #334; background: #0b0f1a; color: inherit; }
.btn { padding: 8px 14px; border: none; border-radius: 8px; cursor: pointer; background: #2563eb; color: #fff; }
.btn.ghost { background: transparent; color: #7dd3fc; border: 1px solid #334; }
.err { color: #fca5a5; }
</style>
