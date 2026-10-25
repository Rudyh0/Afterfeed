<script lang="ts">
  import { onMount } from 'svelte'
  import Logo from '../../components/Logo.svelte'
  import { match, navigate, router } from '../../lib/router.svelte'
  import { session } from '../../lib/session.svelte'
  import { hideSplash } from '../../lib/splash'
  import Account from './Account.svelte'
  import Done from './Done.svelte'
  import Guide from './Guide.svelte'
  import Importing from './Importing.svelte'
  import Question from './Question.svelte'
  import Sources from './Sources.svelte'
  import Upload from './Upload.svelte'
  import Welcome from './Welcome.svelte'

  onMount(hideSplash)

  const view = $derived.by(() => {
    const p = router.path
    if (p === '/take-in' || p === '/take-in/') return { name: 'welcome' as const }
    if (p === '/take-in/account') return { name: 'account' as const }
    if (p === '/take-in/sources') return { name: 'sources' as const }
    if (p === '/take-in/done') return { name: 'done' as const }
    let m = match('/take-in/source/:platform', p)
    if (m) return { name: 'question' as const, platform: m.platform }
    m = match('/take-in/source/:platform/guide', p)
    if (m) return { name: 'guide' as const, platform: m.platform }
    m = match('/take-in/source/:platform/upload', p)
    if (m) return { name: 'upload' as const, platform: m.platform }
    m = match('/take-in/import/:id', p)
    if (m) return { name: 'importing' as const, id: m.id }
    return { name: 'welcome' as const }
  })

  // Before the account exists only the first two steps are available; after
  // that, the account step is skipped.
  $effect(() => {
    if (session.setupRequired && view.name !== 'welcome' && view.name !== 'account') {
      navigate('/take-in', { replace: true })
    } else if (!session.setupRequired && (view.name === 'welcome' || view.name === 'account')) {
      navigate('/take-in/sources', { replace: true })
    }
  })
</script>

<div class="takein">
  <header>
    <a href={session.authenticated && session.takeInDone ? '/' : '/take-in'} class="brand" aria-label="Afterfeed">
      <Logo width={64} />
      <span>Afterfeed</span>
    </a>
  </header>

  <main>
    {#if view.name === 'welcome'}
      <Welcome />
    {:else if view.name === 'account'}
      <Account />
    {:else if view.name === 'sources'}
      <Sources />
    {:else if view.name === 'question'}
      <Question platform={view.platform} />
    {:else if view.name === 'guide'}
      <Guide platform={view.platform} />
    {:else if view.name === 'upload'}
      <Upload platform={view.platform} />
    {:else if view.name === 'importing'}
      <Importing id={view.id} />
    {:else if view.name === 'done'}
      <Done />
    {/if}
  </main>
</div>

<style>
  .takein {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  header {
    display: flex;
    align-items: center;
    height: 72px;
    padding: 0 32px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--text);
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.01em;
    text-decoration: none;
  }

  main {
    flex: 1 1 auto;
    display: flex;
    justify-content: center;
    padding: 24px 16px 96px;
  }

  @media (max-width: 600px) {
    header {
      padding: 0 16px;
    }

    main {
      padding-top: 8px;
    }
  }
</style>
