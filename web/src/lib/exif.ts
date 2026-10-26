// Reads the capture time and location from a JPEG photo, so a new memory can
// be filled in before it is saved. Other formats are read on the server.

export interface PhotoInfo {
  /** Local date and time as YYYY-MM-DD and HH:MM, without a time zone. */
  date?: string
  time?: string
  lat?: number
  lon?: number
}

export async function readPhotoInfo(file: File): Promise<PhotoInfo> {
  if (!/jpe?g$/i.test(file.type) && !/\.jpe?g$/i.test(file.name)) return {}
  const buf = new DataView(await file.slice(0, 256 * 1024).arrayBuffer())
  try {
    return parseJpeg(buf)
  } catch {
    return {}
  }
}

function parseJpeg(v: DataView): PhotoInfo {
  if (v.getUint16(0) !== 0xffd8) return {}
  let off = 2
  while (off + 4 < v.byteLength) {
    if (v.getUint8(off) !== 0xff) return {}
    const marker = v.getUint8(off + 1)
    const size = v.getUint16(off + 2)
    if (marker === 0xda) return {}
    if (marker === 0xe1 && v.getUint32(off + 4) === 0x45786966) {
      return parseTiff(new DataView(v.buffer, v.byteOffset + off + 10, Math.min(size - 8, v.byteLength - off - 10)))
    }
    off += 2 + size
  }
  return {}
}

function parseTiff(v: DataView): PhotoInfo {
  const le = v.getUint16(0) === 0x4949
  const u16 = (o: number) => v.getUint16(o, le)
  const u32 = (o: number) => v.getUint32(o, le)
  const ifd = (o: number) => {
    const out = new Map<number, { type: number; count: number; at: number }>()
    if (o <= 0 || o + 2 > v.byteLength) return out
    const n = u16(o)
    for (let i = 0; i < n && o + 2 + i * 12 + 12 <= v.byteLength; i++) {
      const e = o + 2 + i * 12
      const type = u16(e + 2)
      const count = u32(e + 4)
      const size = ({ 1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8 } as Record<number, number>)[type] ?? 0
      const at = size * count <= 4 ? e + 8 : u32(e + 8)
      out.set(u16(e), { type, count, at })
    }
    return out
  }
  const str = (e?: { count: number; at: number }) => {
    if (!e || e.at + e.count > v.byteLength) return ''
    let s = ''
    for (let i = 0; i < e.count; i++) {
      const c = v.getUint8(e.at + i)
      if (c === 0) break
      s += String.fromCharCode(c)
    }
    return s.trim()
  }
  const rational = (at: number) => (u32(at + 4) ? u32(at) / u32(at + 4) : 0)
  const coord = (e?: { type: number; count: number; at: number }, ref?: string) => {
    if (!e || e.type !== 5 || e.count < 3 || e.at + 24 > v.byteLength) return undefined
    const d = rational(e.at) + rational(e.at + 8) / 60 + rational(e.at + 16) / 3600
    return ref === 'S' || ref === 'W' ? -d : d
  }

  const ifd0 = ifd(u32(4))
  const info: PhotoInfo = {}
  const exifPtr = ifd0.get(0x8769)
  let stamp = str(ifd0.get(0x0132))
  if (exifPtr) {
    const exif = ifd(u32(exifPtr.at))
    stamp = str(exif.get(0x9003)) || str(exif.get(0x9004)) || stamp
  }
  const m = stamp.match(/^(\d{4}):(\d{2}):(\d{2}) (\d{2}):(\d{2})/)
  if (m && Number(m[1]) > 1900) {
    info.date = `${m[1]}-${m[2]}-${m[3]}`
    info.time = `${m[4]}:${m[5]}`
  }
  const gpsPtr = ifd0.get(0x8825)
  if (gpsPtr) {
    const gps = ifd(u32(gpsPtr.at))
    const lat = coord(gps.get(2), str(gps.get(1)))
    const lon = coord(gps.get(4), str(gps.get(3)))
    if (lat !== undefined && lon !== undefined && !(lat === 0 && lon === 0) && Math.abs(lat) <= 90 && Math.abs(lon) <= 180) {
      info.lat = lat
      info.lon = lon
    }
  }
  return info
}
