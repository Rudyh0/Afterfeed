// Theme preference. The choice is stored in a cookie so the server can set
// data-theme on <html> before the page renders, which avoids a flash of the
// wrong colors on load.

export type ThemePreference = 'system' | 'light' | 'dark'

function read(): ThemePreference {
  const v = document.cookie
    .split('; ')
    .find((c) => c.startsWith('af_theme='))
    ?.split('=')[1]
  return v === 'light' || v === 'dark' ? v : 'system'
}

export const theme = $state<{ preference: ThemePreference }>({ preference: read() })

export function setTheme(p: ThemePreference) {
  theme.preference = p
  document.documentElement.dataset.theme = p
  document.cookie = `af_theme=${p}; Path=/; Max-Age=31536000; SameSite=Lax`
}
