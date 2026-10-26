// Uploads export files in chunks. The server remembers how much of each file
// it has, so an upload continues where it stopped when the same file is
// added again, also after a reload.
//
// The queue lives at module level: it keeps running while the user moves
// between Take-in steps.

import { ApiError, del, get, post } from '../../lib/api'
import { randomId } from '../../lib/id'

export type PartStatus = 'waiting' | 'uploading' | 'paused' | 'done' | 'error'

export interface Part {
  key: string
  kind: 'upload' | 'folder'
  name: string
  size: number
  platform: string
  received: number
  status: PartStatus
  /** Translation key of an error, if any. */
  error?: string
  /** The platform the server recognised, or an empty string. */
  detected?: string
  file?: File
  uploadId?: string
  path?: string
}

interface UploadInfo {
  id: string
  name: string
  size: number
  modified: number
  received: number
  platform: string
  detected: string
  status: 'uploading' | 'complete'
}

const DEFAULT_CHUNK = 16 * 1024 * 1024

export const uploader = $state<{ parts: Part[]; running: boolean }>({ parts: [], running: false })

let chunkSize = DEFAULT_CHUNK
let current: XMLHttpRequest | null = null

function fromUpload(u: UploadInfo): Part {
  return {
    key: `upload:${u.id}`,
    kind: 'upload',
    name: u.name,
    size: u.size,
    platform: u.platform,
    received: u.received,
    status: u.status === 'complete' ? 'done' : 'paused',
    detected: u.status === 'complete' ? u.detected : undefined,
    uploadId: u.id,
  }
}

/** Shows uploads of this platform that were started earlier. */
export async function loadUploads(platform: string) {
  const res = await get<{ uploads: UploadInfo[]; chunk_size: number }>(`/uploads?platform=${encodeURIComponent(platform)}`)
  chunkSize = res.chunk_size || DEFAULT_CHUNK
  for (const u of res.uploads) {
    const existing = uploader.parts.find((p) => p.uploadId === u.id)
    if (existing) {
      // Keep the local state of an upload that is running.
      if (existing.status !== 'uploading' && existing.status !== 'waiting') {
        Object.assign(existing, { received: u.received, detected: u.detected, status: u.status === 'complete' ? 'done' : existing.file ? 'waiting' : 'paused' })
      }
    } else {
      uploader.parts.push(fromUpload(u))
    }
  }
}

/** Parts that belong to a platform. */
export function partsFor(platform: string): Part[] {
  return uploader.parts.filter((p) => p.platform === platform)
}

function isZip(f: File): boolean {
  return /\.zip$/i.test(f.name) || f.type === 'application/zip' || f.type === 'application/x-zip-compressed'
}

/** Adds files chosen or dropped by the user and starts uploading them. */
export async function addFiles(files: Iterable<File>, platform: string) {
  for (const file of files) {
    if (!isZip(file)) {
      uploader.parts.push({
        key: `local:${randomId()}`,
        kind: 'upload',
        name: file.name,
        size: file.size,
        platform,
        received: 0,
        status: 'error',
        error: 'takein.not_zip',
      })
      continue
    }
    try {
      const res = await post<{ upload: UploadInfo; chunk_size: number }>('/uploads', {
        name: file.name,
        size: file.size,
        modified: file.lastModified,
        platform,
      })
      chunkSize = res.chunk_size || DEFAULT_CHUNK
      const u = res.upload
      const existing = uploader.parts.find((p) => p.uploadId === u.id)
      const part: Part = existing ?? fromUpload(u)
      part.file = file
      part.received = u.received
      part.platform = platform
      part.status = u.status === 'complete' ? 'done' : 'waiting'
      part.error = undefined
      if (!existing) uploader.parts.push(part)
    } catch (err) {
      uploader.parts.push({
        key: `local:${randomId()}`,
        kind: 'upload',
        name: file.name,
        size: file.size,
        platform,
        received: 0,
        status: 'error',
        error: err instanceof ApiError && err.code === 'upload.no_space' ? 'takein.no_space' : 'takein.upload_failed',
      })
    }
  }
  void run()
}

