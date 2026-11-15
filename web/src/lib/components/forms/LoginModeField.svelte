<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  interface Props {
    legend: string;
    legendId?: string;
    loginRequired: boolean;
    /** Stops the "require a login" choice from being picked. */
    disableOn?: boolean;
    /** Stops the "no login" choice from being picked. */
    disableOff?: boolean;
    /** Explains why a choice is unavailable. */
    note?: string;
    error?: string;
  }

  let {
    legend,
    legendId,
    loginRequired = $bindable(),
    disableOn = false,
    disableOff = false,
    note,
    error
  }: Props = $props();

  const uid = $props.id();
  const errorId = $derived(error ? `${uid}-error` : undefined);
</script>

<!-- The group carries the error, because radio buttons themselves cannot be marked invalid; tabindex lets focusFirstInvalid land on it. -->
<fieldset
  class="stack"
  role="radiogroup"
  tabindex={error ? -1 : undefined}
  aria-invalid={error ? 'true' : undefined}
  aria-describedby={errorId}
>
  <legend id={legendId}>{legend}</legend>
  <label class="choice">
    <input
      type="radio"
      name="login-{uid}"
      checked={loginRequired}
      disabled={disableOn}
      onchange={() => (loginRequired = true)}
    />
    <span class="choice-text">
      <span>{m.setup_login_on()}</span>
      <span class="hint">{m.setup_login_on_hint()}</span>
    </span>
  </label>
  <label class="choice">
    <input
      type="radio"
      name="login-{uid}"
      checked={!loginRequired}
      disabled={disableOff}
      onchange={() => (loginRequired = false)}
    />
    <span class="choice-text">
      <span>{m.setup_login_off()}</span>
      <span class="hint">{m.setup_login_off_hint()}</span>
    </span>
  </label>
  {#if note}
    <p class="hint">{note}</p>
  {/if}
  {#if error}
    <p class="field-error" id={errorId}>{error}</p>
  {/if}
</fieldset>
