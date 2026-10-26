<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../../components/Icon.svelte'
  import { ApiError, get, post } from '../../lib/api'
  import { errorMessage } from '../../lib/errors'
  import { formatBytes, t } from '../../lib/i18n/index.svelte'
  import { navigate, router } from '../../lib/router.svelte'
  import Step from './Step.svelte'
  import { continuePath } from './takein.svelte'
  import { addFiles, clearParts, loadUploads, removePart, resume, toggleFolderEntry, uploader, type Part } from './uploader.svelte'

  let { platform }: { platform: string } = $props()

  interface FolderEntry {
    path: string
    name: string
    size: number
    dir: boolean
    platform: string
  }

  const name = $derived(t(`platform.${platform}`))
  const importId = $derived(router.query.get('import'))
  const parts = $derived(uploader.parts.filter((p) => p.platform === platform))
  const totalSize = $derived(parts.reduce((n, p) => n + p.size, 0))
  const busy = $derived(parts.some((p) => p.status === 'uploading' || p.status === 'waiting'))
  const ready = $derived(parts.filter((p) => p.status === 'done'))
  let starting = $state(false)
  const canStart = $derived(ready.length > 0 && !busy && !starting)

  let folder = $state<FolderEntry[] | null>(null)
  let folderLoading = $state(false)
  let dragging = $state(false)
  let error = $state('')
  // The sentence with the folder path, split around the path so it can be shown as code.
  const folderBody = $derived(t('takein.upload_folder_body', { path: '\u0000' }).split('\u0000'))

  onMount(() => {
    loadUploads(platform).catch(() => {})
    loadFolder()
  })

  async function loadFolder() {
    folderLoading = true
    try {
      const res = await get<{ entries: FolderEntry[] }>('/import-folder')
      folder = res.entries
    } catch {
      folder = []
    } finally {
      folderLoading = false
    }
  }

  function selected(entry: FolderEntry): boolean {
    return parts.some((p) => p.key === `folder:${entry.path}`)
  }

  function onPick(e: Event) {
    const files = (e.currentTarget as HTMLInputElement).files
    if (files?.length) addFiles(Array.from(files), platform)
    ;(e.currentTarget as HTMLInputElement).value = ''
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragging = false
    const files = e.dataTransfer?.files
    if (files?.length) addFiles(Array.from(files), platform)
  }

  function percent(p: Part): number {
    return p.size > 0 ? Math.floor((p.received / p.size) * 100) : 0
  }

  function hint(p: Part): { text: string; warn: boolean } | null {
    if (p.status !== 'done' || p.detected === undefined) return null
    if (p.detected === platform) return null
    if (p.detected === '') return { text: t('takein.not_recognised'), warn: false }
    return { text: t('takein.looks_like', { platform: t(`platform.${p.detected}`) }), warn: true }
  }

  async function start() {
    error = ''
    starting = true
    const body = {
      platform,
      uploads: ready.filter((p) => p.kind === 'upload').map((p) => p.uploadId!),
      files: ready.filter((p) => p.kind === 'folder').map((p) => p.path!),
    }
    try {
      const res = importId
        ? await post<{ import: { id: number } }>(`/imports/${importId}/files`, { uploads: body.uploads, files: body.files })
        : await post<{ import: { id: number } }>('/imports', body)
      clearParts(platform)
      navigate(`/take-in/import/${res.import.id}`)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'import.not_zip') {
        const file = err.message.replace(/^import\.not_zip:\s*/, '').split(/:\s|\s\(/)[0]
        error = t('takein.error_not_zip_named', { name: file })
        loadUploads(platform).catch(() => {})
      } else {
        error = errorMessage(err)
      }
    } finally {
      starting = false
    }
  }
</script>

