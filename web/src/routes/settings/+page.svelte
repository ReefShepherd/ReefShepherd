<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ThemePicker from '#lib/components/forms/ThemePicker.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { theme, type ThemeName } from '#lib/theme/theme.svelte.js';

  let saved = $state(false);
  let failure = $state('');

  async function choose(name: ThemeName) {
    saved = false;
    failure = '';
    try {
      await session.chooseTheme(name);
      saved = true;
    } catch (e) {
      failure = errorText(e);
    }
  }
</script>

<PageTitle title={m.settings_nav_appearance()} />

<section class="card stack" aria-labelledby="appearance-heading">
  <div>
    <h2 id="appearance-heading">{m.settings_nav_appearance()}</h2>
    <p class="hint">{m.appearance_intro()}</p>
  </div>
  <ThemePicker value={theme.current} onchange={choose} />
  {#if saved}
    <Alert kind="success">{m.appearance_saved()}</Alert>
  {/if}
  {#if failure}
    <Alert kind="error">{m.appearance_not_saved()} {failure}</Alert>
  {/if}
</section>

<section class="card stack" aria-labelledby="preview-heading">
  <h2 id="preview-heading">{m.appearance_preview_title()}</h2>
  <div class="sample">
    <h3>{m.appearance_preview_card()}</h3>
    <p>{m.appearance_preview_text()}</p>
    <p class="muted">{m.appearance_preview_muted()}</p>
    <p><a href="/settings">{m.appearance_preview_link()}</a></p>
    <div class="row">
      <span class="badge success">{m.appearance_preview_good()}</span>
      <span class="badge warning">{m.appearance_preview_warn()}</span>
      <span class="badge danger">{m.appearance_preview_bad()}</span>
    </div>
    <div class="row buttons">
      <button type="button" class="btn primary">{m.appearance_preview_button()}</button>
      <button type="button" class="btn">{m.appearance_preview_secondary()}</button>
    </div>
  </div>
</section>

<style>
  .sample {
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }

  .buttons {
    margin-top: var(--space-3);
  }

  .badge.success {
    border-color: var(--success);
    color: var(--success);
  }

  .badge.danger {
    border-color: var(--danger);
    color: var(--danger);
  }

  .badge.warning {
    color: var(--warning);
  }
</style>
