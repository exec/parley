import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Parley Developer API',
  description: 'Build bots and integrations on Parley',
  base: '/docs/',
  cleanUrls: true,
  srcExclude: ['superpowers/**'],

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'Parley API',

    nav: [
      { text: 'Guide', link: '/' },
      { text: 'Endpoints', link: '/endpoints/messages' },
      { text: 'Theming', link: '/theming' },
      { text: 'Parley', link: 'https://parley.x86-64.com' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Introduction', link: '/' },
          { text: 'Authentication', link: '/authentication' },
          { text: 'Bots', link: '/bots' },
          { text: 'Selfbots', link: '/selfbots' },
          { text: 'Rate Limits & Limits', link: '/limits' },
        ],
      },
      {
        text: 'Customization',
        items: [
          { text: 'Theming', link: '/theming' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Messages', link: '/endpoints/messages' },
          { text: 'Channels', link: '/endpoints/channels' },
          { text: 'Servers', link: '/endpoints/servers' },
          { text: 'Users', link: '/endpoints/users' },
          { text: 'Direct Messages', link: '/endpoints/dms' },
          { text: 'Voice', link: '/endpoints/voice' },
          { text: 'API Keys', link: '/endpoints/developer' },
          { text: 'WebSocket', link: '/endpoints/websocket' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com' },
    ],

    footer: {
      message: 'Parley Developer API',
      copyright: '© 2026 Parley',
    },

    search: {
      provider: 'local',
    },
  },

  head: [
    ['meta', { name: 'theme-color', content: '#32CD32' }],
  ],
})
