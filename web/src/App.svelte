<script lang="ts">
  import { onMount, type Component } from 'svelte'
  import Shell from './components/Shell.svelte'
  import { t } from './lib/i18n/index.svelte'
  import { loadLibrary } from './lib/library.svelte'
  import { match, navigate, router, type Params } from './lib/router.svelte'
  import { loadSession, session } from './lib/session.svelte'
  import { hideSplash } from './lib/splash'
  import ChangePassword from './routes/ChangePassword.svelte'
  import Login from './routes/Login.svelte'
  import Memory from './routes/Memory.svelte'
  import Messages from './routes/Messages.svelte'
  import Month from './routes/Month.svelte'
  import NewMemory from './routes/NewMemory.svelte'
  import NotFound from './routes/NotFound.svelte'
  import Search from './routes/Search.svelte'
  import Settings from './routes/Settings.svelte'
  import TakeIn from './routes/takein/TakeIn.svelte'
  import Today from './routes/Today.svelte'
  import Unreachable from './routes/Unreachable.svelte'
  import Vault from './routes/Vault.svelte'
  import Year from './routes/Year.svelte'
  import Years from './routes/Years.svelte'

  type PageComponent = Component<{ params: Params }>

  interface Route {
    pattern: string
    component?: PageComponent
    /** Loads the page on first use, for pages with large dependencies. */
    load?: () => Promise<{ default: PageComponent }>
    bleed?: boolean
    /** Rendered without the sidebar, for example the full-screen photo viewer. */
    bare?: boolean
  }

  const routes: Route[] = [
    { pattern: '/', component: Today },
    { pattern: '/day/:date', component: Today },
    { pattern: '/years', component: Years },
    { pattern: '/years/:year', component: Year },
    { pattern: '/years/:year/:month', component: Month },
    { pattern: '/map', load: () => import('./routes/MapPage.svelte'), bleed: true },
    { pattern: '/search', component: Search },
    { pattern: '/messages/:platform?/:id?', component: Messages, bleed: true },
    { pattern: '/vault', component: Vault },
    { pattern: '/memory/:id', component: Memory, bare: true },
    { pattern: '/new', component: NewMemory },
    { pattern: '/settings', component: Settings },
    { pattern: '/settings/:section', component: Settings },
  ]

  const resolved = $derived.by(() => {
    for (const r of routes) {
      const params = match(r.pattern, router.path)
      if (params) return { route: r, params }
    }
    return null
  })

  // Pages that load on first use are kept once imported.
  let lazyPages = $state<Record<string, PageComponent>>({})

  $effect(() => {
    const r = resolved?.route
    if (!r?.load || lazyPages[r.pattern]) return
    const pattern = r.pattern
    r.load().then((m) => {
      lazyPages = { ...lazyPages, [pattern]: m.default }
    })
  })

  onMount(async () => {
    await loadSession()
    if (session.authenticated) loadLibrary()
  })

  // Route guards: the Take-in for a new installation, sign-in for everything else.
  $effect(() => {
    if (!session.loaded || session.unreachable) return
    const onTakeIn = router.path.startsWith('/take-in')
    if (session.setupRequired && !onTakeIn) {
      navigate('/take-in', { replace: true })
    } else if (session.authenticated && !session.takeInDone && !onTakeIn && !session.user?.must_change_password) {
      navigate('/take-in/sources', { replace: true })
    }
  })

  $effect(() => {
    if (session.loaded && (session.unreachable || !session.authenticated)) hideSplash()
  })
</script>

{#if !session.loaded}
  <!-- The splash covers the page while the session loads. -->
{:else if session.unreachable}
  <Unreachable />
{:else if router.path.startsWith('/take-in') && (session.setupRequired || session.authenticated)}
  <TakeIn />
{:else if !session.authenticated}
  <Login />
{:else if session.user?.must_change_password}
  <ChangePassword />
{:else if resolved}
  {@const Page = resolved?.route.component ?? lazyPages[resolved?.route.pattern ?? '']}
  {#if resolved?.route.bare}
    {#if Page}<Page params={resolved?.params ?? {}} />{/if}
  {:else}
    <Shell bleed={resolved?.route.bleed}>
      {#if Page}
        {#key resolved?.route.pattern}
          <Page params={resolved?.params ?? {}} />
        {/key}
      {:else}
        <p class="visually-hidden">{t('common.loading')}</p>
      {/if}
    </Shell>
  {/if}
{:else}
  <Shell><NotFound /></Shell>
{/if}
