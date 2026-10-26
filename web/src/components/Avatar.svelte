<script lang="ts">
  import { initial, tone } from '../lib/messages'
  import Icon from './Icon.svelte'

  interface Props {
    name: string
    group?: boolean
    size?: number
  }
  let { name, group = false, size = 44 }: Props = $props()
</script>

<span class="avatar" aria-hidden="true" style:--size={`${size}px`} style:background={`var(--tone-${tone(name)})`}>
  {#if group}<Icon name="users" size={Math.round(size * 0.45)} />{:else}{initial(name)}{/if}
</span>

<style>
  .avatar {
    flex: 0 0 var(--size);
    width: var(--size);
    height: var(--size);
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    color: var(--text-2);
    font-size: calc(var(--size) * 0.34);
    font-weight: 600;
  }
</style>
