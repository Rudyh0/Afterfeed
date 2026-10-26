// A random identifier for list keys and local uploads.
//
// crypto.randomUUID only exists on secure pages (https or localhost). A home
// server is often opened over plain http on its local address, where
// getRandomValues still works.
export function randomId(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}
