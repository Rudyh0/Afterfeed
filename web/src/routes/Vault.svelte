<script lang="ts">
  import { onMount } from 'svelte'
  import { hideSplash } from '../lib/splash'
  import type { Params } from '../lib/router.svelte'

  let { params = {} }: { params?: Params } = $props()
  onMount(hideSplash)
</script>

<h1 class="page-title">Vault</h1>
<p class="muted">{JSON.stringify(params)}</p>
