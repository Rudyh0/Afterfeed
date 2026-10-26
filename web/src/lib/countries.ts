// Country codes (ISO 3166-1 alpha-2), for recognising country names that
// people type, in their language or in English.

import { countryName, locale } from './i18n/index.svelte'

const CODES =
  'AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS XK YE YT ZA ZM ZW'.split(' ')

export function fold(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .trim()
}

/** Country codes whose name starts with q. */
export function findCountries(q: string, limit = 3): string[] {
  const f = fold(q)
  if (f.length < 2) return []
  const out: string[] = []
  for (const code of CODES) {
    const names = new Set([countryName(code), countryName(code, 'en')].map(fold))
    if ([...names].some((n) => n.startsWith(f))) out.push(code)
    if (out.length >= limit) break
  }
  return out
}

/** The code of the country with exactly this name, if any. */
export function countryByName(q: string): string | undefined {
  const f = fold(q)
  return CODES.find((code) => fold(countryName(code, locale())) === f || fold(countryName(code, 'en')) === f)
}
