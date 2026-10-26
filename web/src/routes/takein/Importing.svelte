<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import Icon from '../../components/Icon.svelte'
  import { del, get, post } from '../../lib/api'
  import { errorMessage } from '../../lib/errors'
  import { formatNumber, t } from '../../lib/i18n/index.svelte'
  import { loadLibrary } from '../../lib/library.svelte'
  import { navigate } from '../../lib/router.svelte'
  import Step from './Step.svelte'
  import { continuePath, nextAfter } from './takein.svelte'

  let { id }: { id: string } = $props()

  interface Check {
    memories: number
    photos: number
    videos: number
    conversations: number
    messages: number
    files: number
    missing: number
    missing_examples?: string[]
  }

  interface YearReport {
    year: number
    new: number
    existing: number
    photos: number
    videos: number
  }

  interface Report {
    new: number
    existing: number
    photos: number
    videos: number
    conversations: number
    new_conversations: number
    messages: number
    years: YearReport[]
    missing: number
    missing_examples?: string[]
    problems: number
    problem_examples?: { path: string; error: string }[]
  }

  interface ImportStatus {
    id: number
    platform: string
    status: 'queued' | 'running' | 'needs_parts' | 'done' | 'failed' | 'cancelled'
    phase: string
    progress: number
    check?: Check
    report?: Report
    error?: string
  }

  let st = $state<ImportStatus | null>(null)
  let actionError = $state('')
  let busy = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let stopped = false

  const name = $derived(st ? t(`platform.${st.platform}`) : '')
  const active = $derived(st?.status === 'queued' || st?.status === 'running')
  const steps = ['reading', 'copying', 'indexing'] as const
  const stepIndex = $derived.by(() => {
    switch (st?.phase) {
      case 'copying':
        return 1
      case 'indexing':
        return 2
      default:
        return 0
    }
  })

  async function load() {
    try {
      const res = await get<{ import: ImportStatus }>(`/imports/${id}`)
      const wasActive = st?.status === 'queued' || st?.status === 'running'
      st = res.import
      if (wasActive && st.status === 'done') loadLibrary()
    } catch (err) {
      actionError = errorMessage(err)
    }
    if (!stopped && (!st || st.status === 'queued' || st.status === 'running')) {
      timer = setTimeout(load, 1000)
    }
  }

  onMount(load)
  onDestroy(() => {
    stopped = true
    clearTimeout(timer)
  })

  async function act(fn: () => Promise<unknown>) {
    actionError = ''
    busy = true
    try {
      await fn()
      clearTimeout(timer)
      await load()
    } catch (err) {
      actionError = errorMessage(err)
    } finally {
      busy = false
    }
  }

  function stepLabel(step: string, i: number): string {
    if (i === 0 && (st?.phase === 'opening' || st?.status === 'queued')) {
      return st?.status === 'queued' ? t('takein.phase.queued') : t('takein.phase.opening')
    }
    return t(`takein.phase.${step}`)
  }

  function errorText(code = ''): string {
    const platform = name
    if (code.startsWith('import.empty')) return t('takein.error_empty', { platform })
    if (code.startsWith('import.html_format')) return t('takein.error_html')
    if (code.startsWith('import.not_zip')) return t('takein.error_not_zip')
    if (code.startsWith('import.file_gone')) return t('takein.error_file_gone')
    return t('takein.error_generic')
  }

  const next = $derived(st ? nextAfter(st.platform) : undefined)
</script>

