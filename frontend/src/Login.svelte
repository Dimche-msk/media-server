<script>
	import { api, setHash } from './lib/api.js';
	let { doLogin } = $props();
	let username = $state('');
	let password = $state('');
	let busy = $state(false);
	let message = $state(null);
	let showAdmin = $state(false);

	async function submit() {
		if (!username || !password) {
			message = { ok: false, text: 'Заполните имя и пароль' };
			return;
		}
		busy = true;
		message = null;
		const r = await doLogin(username, password);
		busy = false;
		if (r.ok) {
			setHash('films');
		} else {
			message = { ok: false, text: r.error || 'Неверные логин или пароль' };
		}
	}
</script>

<div class="login-card">
  <h2>Вход</h2>
  {#if message}
    <p class="{message.ok ? 'ok' : 'err'}">{message.text}</p>
  {/if}

  <label>
    <input type="text" bind:value={username} placeholder="Имя пользователя" autocomplete="username" />
  </label>
  <label>
    <input type="password" bind:value={password} placeholder="Пароль" autocomplete="current-password" />
  </label>
  {#if showAdmin}
    <label>
      <input type="password" bind:value={password} placeholder="Пароль админа (дефолт: changeme)" autocomplete="off" />
    </label>
  {/if}
  <button type="button" class="btn primary" disabled={busy} onclick={submit}>
    {busy ? 'Подождите…' : 'Войти'}
  </button>
  <button type="button" class="btn ghost" onclick={() => (showAdmin = !showAdmin)}>
    Показать пароль дефолтного админа (changeme)
  </button>
</div>

<style>
.login-card {
  max-width: 400px;
  margin: 40px auto;
  padding: 24px;
  background: #101828;
  border-radius: 12px;
  border: 1px solid #223;
}
.login-card h2 { margin-top: 0; }
label {
  display: block;
  margin: 12px 0;
  opacity: .9;
}
input {
  width: 100%;
  padding: 10px;
  border-radius: 8px;
  border: 1px solid #334;
  background: #0b0f1a;
  color: inherit;
  font: inherit;
  box-sizing: border-box;
}
.btn {
  margin-top: 8px;
  padding: 10px 16px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font: inherit;
}
.btn.primary { background: #2563eb; color: #fff; }
.btn.primary:disabled { opacity: .6; }
.btn.ghost {
  background: transparent;
  color: #7dd3fc;
  border: 1px solid #334;
  width: 100%;
}
.ok { color: #86efac; }
.err { color: #fca5a5; }
</style>
