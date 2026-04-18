import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Axel',
  description: 'Schema and query language tool for PostgreSQL',

  head: [
    ['link', { rel: 'icon', href: '/favicon.ico' }],
  ],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Guide', link: '/installation' },
      { text: 'ASL', link: '/asl' },
      { text: 'AQL', link: '/aql' },
      { text: 'CLI', link: '/cli' },
      {
        text: 'GitHub',
        link: 'https://github.com/struckchure/axel',
      },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Introduction', link: '/' },
          { text: 'Installation', link: '/installation' },
        ],
      },
      {
        text: 'Languages',
        items: [
          { text: 'Schema Language (ASL)', link: '/asl' },
          { text: 'Query Language (AQL)', link: '/aql' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI', link: '/cli' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/struckchure/axel' },
    ],

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2024-present Axel Contributors',
    },

    search: {
      provider: 'local',
    },
  },
})