{#if !st}
  <p class="visually-hidden">{t('common.loading')}</p>
{:else if active}
  <Step title={t('takein.importing', { platform: name })} body={t('takein.close_page')}>
    <ol class="phases">
      {#each steps as step, i (step)}
        {@const state = i < stepIndex ? 'done' : i === stepIndex ? 'current' : 'next'}
        <li class={state}>
          <span class="mark" aria-hidden="true">
            {#if state === 'done'}<Icon name="check" size={16} />{:else if state === 'current'}<span class="dot"></span>{/if}
          </span>
          <div class="phase">
            <span class="label">{stepLabel(step, i)}</span>
            {#if state === 'current' && st.status === 'running'}
              <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(st.progress * 100)}>
                <span style:width={`${Math.max(2, Math.round(st.progress * 100))}%`}></span>
              </div>
            {/if}
          </div>
        </li>
      {/each}
    </ol>

    {#if st.check}
      {@render found(st.check)}
    {/if}

    {#snippet footer()}
      <span></span>
      <button class="btn" type="button" disabled={busy} onclick={() => act(() => del(`/imports/${id}`))}>{t('takein.cancel_import')}</button>
    {/snippet}
  </Step>
{:else if st.status === 'needs_parts'}
  <Step title={t('takein.missing_title')} body={t('takein.missing_body', { count: st.check?.missing ?? 0 })}>
    {#if st.check?.missing_examples?.length}
      <details class="card details">
        <summary>{t('takein.missing_examples')}</summary>
        <ul>
          {#each st.check.missing_examples.slice(0, 20) as ref (ref)}<li><code>{ref}</code></li>{/each}
        </ul>
      </details>
    {/if}
    {#if st.check}{@render found(st.check)}{/if}
    {#if actionError}<p class="error-text" role="alert">{actionError}</p>{/if}

    {#snippet footer()}
      <button class="btn btn-lg" type="button" disabled={busy} onclick={() => act(() => post(`/imports/${id}/continue`))}>
        {t('takein.import_without')}
      </button>
      <a class="btn btn-primary btn-lg" href={`/take-in/source/${st!.platform}/upload?import=${id}`}>{t('takein.add_files')}</a>
    {/snippet}
  </Step>
{:else if st.status === 'failed' || st.status === 'cancelled'}
  <Step title={st.status === 'failed' ? t('takein.report_failed') : t('takein.cancelled_title')}
        body={st.status === 'failed' ? errorText(st.error) : t('takein.cancelled_body')}>
    {#if actionError}<p class="error-text" role="alert">{actionError}</p>{/if}
    {#snippet footer()}
      <a class="btn btn-lg" href={`/take-in/source/${st!.platform}/upload`}>{t('common.back')}</a>
      <button class="btn btn-primary btn-lg" type="button" disabled={busy} onclick={() => act(() => post(`/imports/${id}/retry`))}>
        {t('common.retry')}
      </button>
    {/snippet}
  </Step>
{:else if st.status === 'done' && st.report}
  {@const r = st.report}
  <Step title={t('takein.report_title')} eyebrow={name} wide>
    <div class="stats">
      <div class="stat"><span class="value tabular">{formatNumber(r.new)}</span><span class="name">{t('takein.stat_new_memories')}</span></div>
      <div class="stat"><span class="value tabular">{formatNumber(r.existing)}</span><span class="name">{t('takein.report_existing')}</span></div>
      <div class="stat"><span class="value tabular">{formatNumber(r.photos)}</span><span class="name">{t('takein.stat_photos')}</span></div>
      <div class="stat"><span class="value tabular">{formatNumber(r.videos)}</span><span class="name">{t('takein.stat_videos')}</span></div>
      {#if r.conversations > 0}
        <div class="stat"><span class="value tabular">{formatNumber(r.conversations)}</span><span class="name">{t('takein.stat_conversations')}</span></div>
        <div class="stat"><span class="value tabular">{formatNumber(r.messages)}</span><span class="name">{t('takein.stat_messages')}</span></div>
      {/if}
    </div>

    {#if r.years.length > 0}
      <section class="years">
        <h2>{t('takein.report_per_year')}</h2>
        <div class="card table-wrap">
          <table>
            <thead>
              <tr>
                <th scope="col">{t('takein.report_year')}</th>
                <th scope="col">{t('takein.report_new')}</th>
                <th scope="col">{t('takein.report_existing')}</th>
                <th scope="col">{t('takein.stat_photos')}</th>
                <th scope="col">{t('takein.stat_videos')}</th>
              </tr>
            </thead>
            <tbody>
              {#each r.years as y (y.year)}
                <tr>
                  <th scope="row" class="tabular">{y.year}</th>
                  <td class="tabular">{formatNumber(y.new)}</td>
                  <td class="tabular">{formatNumber(y.existing)}</td>
                  <td class="tabular">{formatNumber(y.photos)}</td>
                  <td class="tabular">{formatNumber(y.videos)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        <p class="muted">{t('takein.report_check_years')}</p>
      </section>
    {/if}

    {#if r.missing > 0}
      <details class="card details">
        <summary>{t('takein.report_missing', { count: r.missing })}</summary>
        <ul>
          {#each (r.missing_examples ?? []).slice(0, 20) as ref (ref)}<li><code>{ref}</code></li>{/each}
        </ul>
      </details>
    {/if}
    {#if r.problems > 0}
      <details class="card details">
        <summary>{t('takein.report_unreadable', { count: r.problems })}</summary>
        <ul>
          {#each (r.problem_examples ?? []).slice(0, 20) as p, i (i)}<li><code>{p.path}</code> <span class="muted">{p.error}</span></li>{/each}
        </ul>
      </details>
    {/if}

    <p class="keep"><Icon name="archive" size={18} />{t('takein.keep_originals')}</p>

    {#snippet footer()}
      <a class="btn btn-lg" href={`/take-in/source/${st!.platform}/upload`}>{t('takein.upload_more')}</a>
      {#if next}
        <button class="btn btn-primary btn-lg" type="button" onclick={() => navigate(continuePath(st!.platform))}>
          {t('takein.next_source', { platform: t(`platform.${next}`) })}
        </button>
      {:else}
        <a class="btn btn-primary btn-lg" href="/take-in/done">{t('common.continue')}</a>
      {/if}
    {/snippet}
  </Step>
{/if}

{#snippet found(c: Check)}
  <section class="found">
    <h2>{t('takein.found')}</h2>
    <ul class="counts">
      <li>{t('count.memories', { count: c.memories })}</li>
      <li>{t('count.photos', { count: c.photos })}</li>
      <li>{t('count.videos', { count: c.videos })}</li>
      {#if c.conversations > 0}
        <li>{t('count.conversations', { count: c.conversations })}</li>
        <li>{t('count.messages', { count: c.messages })}</li>
      {/if}
    </ul>
  </section>
{/snippet}

<style>
  .phases {
    display: flex;
    flex-direction: column;
    gap: 18px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .phases li {
    display: flex;
    gap: 14px;
    align-items: flex-start;
  }

  .mark {
    width: 24px;
    height: 24px;
    flex: 0 0 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1.5px solid var(--border-strong);
    border-radius: 50%;
    color: var(--on-ink);
  }

  .done .mark {
    border-color: var(--ink);
    background: var(--ink);
  }

  .current .mark {
    border-color: var(--ink);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--ink);
    animation: pulse 1.4s ease-in-out infinite;
  }

  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }
  }

  .phase {
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding-top: 2px;
  }

  .label {
    font-size: 16px;
  }

  .next .label {
    color: var(--text-muted);
  }

  .bar {
    height: 6px;
    overflow: hidden;
    border-radius: 3px;
    background: var(--surface-3);
  }

  .bar span {
    display: block;
    height: 100%;
    background: var(--ink);
    transition: width 0.6s ease;
  }

  h2 {
    font-size: 16px;
    font-weight: 600;
  }

  .found {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--divider);
  }

  .counts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 18px;
    margin: 0;
    padding: 0;
    color: var(--text-2);
    list-style: none;
  }

  .stats {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 12px;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 16px;
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .value {
    font-size: 26px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .name {
    color: var(--text-3);
    font-size: 14px;
  }

  .years {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .table-wrap {
    overflow-x: auto;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 15px;
  }

  th,
  td {
    padding: 10px 16px;
    text-align: right;
    white-space: nowrap;
  }

  th:first-child {
    text-align: left;
  }

  thead th {
    color: var(--text-3);
    font-size: 13px;
    font-weight: 500;
  }

  tbody tr {
    border-top: 1px solid var(--divider);
  }

  tbody th {
    font-weight: 600;
  }

  .details {
    padding: 14px 16px;
  }

  .details summary {
    cursor: pointer;
    font-weight: 500;
  }

  .details ul {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin: 12px 0 0;
    padding: 0;
    list-style: none;
    font-size: 13px;
    word-break: break-all;
  }

  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  }

  .muted {
    color: var(--text-3);
    font-size: 14px;
  }

  .keep {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 14px 16px;
    border-radius: var(--radius-lg);
    background: var(--accent-soft);
    color: var(--text);
    font-size: 15px;
  }

  .keep :global(svg) {
    flex: 0 0 auto;
    margin-top: 2px;
  }
</style>