/** Adds or removes a file from the import folder. */
export function toggleFolderEntry(entry: { path: string; name: string; size: number; platform: string }, platform: string, on: boolean) {
  const key = `folder:${entry.path}`
  const i = uploader.parts.findIndex((p) => p.key === key)
  if (on && i < 0) {
    uploader.parts.push({
      key,
      kind: 'folder',
      name: entry.name,
      size: entry.size,
      platform,
      received: entry.size,
      status: 'done',
      detected: entry.platform,
      path: entry.path,
    })
  } else if (!on && i >= 0) {
    uploader.parts.splice(i, 1)
  }
}

/** Removes a part; an upload is deleted on the server. */
export async function removePart(part: Part) {
  const i = uploader.parts.findIndex((p) => p.key === part.key)
  if (i >= 0) uploader.parts.splice(i, 1)
  if (part.status === 'uploading' && current) current.abort()
  if (part.uploadId) {
    try {
      await del(`/uploads/${part.uploadId}`)
    } catch {
      // Already gone.
    }
  }
}

/** Continues a paused upload whose file is still selected. */
export function resume(part: Part) {
  const p = uploader.parts.find((x) => x.key === part.key)
  if (p?.file) {
    p.status = 'waiting'
    void run()
  }
}

/** Removes parts that were used in an import, after it started. */
export function clearParts(platform: string) {
  uploader.parts = uploader.parts.filter((p) => p.platform !== platform)
}

function putChunk(id: string, offset: number, blob: Blob, onProgress: (sent: number) => void): Promise<UploadInfo> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    current = xhr
    xhr.open('PUT', `/api/uploads/${id}?offset=${offset}`)
    xhr.setRequestHeader('X-Afterfeed', '1')
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.upload.onprogress = (e) => onProgress(e.loaded)
    xhr.onload = () => {
      current = null
      let body: { upload?: UploadInfo; error?: { code: string; message: string } } = {}
      try {
        body = JSON.parse(xhr.responseText)
      } catch {
        // Not JSON.
      }
      if (xhr.status === 200 && body.upload) resolve(body.upload)
      else if (xhr.status === 409 && body.upload) resolve(body.upload) // continue from the server's offset
      else reject(new ApiError(xhr.status, body.error?.code ?? 'internal', body.error?.message ?? xhr.statusText))
    }
    xhr.onerror = () => {
      current = null
      reject(new ApiError(0, 'network', 'network error'))
    }
    xhr.onabort = () => {
      current = null
      reject(new ApiError(0, 'aborted', 'aborted'))
    }
    xhr.send(blob)
  })
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** Uploads waiting parts one at a time. */
async function run() {
  if (uploader.running) return
  uploader.running = true
  try {
    for (;;) {
      const part = uploader.parts.find((p) => p.status === 'waiting' && p.file && p.uploadId)
      if (!part) break
      part.status = 'uploading'
      let failures = 0
      while (part.status === 'uploading' && part.received < part.size) {
        const start = part.received
        const blob = part.file!.slice(start, Math.min(start + chunkSize, part.size))
        try {
          const u = await putChunk(part.uploadId!, start, blob, (sent) => {
            part.received = Math.min(start + sent, part.size)
          })
          part.received = u.received
          if (u.status === 'complete') {
            part.status = 'done'
            part.detected = u.detected
          }
          failures = 0
        } catch (err) {
          part.received = start
          if (!uploader.parts.includes(part)) break // removed
          if (err instanceof ApiError && err.code === 'aborted') break
          if (err instanceof ApiError && err.status === 404) {
            part.status = 'error'
            part.error = 'takein.upload_failed'
            break
          }
          failures++
          if (failures > 5) {
            part.status = 'paused'
            break
          }
          await sleep(1000 * 2 ** failures)
        }
      }
      if (part.status === 'uploading' && part.received >= part.size) part.status = 'done'
    }
  } finally {
    uploader.running = false
  }
}
