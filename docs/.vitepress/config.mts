import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Afterfeed',
  description: 'Your memories, without the platform. A self-hosted archive for your social media exports.',
  base: '/Afterfeed/',
  lang: 'en',
  cleanUrls: true,
  lastUpdated: true,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/Afterfeed/favicon.svg' }],
    ['meta', { name: 'theme-color', content: '#17181C' }],
  ],
  themeConfig: {
    logo: { light: '/logo-light.svg', dark: '/logo-dark.svg', alt: 'Afterfeed' },
    siteTitle: 'Afterfeed',
    nav: [
      { text: 'Install', link: '/install/' },
      { text: 'Guide', link: '/guide/take-in' },
      { text: 'Configuration', link: '/configuration/environment' },
      { text: 'Maintenance', link: '/maintenance/updating' },
    ],
    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'What is Afterfeed?', link: '/introduction' },
          { text: 'Keep your exports', link: '/guide/keep-your-exports' },
        ],
      },
      {
        text: 'Install',
        items: [
          { text: 'Overview', link: '/install/' },
          { text: 'Docker Compose', link: '/install/docker-compose' },
          { text: 'Docker CLI', link: '/install/docker-cli' },
          { text: 'Portainer', link: '/install/portainer' },
          { text: 'Synology', link: '/install/synology' },
          { text: 'QNAP', link: '/install/qnap' },
          { text: 'TrueNAS', link: '/install/truenas' },
          { text: 'Unraid', link: '/install/unraid' },
          { text: 'Reverse proxy and HTTPS', link: '/install/reverse-proxy' },
        ],
      },
      {
        text: 'Guide',
        items: [
          { text: 'The Take-in', link: '/guide/take-in' },
          { text: 'Request your Facebook export', link: '/guide/exports/facebook' },
          { text: 'Request your Instagram export', link: '/guide/exports/instagram' },
          { text: 'Request your X export', link: '/guide/exports/x' },
          { text: 'Request your Snapchat export', link: '/guide/exports/snapchat' },
          { text: 'Importing large exports', link: '/guide/large-exports' },
          { text: 'Editing metadata', link: '/guide/metadata' },
          { text: 'The vault', link: '/guide/vault' },
        ],
      },
      {
        text: 'Configuration',
        items: [
          { text: 'Environment variables', link: '/configuration/environment' },
          { text: 'Volumes and folders', link: '/configuration/volumes' },
          { text: 'Map', link: '/configuration/map' },
          { text: 'Document previews', link: '/configuration/document-previews' },
        ],
      },
      {
        text: 'Maintenance',
        items: [
          { text: 'Updating', link: '/maintenance/updating' },
          { text: 'Backups', link: '/maintenance/backups' },
          { text: 'Commands', link: '/maintenance/commands' },
          { text: 'Troubleshooting', link: '/maintenance/troubleshooting' },
        ],
      },
      {
        text: 'Security',
        items: [{ text: 'Security model', link: '/security' }],
      },
      {
        text: 'Contributing',
        items: [
          { text: 'Development', link: '/contributing/development' },
          { text: 'Translating', link: '/contributing/translating' },
          { text: 'Importers', link: '/contributing/importers' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/Rudyh0/Afterfeed' }],
    search: { provider: 'local' },
    editLink: {
      pattern: 'https://github.com/Rudyh0/Afterfeed/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: 'Released under the AGPL-3.0 license.',
    },
  },
})