<Step title={t('takein.upload_title', { platform: name })} body={t('takein.upload_body')} wide>
  <label
    class="drop"
    class:dragging
    ondragover={(e) => {
      e.preventDefault()
      dragging = true
    }}
    ondragleave={() => (dragging = false)}
    ondrop={onDrop}
  >
    <input type="file" multiple accept=".zip,application/zip" onchange={onPick} class="visually-hidden" />
    <span class="drop-icon"><Icon name="upload" size={24} /></span>
    <span class="drop-title">{t('takein.drop')}</span>
    <span class="btn">{t('takein.choose_files')}</span>
  </label>

  {#if parts.length > 0}
    <section class="parts" aria-labelledby="parts-title">
      <div class="parts-head">
        <h2 id="parts-title">{t('takein.parts_title')}</h2>
        <span class="muted tabular">{t('takein.parts_count', { count: parts.length, size: formatBytes(totalSize) })}</span>
      </div>
      <ul class="card list">
        {#each parts as p (p.key)}
          {@const h = hint(p)}
          <li>
            <span class="file-icon"><Icon name={p.kind === 'folder' ? 'folder' : 'archive'} size={20} /></span>
            <div class="file">
              <div class="file-row">
                <span class="file-name">{p.name}</span>
                <span class="muted tabular">{formatBytes(p.size)}</span>
              </div>
              {#if p.status === 'uploading' || p.status === 'waiting'}
                <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={percent(p)}>
                  <span style:width={`${percent(p)}%`}></span>
                </div>
                <span class="status tabular">{t('takein.uploading')} · {percent(p)}%</span>
              {:else if p.status === 'paused'}
                <div class="bar paused"><span style:width={`${percent(p)}%`}></span></div>
                <span class="status">
                  {t('takein.upload_paused')} · {percent(p)}%
                  {#if p.file}
                    · <button class="link" type="button" onclick={() => resume(p)}>{t('takein.resume')}</button>
                  {:else}
                    · {t('takein.upload_resume_hint')}
                  {/if}
                </span>
              {:else if p.status === 'error'}
                <span class="status error-text">{t(p.error ?? 'takein.upload_failed')}</span>
              {:else}
                <span class="status">
                  {p.kind === 'folder' ? t('takein.in_folder') : t('takein.upload_done')}
                  {#if h}· <span class:warn={h.warn}>{h.text}</span>{/if}
                </span>
              {/if}
            </div>
            <button class="icon-btn" type="button" aria-label={t('takein.remove_file', { name: p.name })} onclick={() => removePart(p)}>
              <Icon name="close" size={18} />
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <section class="folder card" aria-labelledby="folder-title">
    <div class="folder-head">
      <div>
        <h2 id="folder-title">{t('takein.upload_folder_title')}</h2>
        <p class="muted">{folderBody[0]}<code>data/import</code>{folderBody[1] ?? ''}</p>
      </div>
      <button class="btn" type="button" onclick={loadFolder} disabled={folderLoading}>
        <Icon name="refresh" size={16} />{t('takein.refresh_folder')}
      </button>
    </div>
    {#if folder && folder.length === 0}
      <p class="muted empty">{t('takein.upload_folder_empty', { path: 'data/import' })}</p>
    {:else if folder}
      <ul class="entries">
        {#each folder as entry (entry.path)}
          <li>
            <label class="entry">
              <input
                type="checkbox"
                checked={selected(entry)}
                onchange={(e) => toggleFolderEntry(entry, platform, (e.currentTarget as HTMLInputElement).checked)}
              />
              <Icon name={entry.dir ? 'folder' : 'archive'} size={18} />
              <span class="file-name">{entry.name}</span>
              {#if entry.platform && entry.platform !== platform}
                <span class="pill">{t(`platform.${entry.platform}`)}</span>
              {/if}
              <span class="muted tabular">{formatBytes(entry.size)}</span>
            </label>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#snippet footer()}
    <a class="btn btn-lg" href={importId ? `/take-in/import/${importId}` : `/take-in/source/${platform}`}>{t('common.back')}</a>
    <div class="right">
      {#if !importId}
        <a class="link" href={continuePath(platform)}>{t('takein.skip_source', { platform: name })}</a>
      {/if}
      <button class="btn btn-primary btn-lg" type="button" disabled={!canStart} onclick={start}>
        {importId ? t('takein.add_to_import') : t('takein.start_import')}
      </button>
    </div>
  {/snippet}
</Step>

<style>
  .drop {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 36px 20px;
    border: 1.5px dashed var(--border-strong);
    border-radius: var(--radius-lg);
    background: var(--surface);
    text-align: center;
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
  }

  .drop:hover,
  .drop.dragging {
    border-color: var(--text-muted);
    background: var(--surface-2);
  }

  .drop:focus-within {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .drop-icon {
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--surface-2);
  }

  .drop-title {
    font-size: 16px;
    font-weight: 600;
  }

  h2 {
    font-size: 16px;
    font-weight: 600;
  }

  .parts {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .parts-head,
  .folder-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
  }

  .list,
  .entries {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .list li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 12px 14px 16px;
  }

  .list li + li {
    border-top: 1px solid var(--divider);
  }

  .file-icon {
    color: var(--text-3);
  }

  .file {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .file-row {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }

  .file-name {
    min-width: 0;
    overflow: hidden;
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status {
    color: var(--text-3);
    font-size: 13px;
  }

  .warn {
    color: var(--danger);
  }

  .bar {
    height: 4px;
    overflow: hidden;
    border-radius: 2px;
    background: var(--surface-3);
  }

  .bar span {
    display: block;
    height: 100%;
    background: var(--ink);
    transition: width 0.3s;
  }

  .bar.paused span {
    background: var(--text-muted);
  }

  .folder {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 20px;
  }

  .folder-head {
    align-items: flex-start;
  }

  .folder-head p {
    margin-top: 4px;
  }

  .entries li + li {
    border-top: 1px solid var(--divider);
  }

  .entry {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 0;
    cursor: pointer;
  }

  .entry .file-name {
    flex: 1 1 auto;
  }

  .entry input {
    width: 18px;
    height: 18px;
    accent-color: var(--ink);
  }

  .muted {
    color: var(--text-3);
    font-size: 14px;
  }

  .empty {
    padding: 4px 0;
  }

  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 13px;
  }

  .right {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
  }

  @media (max-width: 600px) {
    .right {
      width: 100%;
      flex-direction: column-reverse;
      align-items: stretch;
      text-align: center;
    }

    .folder-head {
      flex-direction: column;
    }
  }
</style>
