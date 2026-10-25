<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
</script>

<label class="sr-only" for="tank-switcher">{m.tank_switcher_label()}</label>
<select id="tank-switcher" disabled>
  <option>{m.tank_switcher_none()}</option>
</select>
